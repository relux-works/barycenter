package winprobe

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

type RecoveryOutcome struct {
	SessionID string
	Path      string
	Reason    CaptureReason
	Result    ProbeResult
	Cause     string
	Frames    uint64
	Format    CaptureFormat
}

type recoveryOperationError struct{ error }

func recoveryOperationFailure(err error) error {
	if err == nil {
		return nil
	}
	return recoveryOperationError{error: err}
}

func isRecoveryOperationFailure(err error) bool {
	var target recoveryOperationError
	return errors.As(err, &target)
}

func RecoverArtifacts(dir string, permission PermissionStatus, minimumMillis uint64) ([]RecoveryOutcome, error) {
	return recoverArtifacts(productionArtifactFS, dir, permission, minimumMillis)
}

func recoverArtifacts(fileSystem artifactFileSystem, dir string, permission PermissionStatus, minimumMillis uint64) ([]RecoveryOutcome, error) {
	entries, err := fileSystem.ReadDir(dir)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	partials := make(map[string]bool)
	var outcomes []RecoveryOutcome
	var recoveryFailures []error
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".partial") {
			continue
		}
		sessionID := strings.TrimSuffix(entry.Name(), ".partial")
		partials[sessionID] = true
		partial := filepath.Join(dir, entry.Name())
		sidecar := partial + ".reason"
		outcome := RecoveryOutcome{SessionID: sessionID, Path: partial, Result: ResultDiscard}
		raw, readErr := fileSystem.ReadFile(sidecar)
		if readErr != nil {
			outcome.Cause = "missing/corrupt sidecar: " + readErr.Error()
			outcomes = append(outcomes, discardRecovered(fileSystem, partial, sidecar, outcome))
			continue
		}
		record, parseErr := ParseReasonRecord(raw, sessionID)
		if parseErr != nil {
			outcome.Cause = "invalid sidecar: " + parseErr.Error()
			outcomes = append(outcomes, discardRecovered(fileSystem, partial, sidecar, outcome))
			continue
		}
		outcome.Reason = record.Reason
		if !PromotableReason(record.Reason) || permission != PermissionAllowed {
			outcome.Cause = fmt.Sprintf("non-promotable reason or permission=%s", permission)
			outcomes = append(outcomes, discardRecovered(fileSystem, partial, sidecar, outcome))
			continue
		}
		format, frames, recoverErr := recoverOneArtifact(fileSystem, partial, minimumMillis)
		outcome.Format, outcome.Frames = format, frames
		if recoverErr != nil {
			outcome.Cause = recoverErr.Error()
			if isRecoveryOperationFailure(recoverErr) {
				outcome.Result = ResultFail
			}
			outcomes = append(outcomes, discardRecovered(fileSystem, partial, sidecar, outcome))
			continue
		}
		final := strings.TrimSuffix(partial, ".partial") + ".wav"
		if err := fileSystem.RenameNoReplace(partial, final); err != nil {
			outcome.Result = ResultFail
			cleanupErr := removePathsAndVerifyAbsent(fileSystem, partial, sidecar)
			outcome.Cause = errors.Join(fmt.Errorf("promote recovered artifact: %w", err), cleanupErr).Error()
			outcome.Path = ""
			outcomes = append(outcomes, outcome)
			continue
		}
		verifiedFormat, verifiedFrames, verifyErr := verifyWAVFile(fileSystem, final)
		if verifyErr != nil {
			cleanupErr := removePathsAndVerifyAbsent(fileSystem, final, partial, sidecar)
			outcome.Path = ""
			if cleanupErr != nil {
				outcome.Path = final
			}
			outcome.Result = ResultFail
			outcome.Cause = errors.Join(fmt.Errorf("verify recovered WAV: %w", verifyErr), cleanupErr).Error()
			outcomes = append(outcomes, outcome)
			continue
		}
		outcome.Path = final
		outcome.Format = verifiedFormat
		outcome.Frames = verifiedFrames
		if cleanupErr := removePathsAndVerifyAbsent(fileSystem, partial, sidecar); cleanupErr != nil {
			outcome.Result = ResultFail
			outcome.Cause = cleanupErr.Error()
		} else {
			outcome.Result = ResultPass
		}
		outcomes = append(outcomes, outcome)
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".partial.reason") {
			continue
		}
		sessionID := strings.TrimSuffix(entry.Name(), ".partial.reason")
		if !partials[sessionID] {
			path := filepath.Join(dir, entry.Name())
			outcome := RecoveryOutcome{SessionID: sessionID, Path: path, Result: ResultDiscard, Cause: "orphan sidecar removed"}
			if cleanupErr := removePathsAndVerifyAbsent(fileSystem, path); cleanupErr != nil {
				outcome.Result = ResultFail
				outcome.Cause = "orphan sidecar cleanup: " + cleanupErr.Error()
			} else {
				outcome.Path = ""
			}
			outcomes = append(outcomes, outcome)
		}
	}
	for _, outcome := range outcomes {
		if outcome.Result == ResultFail {
			recoveryFailures = append(recoveryFailures, fmt.Errorf("recovery %s: %s", outcome.SessionID, outcome.Cause))
		}
	}
	return outcomes, errors.Join(recoveryFailures...)
}

func recoverOneArtifact(fileSystem artifactFileSystem, path string, minimumMillis uint64) (format CaptureFormat, frames uint64, resultErr error) {
	file, err := fileSystem.OpenFile(path, os.O_RDWR, 0)
	if err != nil {
		return CaptureFormat{}, 0, recoveryOperationFailure(err)
	}
	defer func() {
		if err := file.Close(); err != nil {
			resultErr = errors.Join(resultErr, recoveryOperationFailure(fmt.Errorf("close recovered partial: %w", err)))
		}
	}()
	header := make([]byte, ProbeWAVHeaderSize)
	if _, err := io.ReadFull(file, header); err != nil {
		return CaptureFormat{}, 0, fmt.Errorf("read streaming header: %w", err)
	}
	format, err = parseStreamingWAVHeader(header)
	if err != nil {
		return CaptureFormat{}, 0, err
	}
	stat, err := file.Stat()
	if err != nil {
		return CaptureFormat{}, 0, recoveryOperationFailure(err)
	}
	if stat.Size() < ProbeWAVHeaderSize {
		return CaptureFormat{}, 0, fmt.Errorf("partial shorter than WAV header")
	}
	bytesPerFrame := int64(format.Channels) * 4
	pcmBytes := stat.Size() - ProbeWAVHeaderSize
	wholeBytes := pcmBytes / bytesPerFrame * bytesPerFrame
	if wholeBytes != pcmBytes {
		if err := file.Truncate(ProbeWAVHeaderSize + wholeBytes); err != nil {
			return CaptureFormat{}, 0, recoveryOperationFailure(err)
		}
		if err := file.Sync(); err != nil {
			return CaptureFormat{}, 0, recoveryOperationFailure(err)
		}
	}
	frames = uint64(wholeBytes / bytesPerFrame)
	minimumFrames := uint64(format.SampleRate) * minimumMillis / 1000
	if frames < minimumFrames {
		return format, frames, fmt.Errorf("partial has %d frames, minimum %d", frames, minimumFrames)
	}
	if wholeBytes > int64(^uint32(0)-36) {
		return format, frames, fmt.Errorf("partial exceeds RIFF32 bounds")
	}
	if _, err := file.Seek(0, 0); err != nil {
		return format, frames, recoveryOperationFailure(err)
	}
	if _, err := file.Write(streamingWAVHeader(format, uint32(wholeBytes))); err != nil {
		return format, frames, recoveryOperationFailure(err)
	}
	if err := file.Sync(); err != nil {
		return format, frames, recoveryOperationFailure(err)
	}
	return format, frames, nil
}

func parseStreamingWAVHeader(header []byte) (CaptureFormat, error) {
	if len(header) != ProbeWAVHeaderSize || string(header[0:4]) != "RIFF" || string(header[8:12]) != "WAVE" ||
		string(header[12:16]) != "fmt " || binary.LittleEndian.Uint32(header[16:20]) != 16 ||
		binary.LittleEndian.Uint16(header[20:22]) != 3 || string(header[36:40]) != "data" {
		return CaptureFormat{}, fmt.Errorf("invalid streaming WAV structure")
	}
	channels := uint32(binary.LittleEndian.Uint16(header[22:24]))
	rate := binary.LittleEndian.Uint32(header[24:28])
	byteRate := binary.LittleEndian.Uint32(header[28:32])
	blockAlign := uint32(binary.LittleEndian.Uint16(header[32:34]))
	bits := uint32(binary.LittleEndian.Uint16(header[34:36]))
	if channels == 0 || channels > 8 || rate == 0 || rate > 384000 || bits != 32 ||
		blockAlign != channels*4 || byteRate != rate*blockAlign {
		return CaptureFormat{}, fmt.Errorf("invalid streaming WAV format")
	}
	format := NewCaptureFormat()
	format.Valid = 1
	format.Ready = 1
	format.SampleRate = rate
	format.Channels = channels
	format.BitsPerSample = 32
	format.ValidBits = 32
	format.NativeSubtype = 3
	format.NativeBits = 32
	format.NativeValidBits = 32
	format.BlockAlign = blockAlign
	return format, nil
}

func discardRecovered(fileSystem artifactFileSystem, partial, sidecar string, outcome RecoveryOutcome) RecoveryOutcome {
	if err := removePathsAndVerifyAbsent(fileSystem, partial, sidecar); err != nil {
		outcome.Result = ResultFail
		outcome.Cause = errors.Join(errors.New(outcome.Cause), err).Error()
	} else {
		outcome.Path = ""
	}
	return outcome
}

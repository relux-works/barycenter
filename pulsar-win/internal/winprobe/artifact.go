package winprobe

import (
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const ProbeWAVHeaderSize = 44

type ArtifactWriter struct {
	sessionID  string
	partial    string
	final      string
	sidecar    string
	file       artifactFile
	fs         artifactFileSystem
	format     CaptureFormat
	frames     uint64
	nextSync   uint64
	writeErr   error
	owned      map[string]os.FileInfo
	ownedOrder []string
}

type PromotionContext struct {
	Permission             PermissionStatus
	PermissionMonitorReady bool
}

func NewArtifactWriter(dir, sessionID string, format CaptureFormat) (*ArtifactWriter, error) {
	return newArtifactWriter(productionArtifactFS, dir, sessionID, format)
}

func newArtifactWriter(fileSystem artifactFileSystem, dir, sessionID string, format CaptureFormat) (*ArtifactWriter, error) {
	if sessionID == "" || sessionID == "." || sessionID == ".." || strings.ContainsAny(sessionID, `/\`) ||
		strings.ContainsRune(sessionID, 0) || format.Valid != 1 || format.SampleRate == 0 ||
		format.Channels == 0 || format.Channels > 8 {
		return nil, fmt.Errorf("invalid evidence artifact identity or format")
	}
	if err := fileSystem.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	base := filepath.Join(dir, sessionID)
	w := &ArtifactWriter{
		sessionID: sessionID,
		partial:   base + ".partial",
		final:     base + ".wav",
		sidecar:   base + ".partial.reason",
		fs:        fileSystem,
		format:    format,
		nextSync:  uint64(format.SampleRate),
		owned:     make(map[string]os.FileInfo, 4),
	}
	f, err := w.claimExclusiveFile(w.partial, os.O_RDWR)
	if err != nil {
		return nil, err
	}
	w.file = f
	if _, err := f.Write(streamingWAVHeader(format, 0)); err != nil {
		closeErr := f.Close()
		w.file = nil
		cleanupErr := w.cleanupAllOwned()
		return nil, errors.Join(err, closeErr, cleanupErr)
	}
	return w, nil
}

func (w *ArtifactWriter) claimExclusiveFile(path string, access int) (artifactFile, error) {
	f, err := w.fs.OpenFile(path, os.O_CREATE|os.O_EXCL|access, 0o600)
	if err != nil {
		return nil, err
	}
	identity, statErr := f.Stat()
	if statErr != nil {
		closeErr := f.Close()
		return nil, errors.Join(
			fmt.Errorf("identify claimed artifact path %s: %w", path, statErr),
			closeErr,
			fmt.Errorf("claimed artifact path %s retained because ownership identity is unavailable for safe rollback", path),
		)
	}
	w.rememberOwned(path, identity)
	return f, nil
}

func (w *ArtifactWriter) rememberOwned(path string, identity os.FileInfo) {
	if _, exists := w.owned[path]; !exists {
		w.ownedOrder = append(w.ownedOrder, path)
	}
	w.owned[path] = identity
}

func (w *ArtifactWriter) forgetOwned(path string) {
	delete(w.owned, path)
}

func (w *ArtifactWriter) verifyOwnedPath(path string) error {
	identity, owned := w.owned[path]
	if !owned {
		return fmt.Errorf("artifact path is not owned: %s", path)
	}
	current, err := w.fs.Stat(path)
	if err != nil {
		return fmt.Errorf("verify artifact ownership %s: %w", path, err)
	}
	if identity == nil || !os.SameFile(identity, current) {
		return fmt.Errorf("artifact path identity changed: %s", path)
	}
	return nil
}

func (w *ArtifactWriter) renameOwnedNoReplace(oldPath, newPath string) error {
	identity, owned := w.owned[oldPath]
	if !owned {
		return fmt.Errorf("cannot rename unowned artifact path %s", oldPath)
	}
	if _, collision := w.owned[newPath]; collision {
		return fmt.Errorf("cannot rename artifact onto another owned path %s", newPath)
	}
	if err := w.verifyOwnedPath(oldPath); err != nil {
		return err
	}
	if err := w.fs.RenameNoReplace(oldPath, newPath); err != nil {
		return err
	}
	delete(w.owned, oldPath)
	w.owned[newPath] = identity
	for index, path := range w.ownedOrder {
		if path == oldPath {
			w.ownedOrder[index] = newPath
			break
		}
	}
	return w.verifyOwnedPath(newPath)
}

func (w *ArtifactWriter) cleanupAllOwned() error {
	return w.cleanupAllOwnedExcept()
}

func (w *ArtifactWriter) cleanupAllOwnedExcept(excluded ...string) error {
	var failures []error
	skip := make(map[string]struct{}, len(excluded))
	for _, path := range excluded {
		skip[path] = struct{}{}
	}
	for _, path := range w.ownedOrder {
		if _, excluded := skip[path]; excluded {
			continue
		}
		identity, owned := w.owned[path]
		if !owned {
			continue
		}
		if err := removeOwnedAndVerifyAbsent(w.fs, path, identity); err != nil {
			failures = append(failures, err)
			continue
		}
		delete(w.owned, path)
	}
	return errors.Join(failures...)
}

func (w *ArtifactWriter) WriteFrames(samples []float32, frames uint32) error {
	return w.writeFrames(samples, frames, true)
}

// WriteBufferedFramesWithoutSync appends already-buffered capture data to the
// owned partial without calling File.Sync. It is reserved for the confirmed
// WM_ENDSESSION handoff, where Windows owns the remaining process lifetime and
// startup recovery owns the still-open partial artifact.
func (w *ArtifactWriter) WriteBufferedFramesWithoutSync(samples []float32, frames uint32) error {
	return w.writeFrames(samples, frames, false)
}

func (w *ArtifactWriter) writeFrames(samples []float32, frames uint32, periodicSync bool) error {
	if w.file == nil {
		return fmt.Errorf("artifact writer is closed")
	}
	if w.writeErr != nil {
		return w.writeErr
	}
	want := uint64(frames) * uint64(w.format.Channels)
	if want != uint64(len(samples)) {
		return fmt.Errorf("samples=%d does not match frames=%d channels=%d", len(samples), frames, w.format.Channels)
	}
	if want > uint64(^uint(0)>>1)/4 {
		return fmt.Errorf("sample byte count overflow")
	}
	raw := make([]byte, len(samples)*4)
	for i, sample := range samples {
		binary.LittleEndian.PutUint32(raw[i*4:], math.Float32bits(sample))
	}
	if _, err := w.file.Write(raw); err != nil {
		w.writeErr = fmt.Errorf("write evidence frames: %w", err)
		return w.writeErr
	}
	w.frames += uint64(frames)
	if periodicSync && w.frames >= w.nextSync {
		if err := w.syncNow("periodic durable sync"); err != nil {
			return err
		}
		rate := uint64(w.format.SampleRate)
		w.nextSync = (w.frames/rate + 1) * rate
	}
	return nil
}

func (w *ArtifactWriter) Sync() error {
	if w.file == nil {
		return nil
	}
	return w.syncNow("durable sync")
}

func (w *ArtifactWriter) syncNow(action string) error {
	if err := w.file.Sync(); err != nil {
		w.writeErr = fmt.Errorf("%s at frame %d: %w", action, w.frames, err)
		return w.writeErr
	}
	return nil
}

func (w *ArtifactWriter) Frames() uint64 { return w.frames }

// Abort removes every artifact owned by this writer without creating a
// sidecar. It is used when CaptureGetResult itself fails, because no terminal
// reason can be trusted or promoted in that state.
func (w *ArtifactWriter) Abort() error {
	var failures []error
	if w.file != nil {
		if err := w.file.Close(); err != nil {
			failures = append(failures, fmt.Errorf("close artifact before abort: %w", err))
		}
		w.file = nil
	}
	if err := w.cleanupAllOwned(); err != nil {
		failures = append(failures, err)
	}
	return errors.Join(failures...)
}

func (w *ArtifactWriter) Finalize(reason CaptureReason, hr HResult, promotion PromotionContext, minFrames uint64) (string, ProbeResult, error) {
	if w.file == nil {
		return "", ResultFail, fmt.Errorf("artifact writer is closed")
	}
	if w.writeErr != nil {
		return w.discard(w.writeErr)
	}
	// Do not durably record a promotable reason when the revoke monitor gate
	// was unavailable. A crash between sidecar creation and deletion could
	// otherwise let startup recovery promote evidence from an unmonitored
	// session. With no sidecar, recovery deterministically discards it.
	if PromotableReason(reason) && promotion.Permission == PermissionAllowed &&
		w.frames >= minFrames && !promotion.PermissionMonitorReady {
		return w.discardAs(ResultBlocked, fmt.Errorf("permission AccessChanged monitor unavailable; artifact promotion blocked"))
	}
	record := ReasonRecord{Version: SidecarVersion, SessionID: w.sessionID, Reason: reason, ReasonName: reason.String(), HResult: hr, TimestampMS: time.Now().UnixMilli()}
	if err := record.Validate(w.sessionID); err != nil {
		return w.discard(fmt.Errorf("invalid terminal record: %w", err))
	}
	var sidecarErr error
	if err := w.writeReasonRecordAtomic(record); err != nil {
		sidecarErr = err
		// Continue normal in-process finalization, but preserve the write error in
		// the structured outcome; crash recovery will discard a missing sidecar.
		if !PromotableReason(reason) || promotion.Permission != PermissionAllowed ||
			w.frames < minFrames {
			return w.discard(err)
		}
	}
	if !PromotableReason(reason) || promotion.Permission != PermissionAllowed || w.frames < minFrames {
		return w.discard(nil)
	}
	dataBytes := w.frames * uint64(w.format.Channels) * 4
	if dataBytes > math.MaxUint32-36 {
		return w.discard(fmt.Errorf("probe WAV exceeds RIFF32 bounds"))
	}
	if _, err := w.file.Seek(0, 0); err != nil {
		return w.discard(err)
	}
	if _, err := w.file.Write(streamingWAVHeader(w.format, uint32(dataBytes))); err != nil {
		return w.discard(err)
	}
	if err := w.file.Sync(); err != nil {
		return w.discard(err)
	}
	if err := w.file.Close(); err != nil {
		w.file = nil
		cleanupErr := w.cleanupAllOwned()
		return "", ResultFail, errors.Join(fmt.Errorf("close finalized artifact: %w", err), sidecarErr, cleanupErr)
	}
	w.file = nil
	if err := w.renameOwnedNoReplace(w.partial, w.final); err != nil {
		cleanupErr := w.cleanupAllOwned()
		return "", ResultFail, errors.Join(fmt.Errorf("promote artifact: %w", err), sidecarErr, cleanupErr)
	}
	if _, _, err := verifyWAVFile(w.fs, w.final); err != nil {
		cleanupErr := w.cleanupAllOwned()
		return "", ResultFail, errors.Join(fmt.Errorf("verify finalized WAV: %w", err), sidecarErr, cleanupErr)
	}
	if err := w.verifyOwnedPath(w.final); err != nil {
		cleanupErr := w.cleanupAllOwned()
		return "", ResultFail, errors.Join(fmt.Errorf("verify finalized WAV ownership: %w", err), sidecarErr, cleanupErr)
	}
	// A verified final is now deliberate output, not cleanup inventory. Keep it
	// even if later sidecar cleanup prevents this run from reporting a pass.
	w.forgetOwned(w.final)
	cleanupErr := w.cleanupAllOwned()
	if err := errors.Join(sidecarErr, cleanupErr); err != nil {
		return w.final, ResultFail, err
	}
	return w.final, ResultPass, nil
}

func (w *ArtifactWriter) discard(cause error) (string, ProbeResult, error) {
	result := ResultDiscard
	if cause != nil {
		result = ResultFail
	}
	return w.discardAs(result, cause)
}

func (w *ArtifactWriter) discardAs(result ProbeResult, cause error) (string, ProbeResult, error) {
	var cleanupFailures []error
	if w.file != nil {
		if err := w.file.Close(); err != nil {
			cleanupFailures = append(cleanupFailures, fmt.Errorf("close artifact before discard: %w", err))
		}
		w.file = nil
	}
	if err := w.cleanupAllOwned(); err != nil {
		cleanupFailures = append(cleanupFailures, err)
	}
	cleanupErr := errors.Join(cleanupFailures...)
	if cleanupErr != nil {
		result = ResultFail
	}
	return "", result, errors.Join(cause, cleanupErr)
}

// VerifyWAVFile is the strict local post-write gate used for every normal and
// recovered artifact before a pass is reported. It validates both the header
// and the actual on-disk length; a syntactically plausible header with missing
// or trailing bytes is rejected.
func VerifyWAVFile(path string) (CaptureFormat, uint64, error) {
	return verifyWAVFile(productionArtifactFS, path)
}

func verifyWAVFile(fileSystem artifactFileSystem, path string) (format CaptureFormat, frames uint64, resultErr error) {
	file, err := fileSystem.Open(path)
	if err != nil {
		return CaptureFormat{}, 0, err
	}
	defer func() {
		if err := file.Close(); err != nil {
			resultErr = errors.Join(resultErr, fmt.Errorf("close verified WAV: %w", err))
		}
	}()
	header := make([]byte, ProbeWAVHeaderSize)
	if _, err := io.ReadFull(file, header); err != nil {
		return CaptureFormat{}, 0, fmt.Errorf("read WAV header: %w", err)
	}
	format, err = parseStreamingWAVHeader(header)
	if err != nil {
		return CaptureFormat{}, 0, err
	}
	stat, err := file.Stat()
	if err != nil {
		return CaptureFormat{}, 0, err
	}
	dataBytes := uint64(binary.LittleEndian.Uint32(header[40:44]))
	riffBytes := uint64(binary.LittleEndian.Uint32(header[4:8]))
	if dataBytes == 0 || riffBytes != dataBytes+36 {
		return CaptureFormat{}, 0, fmt.Errorf("invalid finalized RIFF/data sizes %d/%d", riffBytes, dataBytes)
	}
	if uint64(stat.Size()) != uint64(ProbeWAVHeaderSize)+dataBytes {
		return CaptureFormat{}, 0, fmt.Errorf("file size %d does not match header data size %d", stat.Size(), dataBytes)
	}
	bytesPerFrame := uint64(format.Channels) * 4
	if dataBytes%bytesPerFrame != 0 {
		return CaptureFormat{}, 0, fmt.Errorf("data size %d is not a whole frame", dataBytes)
	}
	return format, dataBytes / bytesPerFrame, nil
}

func (w *ArtifactWriter) writeReasonRecordAtomic(record ReasonRecord) error {
	raw, err := json.Marshal(record)
	if err != nil {
		return err
	}
	tmp := w.sidecar + ".tmp"
	f, err := w.claimExclusiveFile(tmp, os.O_WRONLY)
	if err != nil {
		return err
	}
	var failures []error
	if _, writeErr := f.Write(raw); writeErr != nil {
		failures = append(failures, fmt.Errorf("write sidecar temp: %w", writeErr))
	} else if syncErr := f.Sync(); syncErr != nil {
		failures = append(failures, fmt.Errorf("sync sidecar temp: %w", syncErr))
	}
	closeErr := f.Close()
	if closeErr != nil {
		failures = append(failures, fmt.Errorf("close sidecar temp: %w", closeErr))
	}
	if len(failures) != 0 {
		failures = append(failures, w.cleanupAllOwnedExcept(w.partial))
		return errors.Join(failures...)
	}
	if err := w.renameOwnedNoReplace(tmp, w.sidecar); err != nil {
		return errors.Join(fmt.Errorf("install sidecar without replacement: %w", err), w.cleanupAllOwnedExcept(w.partial))
	}
	return nil
}

func streamingWAVHeader(format CaptureFormat, dataBytes uint32) []byte {
	header := make([]byte, ProbeWAVHeaderSize)
	copy(header[0:4], "RIFF")
	if dataBytes != 0 {
		binary.LittleEndian.PutUint32(header[4:8], dataBytes+36)
	}
	copy(header[8:12], "WAVE")
	copy(header[12:16], "fmt ")
	binary.LittleEndian.PutUint32(header[16:20], 16)
	binary.LittleEndian.PutUint16(header[20:22], 3) // IEEE float
	binary.LittleEndian.PutUint16(header[22:24], uint16(format.Channels))
	binary.LittleEndian.PutUint32(header[24:28], format.SampleRate)
	blockAlign := uint16(format.Channels * 4)
	binary.LittleEndian.PutUint32(header[28:32], format.SampleRate*uint32(blockAlign))
	binary.LittleEndian.PutUint16(header[32:34], blockAlign)
	binary.LittleEndian.PutUint16(header[34:36], 32)
	copy(header[36:40], "data")
	binary.LittleEndian.PutUint32(header[40:44], dataBytes)
	return header
}

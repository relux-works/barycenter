// Package pmcprobe is a repository-only protected-media container experiment.
// It is not a production format and must not be registered as e2ee_media_v1.
package pmcprobe

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hkdf"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"fmt"
	"math"
)

const (
	HeaderSize         = 144
	ManifestTagSize    = 16
	MaximumChunkBytes  = 1 << 20
	Version            = uint16(1)
	SuiteAES256GCMHKDF = uint16(1)
)

var (
	magic                  = [8]byte{'P', 'M', 'C', 'P', 'R', 'B', '0', '1'}
	ErrMalformedContainer  = errors.New("malformed protected-media probe container")
	ErrAuthentication      = errors.New("protected-media probe authentication failed")
	ErrUnsupportedContract = errors.New("unsupported protected-media probe contract")
)

type MediaKind byte

const (
	MediaClip MediaKind = iota + 1
	MediaTrack
	MediaSavedCue
)

type Config struct {
	Kind                 MediaKind
	ChunkSize            uint32
	DurationMS           uint64
	Epoch                uint64
	ContainerID          [16]byte
	TargetSnapshotDigest [32]byte
	Salt                 [32]byte
	NoncePrefix          [4]byte
}

type Header struct {
	Kind                 MediaKind
	ChunkSize            uint32
	ChunkCount           uint32
	PlaintextSize        uint64
	DurationMS           uint64
	Epoch                uint64
	ContainerID          [16]byte
	TargetSnapshotDigest [32]byte
	Salt                 [32]byte
	NoncePrefix          [4]byte
	PrivatePlainSize     uint32
	PrivateCipherSize    uint32
}

type ByteRange struct {
	Offset int64
	Length int64
}

func validateConfig(config Config, plaintextSize int, privateSize int) error {
	if config.Kind < MediaClip || config.Kind > MediaSavedCue || config.ChunkSize == 0 || config.ChunkSize > MaximumChunkBytes {
		return ErrUnsupportedContract
	}
	if plaintextSize <= 0 || privateSize < 0 || privateSize > math.MaxUint32-ManifestTagSize {
		return ErrUnsupportedContract
	}
	chunkCount := (uint64(plaintextSize) + uint64(config.ChunkSize) - 1) / uint64(config.ChunkSize)
	if chunkCount == 0 || chunkCount > math.MaxUint32 {
		return ErrUnsupportedContract
	}
	return nil
}

func headerFor(config Config, plaintextSize int, privateSize int) Header {
	return Header{
		Kind:                 config.Kind,
		ChunkSize:            config.ChunkSize,
		ChunkCount:           uint32((uint64(plaintextSize) + uint64(config.ChunkSize) - 1) / uint64(config.ChunkSize)),
		PlaintextSize:        uint64(plaintextSize),
		DurationMS:           config.DurationMS,
		Epoch:                config.Epoch,
		ContainerID:          config.ContainerID,
		TargetSnapshotDigest: config.TargetSnapshotDigest,
		Salt:                 config.Salt,
		NoncePrefix:          config.NoncePrefix,
		PrivatePlainSize:     uint32(privateSize),
		PrivateCipherSize:    uint32(privateSize + ManifestTagSize),
	}
}

func encodeHeader(header Header) []byte {
	raw := make([]byte, HeaderSize)
	copy(raw[0:8], magic[:])
	binary.BigEndian.PutUint16(raw[8:10], Version)
	binary.BigEndian.PutUint16(raw[10:12], SuiteAES256GCMHKDF)
	raw[12] = byte(header.Kind)
	binary.BigEndian.PutUint32(raw[16:20], header.ChunkSize)
	binary.BigEndian.PutUint32(raw[20:24], header.ChunkCount)
	binary.BigEndian.PutUint64(raw[24:32], header.PlaintextSize)
	binary.BigEndian.PutUint64(raw[32:40], header.DurationMS)
	binary.BigEndian.PutUint64(raw[40:48], header.Epoch)
	copy(raw[48:64], header.ContainerID[:])
	copy(raw[64:96], header.TargetSnapshotDigest[:])
	copy(raw[96:128], header.Salt[:])
	copy(raw[128:132], header.NoncePrefix[:])
	binary.BigEndian.PutUint32(raw[132:136], header.PrivatePlainSize)
	binary.BigEndian.PutUint32(raw[136:140], header.PrivateCipherSize)
	return raw
}

func DecodeHeader(container []byte) (Header, error) {
	if len(container) < HeaderSize || string(container[0:8]) != string(magic[:]) {
		return Header{}, ErrMalformedContainer
	}
	if binary.BigEndian.Uint16(container[8:10]) != Version || binary.BigEndian.Uint16(container[10:12]) != SuiteAES256GCMHKDF {
		return Header{}, ErrUnsupportedContract
	}
	for _, value := range append(append([]byte(nil), container[13:16]...), container[140:144]...) {
		if value != 0 {
			return Header{}, ErrMalformedContainer
		}
	}
	var header Header
	header.Kind = MediaKind(container[12])
	header.ChunkSize = binary.BigEndian.Uint32(container[16:20])
	header.ChunkCount = binary.BigEndian.Uint32(container[20:24])
	header.PlaintextSize = binary.BigEndian.Uint64(container[24:32])
	header.DurationMS = binary.BigEndian.Uint64(container[32:40])
	header.Epoch = binary.BigEndian.Uint64(container[40:48])
	copy(header.ContainerID[:], container[48:64])
	copy(header.TargetSnapshotDigest[:], container[64:96])
	copy(header.Salt[:], container[96:128])
	copy(header.NoncePrefix[:], container[128:132])
	header.PrivatePlainSize = binary.BigEndian.Uint32(container[132:136])
	header.PrivateCipherSize = binary.BigEndian.Uint32(container[136:140])
	if header.Kind < MediaClip || header.Kind > MediaSavedCue || header.ChunkSize == 0 || header.ChunkSize > MaximumChunkBytes || header.ChunkCount == 0 || header.PlaintextSize == 0 {
		return Header{}, ErrMalformedContainer
	}
	expectedChunks := (header.PlaintextSize + uint64(header.ChunkSize) - 1) / uint64(header.ChunkSize)
	if expectedChunks != uint64(header.ChunkCount) || header.PrivateCipherSize != header.PrivatePlainSize+ManifestTagSize {
		return Header{}, ErrMalformedContainer
	}
	if expectedContainerSize(header) != uint64(len(container)) {
		return Header{}, ErrMalformedContainer
	}
	return header, nil
}

func expectedContainerSize(header Header) uint64 {
	return HeaderSize + ManifestTagSize + uint64(header.PrivateCipherSize) + header.PlaintextSize + uint64(header.ChunkCount)*ManifestTagSize
}

func key(master []byte, salt [32]byte, domain string) ([]byte, error) {
	if len(master) != 32 {
		return nil, ErrUnsupportedContract
	}
	return hkdf.Key(sha256.New, master, salt[:], domain, 32)
}

func gcm(key []byte) (cipher.AEAD, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(block)
}

func nonce(prefix [4]byte, counter uint64) []byte {
	result := make([]byte, 12)
	copy(result, prefix[:])
	binary.BigEndian.PutUint64(result[4:], counter)
	return result
}

func aad(domain string, headerHash [32]byte, index uint32, plaintextSize uint32) []byte {
	result := make([]byte, 0, len(domain)+32+8)
	result = append(result, domain...)
	result = append(result, headerHash[:]...)
	trailer := make([]byte, 8)
	binary.BigEndian.PutUint32(trailer[0:4], index)
	binary.BigEndian.PutUint32(trailer[4:8], plaintextSize)
	return append(result, trailer...)
}

func Seal(config Config, masterKey, privateManifest, plaintext []byte) ([]byte, error) {
	if err := validateConfig(config, len(plaintext), len(privateManifest)); err != nil {
		return nil, err
	}
	header := headerFor(config, len(plaintext), len(privateManifest))
	headerBytes := encodeHeader(header)
	headerHash := sha256.Sum256(headerBytes)

	manifestKey, err := key(masterKey, header.Salt, "barycenter/pmc-probe/v1/manifest")
	if err != nil {
		return nil, err
	}
	privateKey, err := key(masterKey, header.Salt, "barycenter/pmc-probe/v1/private-manifest")
	if err != nil {
		return nil, err
	}
	chunkKey, err := key(masterKey, header.Salt, "barycenter/pmc-probe/v1/chunks")
	if err != nil {
		return nil, err
	}
	manifestAEAD, err := gcm(manifestKey)
	if err != nil {
		return nil, err
	}
	privateAEAD, err := gcm(privateKey)
	if err != nil {
		return nil, err
	}
	chunkAEAD, err := gcm(chunkKey)
	if err != nil {
		return nil, err
	}

	result := make([]byte, 0, expectedContainerSize(header))
	result = append(result, headerBytes...)
	result = manifestAEAD.Seal(result, nonce(header.NoncePrefix, math.MaxUint64), nil, headerBytes)
	result = privateAEAD.Seal(result, nonce(header.NoncePrefix, math.MaxUint64-1), privateManifest,
		aad("barycenter/pmc-probe/v1/private-manifest", headerHash, math.MaxUint32, uint32(len(privateManifest))))
	for index := uint32(0); index < header.ChunkCount; index++ {
		start := uint64(index) * uint64(header.ChunkSize)
		end := min(start+uint64(header.ChunkSize), uint64(len(plaintext)))
		chunk := plaintext[start:end]
		result = chunkAEAD.Seal(result, nonce(header.NoncePrefix, uint64(index)), chunk,
			aad("barycenter/pmc-probe/v1/chunk", headerHash, index, uint32(len(chunk))))
	}
	return result, nil
}

func authenticateManifest(container, masterKey []byte, header Header) ([32]byte, error) {
	headerBytes := container[:HeaderSize]
	headerHash := sha256.Sum256(headerBytes)
	manifestKey, err := key(masterKey, header.Salt, "barycenter/pmc-probe/v1/manifest")
	if err != nil {
		return [32]byte{}, err
	}
	manifestAEAD, err := gcm(manifestKey)
	if err != nil {
		return [32]byte{}, err
	}
	if _, err := manifestAEAD.Open(nil, nonce(header.NoncePrefix, math.MaxUint64),
		container[HeaderSize:HeaderSize+ManifestTagSize], headerBytes); err != nil {
		return [32]byte{}, fmt.Errorf("%w: manifest", ErrAuthentication)
	}
	return headerHash, nil
}

func OpenPrivateManifest(container, masterKey []byte) ([]byte, error) {
	header, err := DecodeHeader(container)
	if err != nil {
		return nil, err
	}
	headerHash, err := authenticateManifest(container, masterKey, header)
	if err != nil {
		return nil, err
	}
	return openPrivateManifest(container, masterKey, header, headerHash)
}

func openPrivateManifest(container, masterKey []byte, header Header, headerHash [32]byte) ([]byte, error) {
	privateKey, err := key(masterKey, header.Salt, "barycenter/pmc-probe/v1/private-manifest")
	if err != nil {
		return nil, err
	}
	privateAEAD, err := gcm(privateKey)
	if err != nil {
		return nil, err
	}
	start := HeaderSize + ManifestTagSize
	end := start + int(header.PrivateCipherSize)
	plaintext, err := privateAEAD.Open(nil, nonce(header.NoncePrefix, math.MaxUint64-1), container[start:end],
		aad("barycenter/pmc-probe/v1/private-manifest", headerHash, math.MaxUint32, header.PrivatePlainSize))
	if err != nil {
		return nil, fmt.Errorf("%w: private manifest", ErrAuthentication)
	}
	return plaintext, nil
}

func ChunkRange(header Header, index uint32) (ByteRange, error) {
	if index >= header.ChunkCount {
		return ByteRange{}, ErrMalformedContainer
	}
	prefix := uint64(HeaderSize+ManifestTagSize) + uint64(header.PrivateCipherSize)
	offset := prefix + uint64(index)*(uint64(header.ChunkSize)+ManifestTagSize)
	plainLength := uint64(header.ChunkSize)
	if index == header.ChunkCount-1 {
		plainLength = header.PlaintextSize - uint64(index)*uint64(header.ChunkSize)
	}
	return ByteRange{Offset: int64(offset), Length: int64(plainLength + ManifestTagSize)}, nil
}

func OpenChunk(container, masterKey []byte, index uint32) ([]byte, error) {
	header, err := DecodeHeader(container)
	if err != nil {
		return nil, err
	}
	headerHash, err := authenticateManifest(container, masterKey, header)
	if err != nil {
		return nil, err
	}
	if _, err := openPrivateManifest(container, masterKey, header, headerHash); err != nil {
		return nil, err
	}
	byteRange, err := ChunkRange(header, index)
	if err != nil {
		return nil, err
	}
	chunkKey, err := key(masterKey, header.Salt, "barycenter/pmc-probe/v1/chunks")
	if err != nil {
		return nil, err
	}
	chunkAEAD, err := gcm(chunkKey)
	if err != nil {
		return nil, err
	}
	start, end := int(byteRange.Offset), int(byteRange.Offset+byteRange.Length)
	plainLength := uint32(byteRange.Length - ManifestTagSize)
	plaintext, err := chunkAEAD.Open(nil, nonce(header.NoncePrefix, uint64(index)), container[start:end],
		aad("barycenter/pmc-probe/v1/chunk", headerHash, index, plainLength))
	if err != nil {
		return nil, fmt.Errorf("%w: chunk %d", ErrAuthentication, index)
	}
	return plaintext, nil
}

func OpenAll(container, masterKey []byte) ([]byte, error) {
	header, err := DecodeHeader(container)
	if err != nil {
		return nil, err
	}
	result := make([]byte, 0, header.PlaintextSize)
	for index := uint32(0); index < header.ChunkCount; index++ {
		chunk, err := OpenChunk(container, masterKey, index)
		if err != nil {
			return nil, err
		}
		result = append(result, chunk...)
	}
	return result, nil
}

// ResumeBoundary returns the greatest complete authenticated-record boundary
// not after uploadedBytes. A partial record must be uploaded again verbatim.
func ResumeBoundary(container []byte, uploadedBytes int64) (int64, error) {
	header, err := DecodeHeader(container)
	if err != nil {
		return 0, err
	}
	if uploadedBytes <= 0 {
		return 0, nil
	}
	prefix := int64(HeaderSize+ManifestTagSize) + int64(header.PrivateCipherSize)
	if uploadedBytes < prefix {
		return 0, nil
	}
	boundary := prefix
	for index := uint32(0); index < header.ChunkCount; index++ {
		byteRange, _ := ChunkRange(header, index)
		end := byteRange.Offset + byteRange.Length
		if end > uploadedBytes {
			break
		}
		boundary = end
	}
	return boundary, nil
}

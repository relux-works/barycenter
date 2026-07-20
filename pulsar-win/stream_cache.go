package main

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	protocol "relux.works/duet/pulsar-win/wire"
)

const (
	windowsStreamCacheGlobalBytes     int64 = 512 << 20
	windowsStreamCachePerVariantBytes int64 = 64 << 20
	windowsStreamCachePinnedBytes     int64 = 128 << 20
	windowsStreamMaximumChunkBytes    int64 = 1 << 20
	windowsStreamMaximumNetworkBytes  int64 = 1 << 20
	windowsStreamSeekPointSpacingMS   int64 = 10_000
)

type WindowsStreamChunk struct {
	Index      int
	Start, End int64
	SHA256     string
}

type WindowsStreamSeekPoint struct {
	TimeMS, Offset int64
}

// WindowsStreamManifest is resolved trusted metadata for the opaque svm1
// identity in stream_load. It remains candidate-only while the codec ADR is
// no-go; production code neither resolves nor registers one.
type WindowsStreamManifest struct {
	Identity, VariantURL, ETag, SHA256 string
	SizeBytes, DurationMS              int64
	Chunks                             []WindowsStreamChunk
	SeekMap                            []WindowsStreamSeekPoint
}

func validateWindowsStreamManifest(load protocol.StreamLoadPayload, manifest WindowsStreamManifest) error {
	if protocol.ValidateStreamLoadPayload(load) != nil || manifest.Identity != load.VariantManifest ||
		manifest.VariantURL != load.VariantURL || manifest.ETag != load.VariantETag ||
		manifest.SHA256 != load.VariantSHA256 || manifest.SizeBytes != load.VariantSizeBytes ||
		manifest.DurationMS <= 0 || load.StartPositionMS > manifest.DurationMS {
		return windowsStreamFailure("manifest", "invalid_manifest")
	}
	return validateWindowsStreamManifestShape(manifest)
}

func validateWindowsStreamManifestShape(manifest WindowsStreamManifest) error {
	if !strings.HasPrefix(manifest.Identity, "svm1.") || len(manifest.Identity) > 512 ||
		strings.Trim(manifest.Identity, "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789._-") != "" ||
		!strings.HasPrefix(manifest.VariantURL, "/v1/media/") ||
		strings.ContainsAny(manifest.VariantURL, "?#@") || strings.Contains(manifest.VariantURL, "://") ||
		manifest.ETag != `"sha256-`+manifest.SHA256+`"` || !validLowerSHA256(manifest.SHA256) ||
		manifest.SizeBytes <= 0 || manifest.DurationMS <= 0 || len(manifest.Chunks) == 0 ||
		len(manifest.SeekMap) == 0 {
		return windowsStreamFailure("manifest", "invalid_manifest")
	}
	var next int64
	chunkStarts := make(map[int64]bool, len(manifest.Chunks))
	for index, chunk := range manifest.Chunks {
		if chunk.Index != index || chunk.Start != next || chunk.End < chunk.Start ||
			chunk.End-chunk.Start+1 > windowsStreamMaximumChunkBytes || !validLowerSHA256(chunk.SHA256) {
			return windowsStreamFailure("manifest", "invalid_manifest")
		}
		chunkStarts[chunk.Start] = true
		next = chunk.End + 1
	}
	if next != manifest.SizeBytes {
		return windowsStreamFailure("manifest", "invalid_manifest")
	}
	for index, point := range manifest.SeekMap {
		if point.TimeMS < 0 || point.TimeMS > manifest.DurationMS || !chunkStarts[point.Offset] ||
			(index == 0 && (point.TimeMS != 0 || point.Offset != 0)) ||
			(index > 0 && (point.TimeMS <= manifest.SeekMap[index-1].TimeMS ||
				point.Offset < manifest.SeekMap[index-1].Offset ||
				point.TimeMS-manifest.SeekMap[index-1].TimeMS > windowsStreamSeekPointSpacingMS)) {
			return windowsStreamFailure("manifest", "invalid_manifest")
		}
	}
	return nil
}

func (manifest WindowsStreamManifest) ChunkForTime(positionMS int64) int {
	point := manifest.SeekMap[0]
	for _, candidate := range manifest.SeekMap[1:] {
		if candidate.TimeMS > positionMS {
			break
		}
		point = candidate
	}
	index := sort.Search(len(manifest.Chunks), func(index int) bool {
		return manifest.Chunks[index].Start >= point.Offset
	})
	if index >= len(manifest.Chunks) {
		return len(manifest.Chunks) - 1
	}
	return index
}

type WindowsStreamFailure struct {
	Stage, Code string
}

func (failure *WindowsStreamFailure) Error() string { return failure.Stage + ":" + failure.Code }

func windowsStreamFailure(stage, code string) error {
	return &WindowsStreamFailure{Stage: stage, Code: code}
}

func windowsStreamFailureCode(err error) (string, string) {
	var failure *WindowsStreamFailure
	if errors.As(err, &failure) && validWindowsStreamFailureToken(failure.Stage) &&
		validWindowsStreamFailureToken(failure.Code) {
		return failure.Stage, failure.Code
	}
	return "internal", "internal_error"
}

func validWindowsStreamFailureToken(value string) bool {
	if value == "" || len(value) > 64 {
		return false
	}
	for _, char := range value {
		if (char < 'a' || char > 'z') && (char < '0' || char > '9') && char != '_' {
			return false
		}
	}
	return true
}

type windowsStreamRangeFetcher interface {
	FetchRange(context.Context, string, string, int64, int64) ([]byte, string, error)
}

type windowsStreamCacheLimits struct {
	Global, PerVariant, Pinned int64
	Chunk, Network             int64
}

func defaultWindowsStreamCacheLimits() windowsStreamCacheLimits {
	return windowsStreamCacheLimits{
		Global: windowsStreamCacheGlobalBytes, PerVariant: windowsStreamCachePerVariantBytes,
		Pinned: windowsStreamCachePinnedBytes, Chunk: windowsStreamMaximumChunkBytes,
		Network: windowsStreamMaximumNetworkBytes,
	}
}

type windowsStreamCacheEntry struct {
	Key, VariantKey string
	ChunkIndex      int
	Size, LastUse   int64
	Pinned          bool `json:"-"`
}

type windowsStreamCacheIndex struct {
	Version    int                       `json:"version"`
	Entries    []windowsStreamCacheEntry `json:"entries"`
	Tombstones []string                  `json:"tombstones"`
}

type windowsStreamChunkFlight struct{ done chan struct{} }

type windowsStreamProcessLockRegistry struct {
	mu    sync.Mutex
	locks map[string]*sync.Mutex
}

func (registry *windowsStreamProcessLockRegistry) lockFor(path string) *sync.Mutex {
	registry.mu.Lock()
	defer registry.mu.Unlock()
	if registry.locks == nil {
		registry.locks = make(map[string]*sync.Mutex)
	}
	if registry.locks[path] == nil {
		registry.locks[path] = &sync.Mutex{}
	}
	return registry.locks[path]
}

var globalWindowsStreamProcessLocks windowsStreamProcessLockRegistry

type WindowsStreamCacheStats struct {
	Hits, Fetches, Evictions, IntegrityFailures, Repairs int64
	Bytes, PinnedBytes                                   int64
}

// WindowsStreamChunkCache owns candidate range bytes only. Decoder adapters
// receive verified chunks through Get and never own HTTP, credentials or disk.
type WindowsStreamChunkCache struct {
	dir, indexPath string
	secret         []byte
	fetcher        windowsStreamRangeFetcher
	limits         windowsStreamCacheLimits
	processMu      *sync.Mutex

	mu         sync.Mutex
	entries    map[string]*windowsStreamCacheEntry
	tombstones map[string]bool
	inflight   map[string]*windowsStreamChunkFlight
	clock      int64

	hits, fetches, evictions, integrityFailures, repairs atomic.Int64
}

func NewWindowsStreamChunkCache(
	root string,
	installationSecret []byte,
	fetcher windowsStreamRangeFetcher,
) (*WindowsStreamChunkCache, error) {
	return newWindowsStreamChunkCache(root, installationSecret, fetcher, defaultWindowsStreamCacheLimits())
}

func newWindowsStreamChunkCache(
	root string,
	installationSecret []byte,
	fetcher windowsStreamRangeFetcher,
	limits windowsStreamCacheLimits,
) (*WindowsStreamChunkCache, error) {
	if root == "" || len(installationSecret) < 16 || fetcher == nil ||
		limits.Global <= 0 || limits.PerVariant <= 0 || limits.Pinned <= 0 ||
		limits.Chunk <= 0 || limits.Chunk > windowsStreamMaximumChunkBytes ||
		limits.Network <= 0 || limits.Network > windowsStreamMaximumNetworkBytes ||
		limits.PerVariant > limits.Global || limits.Pinned > limits.Global {
		return nil, windowsStreamFailure("cache", "invalid_configuration")
	}
	dir := filepath.Join(root, "stream-v1")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, windowsStreamFailure("cache", "cache_unavailable")
	}
	lockPath, err := filepath.Abs(filepath.Join(dir, "index-v1.json"))
	if err != nil {
		return nil, windowsStreamFailure("cache", "invalid_configuration")
	}
	cache := &WindowsStreamChunkCache{
		dir: dir, indexPath: filepath.Join(dir, "index-v1.json"),
		secret: append([]byte(nil), installationSecret...), fetcher: fetcher, limits: limits,
		entries: make(map[string]*windowsStreamCacheEntry), tombstones: make(map[string]bool),
		inflight:  make(map[string]*windowsStreamChunkFlight),
		processMu: globalWindowsStreamProcessLocks.lockFor(lockPath),
	}
	if err := cache.repair(); err != nil {
		return nil, err
	}
	return cache, nil
}

func (cache *WindowsStreamChunkCache) hmac(parts ...string) string {
	mac := hmac.New(sha256.New, cache.secret)
	for index, part := range parts {
		if index > 0 {
			_, _ = mac.Write([]byte{0})
		}
		_, _ = mac.Write([]byte(part))
	}
	return hex.EncodeToString(mac.Sum(nil))
}

func (cache *WindowsStreamChunkCache) variantKey(manifest WindowsStreamManifest) string {
	// The opaque identity and ETag are content/version labels, not object
	// authority. Include the exact authenticated route so delete/revocation of
	// one object cannot tombstone or reuse another object's equal ciphertext.
	return cache.hmac("variant", manifest.Identity, manifest.VariantURL, manifest.ETag)
}

func (cache *WindowsStreamChunkCache) chunkKey(variantKey string, index int) string {
	return cache.hmac("chunk", variantKey, strconv.Itoa(index))
}

func (cache *WindowsStreamChunkCache) repair() error {
	cache.mu.Lock()
	defer cache.mu.Unlock()
	cache.processMu.Lock()
	defer cache.processMu.Unlock()
	var index windowsStreamCacheIndex
	if raw, err := os.ReadFile(cache.indexPath); err == nil {
		if json.Unmarshal(raw, &index) != nil || index.Version != 1 {
			index = windowsStreamCacheIndex{Version: 1}
			cache.repairs.Add(1)
		}
	}
	for _, value := range index.Tombstones {
		if len(value) == sha256.Size*2 {
			cache.tombstones[value] = true
		}
	}
	owned := make(map[string]bool)
	for _, persisted := range index.Entries {
		path := filepath.Join(cache.dir, persisted.Key+".chunk")
		info, err := os.Lstat(path)
		if err != nil || !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 ||
			info.Size() != persisted.Size || persisted.Size <= 0 || persisted.Size > cache.limits.Chunk ||
			len(persisted.Key) != sha256.Size*2 || len(persisted.VariantKey) != sha256.Size*2 ||
			cache.tombstones[persisted.VariantKey] {
			_ = os.Remove(path)
			cache.repairs.Add(1)
			continue
		}
		copyValue := persisted
		copyValue.Pinned = false
		cache.entries[copyValue.Key] = &copyValue
		owned[copyValue.Key+".chunk"] = true
		if copyValue.LastUse > cache.clock {
			cache.clock = copyValue.LastUse
		}
	}
	items, _ := os.ReadDir(cache.dir)
	for _, item := range items {
		if item.IsDir() || item.Name() == filepath.Base(cache.indexPath) {
			continue
		}
		if strings.HasSuffix(item.Name(), ".part") ||
			(strings.HasSuffix(item.Name(), ".chunk") && !owned[item.Name()]) {
			_ = os.Remove(filepath.Join(cache.dir, item.Name()))
			cache.repairs.Add(1)
		}
	}
	keys := make([]string, 0, len(cache.entries))
	for key := range cache.entries {
		keys = append(keys, key)
	}
	for _, key := range keys {
		if cache.entries[key] != nil {
			if err := cache.enforceLimitsLocked(key); err != nil {
				return err
			}
		}
	}
	if err := cache.enforceLimitsLocked(""); err != nil {
		return err
	}
	return cache.writeIndexProcessLocked()
}

func (cache *WindowsStreamChunkCache) Get(
	ctx context.Context,
	manifest WindowsStreamManifest,
	chunkIndex int,
) ([]byte, error) {
	if validateWindowsStreamManifestShape(manifest) != nil || chunkIndex < 0 || chunkIndex >= len(manifest.Chunks) {
		return nil, windowsStreamFailure("manifest", "invalid_manifest")
	}
	variantKey := cache.variantKey(manifest)
	chunk := manifest.Chunks[chunkIndex]
	key := cache.chunkKey(variantKey, chunkIndex)
	for {
		cache.mu.Lock()
		if cache.tombstones[variantKey] {
			cache.mu.Unlock()
			return nil, windowsStreamFailure("fetch", "revoked")
		}
		if entry := cache.entries[key]; entry != nil {
			data, err := os.ReadFile(filepath.Join(cache.dir, key+".chunk"))
			if err == nil && int64(len(data)) == entry.Size && lowerSHA256(data) == chunk.SHA256 {
				cache.clock++
				entry.LastUse = cache.clock
				cache.hits.Add(1)
				if err := cache.persistLocked(); err != nil {
					cache.mu.Unlock()
					zeroBytes(data)
					return nil, err
				}
				if cache.tombstones[variantKey] {
					cache.mu.Unlock()
					zeroBytes(data)
					return nil, windowsStreamFailure("fetch", "revoked")
				}
				cache.mu.Unlock()
				return data, nil
			}
			cache.removeEntryLocked(key)
			cache.integrityFailures.Add(1)
			_ = cache.persistLocked()
		}
		if flight := cache.inflight[key]; flight != nil {
			done := flight.done
			cache.mu.Unlock()
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-done:
				continue
			}
		}
		flight := &windowsStreamChunkFlight{done: make(chan struct{})}
		cache.inflight[key] = flight
		cache.mu.Unlock()

		data, err := cache.fetchAndStore(ctx, manifest, variantKey, key, chunk)
		cache.mu.Lock()
		delete(cache.inflight, key)
		close(flight.done)
		cache.mu.Unlock()
		return data, err
	}
}

func (cache *WindowsStreamChunkCache) fetchAndStore(
	ctx context.Context,
	manifest WindowsStreamManifest,
	variantKey, key string,
	chunk WindowsStreamChunk,
) ([]byte, error) {
	expected := chunk.End - chunk.Start + 1
	if expected > cache.limits.Chunk || expected > cache.limits.Network {
		return nil, windowsStreamFailure("fetch", "range_too_large")
	}
	var data []byte
	for attempt := 0; attempt < 2; attempt++ {
		cache.fetches.Add(1)
		body, etag, err := cache.fetcher.FetchRange(ctx, manifest.VariantURL, manifest.ETag, chunk.Start, chunk.End)
		if err != nil {
			stage, code := windowsStreamFailureCode(err)
			if code == "network_failed" && attempt == 0 {
				continue
			}
			if code == "revoked" {
				_ = cache.Tombstone(manifest)
			} else if code == "etag_changed" {
				_ = cache.Invalidate(manifest)
			}
			return nil, windowsStreamFailure(stage, code)
		}
		if etag != manifest.ETag {
			_ = cache.Invalidate(manifest)
			return nil, windowsStreamFailure("fetch", "etag_changed")
		}
		if int64(len(body)) == expected && lowerSHA256(body) == chunk.SHA256 {
			data = body
			break
		}
		cache.integrityFailures.Add(1)
	}
	if data == nil {
		return nil, windowsStreamFailure("integrity", "chunk_hash_mismatch")
	}
	cache.mu.Lock()
	defer cache.mu.Unlock()
	cache.processMu.Lock()
	defer cache.processMu.Unlock()
	if err := cache.mergePersistedIndexProcessLocked(); err != nil {
		zeroBytes(data)
		return nil, err
	}
	if cache.tombstones[variantKey] {
		zeroBytes(data)
		return nil, windowsStreamFailure("fetch", "revoked")
	}
	path := filepath.Join(cache.dir, key+".chunk")
	file, err := os.CreateTemp(cache.dir, "."+key+"-*.part")
	if err != nil {
		return nil, windowsStreamFailure("cache", "cache_unavailable")
	}
	tmp := file.Name()
	_, writeErr := file.Write(data)
	if writeErr == nil {
		writeErr = file.Sync()
	}
	if closeErr := file.Close(); writeErr == nil {
		writeErr = closeErr
	}
	renameErr := error(nil)
	if writeErr == nil {
		renameErr = os.Rename(tmp, path)
		if renameErr != nil {
			if existing, readErr := os.ReadFile(path); readErr == nil && bytes.Equal(existing, data) {
				renameErr = nil
			}
		}
	}
	if writeErr != nil || renameErr != nil {
		_ = os.Remove(tmp)
		return nil, windowsStreamFailure("cache", "cache_unavailable")
	}
	_ = os.Remove(tmp)
	cache.clock++
	cache.entries[key] = &windowsStreamCacheEntry{
		Key: key, VariantKey: variantKey, ChunkIndex: chunk.Index,
		Size: int64(len(data)), LastUse: cache.clock,
	}
	if err := cache.enforceLimitsLocked(key); err != nil {
		cache.removeEntryLocked(key)
		_ = cache.writeIndexProcessLocked()
		return nil, err
	}
	if err := cache.writeIndexProcessLocked(); err != nil {
		cache.removeEntryLocked(key)
		return nil, err
	}
	return append([]byte(nil), data...), nil
}

func (cache *WindowsStreamChunkCache) SetPinned(manifest WindowsStreamManifest, chunkIndexes []int) error {
	if validateWindowsStreamManifestShape(manifest) != nil {
		return windowsStreamFailure("manifest", "invalid_manifest")
	}
	variantKey := cache.variantKey(manifest)
	desired := make(map[string]bool, len(chunkIndexes))
	for _, index := range chunkIndexes {
		if index < 0 || index >= len(manifest.Chunks) {
			return windowsStreamFailure("cache", "invalid_pin")
		}
		desired[cache.chunkKey(variantKey, index)] = true
	}
	cache.mu.Lock()
	defer cache.mu.Unlock()
	var pinned int64
	for key, entry := range cache.entries {
		willPin := entry.VariantKey == variantKey && desired[key]
		if entry.Pinned && entry.VariantKey != variantKey || willPin {
			pinned += entry.Size
		}
	}
	if pinned > cache.limits.Pinned {
		return windowsStreamFailure("cache", "pinned_limit")
	}
	for key, entry := range cache.entries {
		if entry.VariantKey == variantKey {
			entry.Pinned = desired[key]
		}
	}
	return nil
}

func (cache *WindowsStreamChunkCache) Invalidate(manifest WindowsStreamManifest) error {
	return cache.removeVariant(manifest, false)
}

func (cache *WindowsStreamChunkCache) Tombstone(manifest WindowsStreamManifest) error {
	return cache.removeVariant(manifest, true)
}

func (cache *WindowsStreamChunkCache) removeVariant(manifest WindowsStreamManifest, tombstone bool) error {
	if validateWindowsStreamManifestShape(manifest) != nil {
		return windowsStreamFailure("manifest", "invalid_manifest")
	}
	variantKey := cache.variantKey(manifest)
	cache.mu.Lock()
	defer cache.mu.Unlock()
	if tombstone {
		cache.tombstones[variantKey] = true
	}
	for key, entry := range cache.entries {
		if entry.VariantKey == variantKey {
			cache.removeEntryLocked(key)
		}
	}
	return cache.persistLocked()
}

func (cache *WindowsStreamChunkCache) VerifyWhole(manifest WindowsStreamManifest) error {
	if validateWindowsStreamManifestShape(manifest) != nil {
		return windowsStreamFailure("manifest", "invalid_manifest")
	}
	hasher := sha256.New()
	cache.mu.Lock()
	defer cache.mu.Unlock()
	variantKey := cache.variantKey(manifest)
	for _, chunk := range manifest.Chunks {
		key := cache.chunkKey(variantKey, chunk.Index)
		entry := cache.entries[key]
		if entry == nil {
			return windowsStreamFailure("integrity", "whole_object_incomplete")
		}
		data, err := os.ReadFile(filepath.Join(cache.dir, key+".chunk"))
		if err != nil || int64(len(data)) != entry.Size || lowerSHA256(data) != chunk.SHA256 {
			cache.removeEntryLocked(key)
			cache.integrityFailures.Add(1)
			_ = cache.persistLocked()
			return windowsStreamFailure("integrity", "chunk_hash_mismatch")
		}
		_, _ = hasher.Write(data)
	}
	if hex.EncodeToString(hasher.Sum(nil)) != manifest.SHA256 {
		for key, entry := range cache.entries {
			if entry.VariantKey == variantKey {
				cache.removeEntryLocked(key)
			}
		}
		cache.integrityFailures.Add(1)
		_ = cache.persistLocked()
		return windowsStreamFailure("integrity", "whole_hash_mismatch")
	}
	return nil
}

func (cache *WindowsStreamChunkCache) Stats() WindowsStreamCacheStats {
	cache.mu.Lock()
	defer cache.mu.Unlock()
	var bytes, pinned int64
	for _, entry := range cache.entries {
		bytes += entry.Size
		if entry.Pinned {
			pinned += entry.Size
		}
	}
	return WindowsStreamCacheStats{
		Hits: cache.hits.Load(), Fetches: cache.fetches.Load(), Evictions: cache.evictions.Load(),
		IntegrityFailures: cache.integrityFailures.Load(), Repairs: cache.repairs.Load(),
		Bytes: bytes, PinnedBytes: pinned,
	}
}

func (cache *WindowsStreamChunkCache) enforceLimitsLocked(protected string) error {
	for {
		var total, protectedVariantTotal int64
		protectedVariant := ""
		if entry := cache.entries[protected]; entry != nil {
			protectedVariant = entry.VariantKey
		}
		for _, entry := range cache.entries {
			total += entry.Size
			if entry.VariantKey == protectedVariant {
				protectedVariantTotal += entry.Size
			}
		}
		if total <= cache.limits.Global && (protectedVariant == "" || protectedVariantTotal <= cache.limits.PerVariant) {
			return nil
		}
		var victim *windowsStreamCacheEntry
		for _, entry := range cache.entries {
			if entry.Pinned || entry.Key == protected {
				continue
			}
			if total > cache.limits.Global || entry.VariantKey == protectedVariant {
				if victim == nil || entry.LastUse < victim.LastUse {
					victim = entry
				}
			}
		}
		if victim == nil {
			return windowsStreamFailure("cache", "cache_limit")
		}
		cache.removeEntryLocked(victim.Key)
		cache.evictions.Add(1)
	}
}

func (cache *WindowsStreamChunkCache) removeEntryLocked(key string) {
	delete(cache.entries, key)
	_ = os.Remove(filepath.Join(cache.dir, key+".chunk"))
}

func (cache *WindowsStreamChunkCache) persistLocked() error {
	cache.processMu.Lock()
	defer cache.processMu.Unlock()
	if err := cache.mergePersistedIndexProcessLocked(); err != nil {
		return err
	}
	return cache.writeIndexProcessLocked()
}

func (cache *WindowsStreamChunkCache) mergePersistedIndexProcessLocked() error {
	raw, err := os.ReadFile(cache.indexPath)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return windowsStreamFailure("cache", "cache_unavailable")
	}
	var persisted windowsStreamCacheIndex
	if json.Unmarshal(raw, &persisted) != nil || persisted.Version != 1 {
		return windowsStreamFailure("cache", "cache_unavailable")
	}
	for _, value := range persisted.Tombstones {
		if len(value) == sha256.Size*2 {
			cache.tombstones[value] = true
		}
	}
	for key, entry := range cache.entries {
		if cache.tombstones[entry.VariantKey] {
			cache.removeEntryLocked(key)
		}
	}
	for _, entry := range persisted.Entries {
		if cache.entries[entry.Key] != nil || cache.tombstones[entry.VariantKey] ||
			len(entry.Key) != sha256.Size*2 || len(entry.VariantKey) != sha256.Size*2 ||
			entry.Size <= 0 || entry.Size > cache.limits.Chunk {
			continue
		}
		info, statErr := os.Lstat(filepath.Join(cache.dir, entry.Key+".chunk"))
		if statErr != nil || !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 || info.Size() != entry.Size {
			continue
		}
		copyValue := entry
		copyValue.Pinned = false
		cache.entries[copyValue.Key] = &copyValue
		if copyValue.LastUse > cache.clock {
			cache.clock = copyValue.LastUse
		}
	}
	return nil
}

func (cache *WindowsStreamChunkCache) writeIndexProcessLocked() error {
	index := windowsStreamCacheIndex{Version: 1}
	for _, entry := range cache.entries {
		copyValue := *entry
		copyValue.Pinned = false
		index.Entries = append(index.Entries, copyValue)
	}
	for value := range cache.tombstones {
		index.Tombstones = append(index.Tombstones, value)
	}
	sort.Slice(index.Entries, func(i, j int) bool { return index.Entries[i].Key < index.Entries[j].Key })
	sort.Strings(index.Tombstones)
	raw, err := json.Marshal(index)
	if err != nil {
		return windowsStreamFailure("cache", "cache_unavailable")
	}
	file, err := os.CreateTemp(cache.dir, ".index-*.part")
	if err != nil {
		return windowsStreamFailure("cache", "cache_unavailable")
	}
	tmp := file.Name()
	_, writeErr := file.Write(raw)
	if writeErr == nil {
		writeErr = file.Sync()
	}
	if closeErr := file.Close(); writeErr == nil {
		writeErr = closeErr
	}
	if writeErr != nil || os.Rename(tmp, cache.indexPath) != nil {
		_ = os.Remove(tmp)
		return windowsStreamFailure("cache", "cache_unavailable")
	}
	return nil
}

func lowerSHA256(data []byte) string {
	digest := sha256.Sum256(data)
	return hex.EncodeToString(digest[:])
}

// WindowsStreamHTTPRangeFetcher is production-shaped transport scaffolding,
// but is not composed into the production player while the decoder registry
// is empty. It permits only coordinator-relative paths and never redirects.
type WindowsStreamHTTPRangeFetcher struct {
	base  *url.URL
	token string
	httpc *http.Client
}

func NewWindowsStreamHTTPRangeFetcher(nodeToken, coordinatorURL string) (*WindowsStreamHTTPRangeFetcher, error) {
	parsed, err := url.Parse(coordinatorURL)
	if err != nil || parsed.Opaque != "" || parsed.Host == "" || parsed.User != nil || nodeToken == "" {
		return nil, windowsStreamFailure("fetch", "auth_failed")
	}
	scheme := strings.ToLower(parsed.Scheme)
	if scheme == "wss" {
		scheme = "https"
	} else if scheme == "ws" {
		scheme = "http"
	}
	if scheme != "https" && scheme != "http" {
		return nil, windowsStreamFailure("fetch", "auth_failed")
	}
	return &WindowsStreamHTTPRangeFetcher{
		base: &url.URL{Scheme: scheme, Host: parsed.Host}, token: nodeToken,
		httpc: &http.Client{Timeout: 30 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		}},
	}, nil
}

func (fetcher *WindowsStreamHTTPRangeFetcher) FetchRange(
	ctx context.Context,
	variantPath, etag string,
	start, end int64,
) ([]byte, string, error) {
	if start < 0 || end < start || end-start+1 > windowsStreamMaximumNetworkBytes ||
		etag == "" || !strings.HasPrefix(variantPath, "/v1/media/") {
		return nil, "", windowsStreamFailure("fetch", "invalid_range")
	}
	relative, err := url.Parse(variantPath)
	if err != nil || relative.IsAbs() || relative.Host != "" || relative.User != nil ||
		relative.RawQuery != "" || relative.Fragment != "" || relative.Path != variantPath {
		return nil, "", windowsStreamFailure("fetch", "auth_failed")
	}
	remote := *fetcher.base
	remote.Path = relative.Path
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, remote.String(), nil)
	if err != nil {
		return nil, "", windowsStreamFailure("fetch", "auth_failed")
	}
	request.Header.Set("Authorization", "Bearer "+fetcher.token)
	request.Header.Set("Accept", "application/octet-stream")
	request.Header.Set("Range", fmt.Sprintf("bytes=%d-%d", start, end))
	request.Header.Set("If-Range", etag)
	response, err := fetcher.httpc.Do(request)
	if err != nil {
		if ctx.Err() != nil {
			return nil, "", ctx.Err()
		}
		return nil, "", windowsStreamFailure("fetch", "network_failed")
	}
	defer response.Body.Close()
	if response.StatusCode == http.StatusNotFound || response.StatusCode == http.StatusGone {
		drainMediaResponse(response.Body)
		return nil, "", windowsStreamFailure("fetch", "revoked")
	}
	if response.StatusCode == http.StatusUnauthorized || response.StatusCode == http.StatusForbidden {
		drainMediaResponse(response.Body)
		return nil, "", windowsStreamFailure("fetch", "auth_failed")
	}
	if response.StatusCode != http.StatusPartialContent && response.StatusCode != http.StatusOK {
		drainMediaResponse(response.Body)
		return nil, "", windowsStreamFailure("fetch", "network_failed")
	}
	returnedETag := response.Header.Get("ETag")
	if returnedETag != etag {
		drainMediaResponse(response.Body)
		return nil, returnedETag, windowsStreamFailure("fetch", "etag_changed")
	}
	if response.StatusCode == http.StatusPartialContent {
		wantRange := fmt.Sprintf("bytes %d-%d/", start, end)
		if !strings.HasPrefix(response.Header.Get("Content-Range"), wantRange) {
			drainMediaResponse(response.Body)
			return nil, returnedETag, windowsStreamFailure("fetch", "invalid_range")
		}
	} else if start != 0 {
		drainMediaResponse(response.Body)
		return nil, returnedETag, windowsStreamFailure("fetch", "etag_changed")
	}
	expected := end - start + 1
	limited := &io.LimitedReader{R: response.Body, N: expected + 1}
	body, err := io.ReadAll(limited)
	if err != nil || int64(len(body)) != expected {
		return nil, returnedETag, windowsStreamFailure("fetch", "network_failed")
	}
	return body, returnedETag, nil
}

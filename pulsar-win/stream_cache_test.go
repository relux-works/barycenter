package main

import (
	"context"
	"crypto/sha256"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

type fakeWindowsStreamRangeFetcher struct {
	mu           sync.Mutex
	chunks       map[string][]byte
	calls        map[string]int
	corruptFirst map[string]bool
	failFirst    map[string]bool
	etag         string
	err          error
}

func newFakeWindowsStreamRangeFetcher(etag string) *fakeWindowsStreamRangeFetcher {
	return &fakeWindowsStreamRangeFetcher{
		chunks: make(map[string][]byte), calls: make(map[string]int),
		corruptFirst: make(map[string]bool), failFirst: make(map[string]bool), etag: etag,
	}
}

func streamRangeKey(start, end int64) string { return fmt.Sprintf("%d-%d", start, end) }

func (fetcher *fakeWindowsStreamRangeFetcher) FetchRange(
	_ context.Context,
	_ string,
	_ string,
	start, end int64,
) ([]byte, string, error) {
	fetcher.mu.Lock()
	defer fetcher.mu.Unlock()
	key := streamRangeKey(start, end)
	fetcher.calls[key]++
	if fetcher.failFirst[key] && fetcher.calls[key] == 1 {
		return nil, "", windowsStreamFailure("fetch", "network_failed")
	}
	if fetcher.err != nil {
		return nil, "", fetcher.err
	}
	body := append([]byte(nil), fetcher.chunks[key]...)
	if fetcher.corruptFirst[key] && fetcher.calls[key] == 1 && len(body) > 0 {
		body[0] ^= 0xff
	}
	return body, fetcher.etag, nil
}

func (fetcher *fakeWindowsStreamRangeFetcher) callCount(start, end int64) int {
	fetcher.mu.Lock()
	defer fetcher.mu.Unlock()
	return fetcher.calls[streamRangeKey(start, end)]
}

func windowsStreamManifest(id string, chunks ...[]byte) WindowsStreamManifest {
	manifest := WindowsStreamManifest{
		Identity: "svm1." + id, VariantURL: "/v1/media/m_" + id + "/variants/sv_" + id,
		DurationMS: int64(len(chunks)) * 1000,
	}
	var all []byte
	var offset int64
	for index, body := range chunks {
		manifest.Chunks = append(manifest.Chunks, WindowsStreamChunk{
			Index: index, Start: offset, End: offset + int64(len(body)) - 1,
			SHA256: lowerSHA256(body),
		})
		manifest.SeekMap = append(manifest.SeekMap, WindowsStreamSeekPoint{
			TimeMS: int64(index) * 1000, Offset: offset,
		})
		offset += int64(len(body))
		all = append(all, body...)
	}
	manifest.SizeBytes = offset
	manifest.SHA256 = lowerSHA256(all)
	manifest.ETag = `"sha256-` + manifest.SHA256 + `"`
	return manifest
}

func populateStreamFetcher(fetcher *fakeWindowsStreamRangeFetcher, manifest WindowsStreamManifest, chunks ...[]byte) {
	for index, chunk := range manifest.Chunks {
		fetcher.chunks[streamRangeKey(chunk.Start, chunk.End)] = append([]byte(nil), chunks[index]...)
	}
	fetcher.etag = manifest.ETag
}

func TestWindowsStreamChunkCacheRetriesIntegrityThenHitsAndVerifiesWhole(t *testing.T) {
	parts := [][]byte{[]byte("abcd"), []byte("efgh"), []byte("ijkl")}
	manifest := windowsStreamManifest("integrity", parts...)
	fetcher := newFakeWindowsStreamRangeFetcher(manifest.ETag)
	populateStreamFetcher(fetcher, manifest, parts...)
	fetcher.corruptFirst[streamRangeKey(0, 3)] = true
	fetcher.failFirst[streamRangeKey(4, 7)] = true
	cache, err := NewWindowsStreamChunkCache(t.TempDir(), []byte("0123456789abcdef"), fetcher)
	if err != nil {
		t.Fatal(err)
	}
	for index, want := range parts {
		got, err := cache.Get(context.Background(), manifest, index)
		if err != nil || string(got) != string(want) {
			t.Fatalf("chunk %d got=%q err=%v", index, got, err)
		}
	}
	if fetcher.callCount(0, 3) != 2 {
		t.Fatalf("corrupt chunk fetches=%d want=2", fetcher.callCount(0, 3))
	}
	if fetcher.callCount(4, 7) != 2 {
		t.Fatalf("reset range fetches=%d want=2", fetcher.callCount(4, 7))
	}
	if _, err := cache.Get(context.Background(), manifest, 0); err != nil {
		t.Fatal(err)
	}
	if fetcher.callCount(0, 3) != 2 {
		t.Fatal("cache hit unexpectedly refetched")
	}
	if err := cache.VerifyWhole(manifest); err != nil {
		t.Fatal(err)
	}
	stats := cache.Stats()
	if stats.Hits != 1 || stats.Fetches != 5 || stats.IntegrityFailures != 1 || stats.Bytes != 12 {
		t.Fatalf("stats=%+v", stats)
	}
}

func TestWindowsStreamChunkCacheBoundsPinsRepairAndDurableTombstone(t *testing.T) {
	root := t.TempDir()
	secret := []byte("0123456789abcdef")
	parts := [][]byte{[]byte("aaaa"), []byte("bbbb"), []byte("cccc")}
	manifest := windowsStreamManifest("bounded", parts...)
	fetcher := newFakeWindowsStreamRangeFetcher(manifest.ETag)
	populateStreamFetcher(fetcher, manifest, parts...)
	limits := windowsStreamCacheLimits{Global: 8, PerVariant: 8, Pinned: 4, Chunk: 4, Network: 4}
	cache, err := newWindowsStreamChunkCache(root, secret, fetcher, limits)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := cache.Get(context.Background(), manifest, 0); err != nil {
		t.Fatal(err)
	}
	if err := cache.SetPinned(manifest, []int{0}); err != nil {
		t.Fatal(err)
	}
	if _, err := cache.Get(context.Background(), manifest, 1); err != nil {
		t.Fatal(err)
	}
	if _, err := cache.Get(context.Background(), manifest, 2); err != nil {
		t.Fatal(err)
	}
	stats := cache.Stats()
	if stats.Bytes > limits.Global || stats.PinnedBytes != 4 || stats.Evictions == 0 {
		t.Fatalf("bounded stats=%+v", stats)
	}
	beforeRefetch := fetcher.callCount(4, 7)
	if _, err := cache.Get(context.Background(), manifest, 1); err != nil {
		t.Fatal(err)
	}
	if fetcher.callCount(4, 7) != beforeRefetch+1 {
		t.Fatal("evicted partial range was not independently refetched")
	}
	partPath := filepath.Join(root, "stream-v1", "crash.part")
	if err := os.WriteFile(partPath, []byte("partial"), 0o600); err != nil {
		t.Fatal(err)
	}
	repaired, err := newWindowsStreamChunkCache(root, secret, fetcher, limits)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(partPath); !os.IsNotExist(err) || repaired.Stats().Repairs == 0 {
		t.Fatalf("part repair err=%v stats=%+v", err, repaired.Stats())
	}
	if err := repaired.Tombstone(manifest); err != nil {
		t.Fatal(err)
	}
	reopened, err := newWindowsStreamChunkCache(root, secret, fetcher, limits)
	if err != nil {
		t.Fatal(err)
	}
	before := fetcher.callCount(0, 3)
	if _, err := reopened.Get(context.Background(), manifest, 0); err == nil {
		t.Fatal("tombstoned variant refilled")
	} else if _, code := windowsStreamFailureCode(err); code != "revoked" {
		t.Fatalf("tombstone error=%v", err)
	}
	if fetcher.callCount(0, 3) != before || reopened.Stats().Bytes != 0 {
		t.Fatalf("tombstone refetched or retained bytes: calls=%d stats=%+v", fetcher.callCount(0, 3), reopened.Stats())
	}
	index, err := os.ReadFile(filepath.Join(root, "stream-v1", "index-v1.json"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(index), manifest.Identity) || strings.Contains(string(index), "m_bounded") {
		t.Fatalf("cache index exposed manifest identity: %s", index)
	}
}

func TestWindowsStreamChunkCacheETagFlipInvalidatesAndRefetches(t *testing.T) {
	parts := [][]byte{[]byte("etag-a"), []byte("etag-b")}
	manifest := windowsStreamManifest("etag_flip", parts...)
	fetcher := newFakeWindowsStreamRangeFetcher(manifest.ETag)
	populateStreamFetcher(fetcher, manifest, parts...)
	cache, err := NewWindowsStreamChunkCache(t.TempDir(), []byte("0123456789abcdef"), fetcher)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := cache.Get(context.Background(), manifest, 0); err != nil {
		t.Fatal(err)
	}
	fetcher.mu.Lock()
	fetcher.etag = `"sha256-aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"`
	fetcher.mu.Unlock()
	if _, err := cache.Get(context.Background(), manifest, 1); err == nil {
		t.Fatal("mixed ETag was accepted")
	} else if stage, code := windowsStreamFailureCode(err); stage != "fetch" || code != "etag_changed" {
		t.Fatalf("etag flip error=%s:%s (%v)", stage, code, err)
	}
	if stats := cache.Stats(); stats.Bytes != 0 {
		t.Fatalf("etag invalidation retained bytes: %+v", stats)
	}
	fetcher.mu.Lock()
	fetcher.etag = manifest.ETag
	fetcher.mu.Unlock()
	before := fetcher.callCount(0, int64(len(parts[0])-1))
	if _, err := cache.Get(context.Background(), manifest, 0); err != nil {
		t.Fatal(err)
	}
	if fetcher.callCount(0, int64(len(parts[0])-1)) != before+1 {
		t.Fatal("invalidated chunk was not refetched after manifest re-resolution")
	}
}

func TestWindowsStreamChunkCacheWholeHashMismatchPurgesVariant(t *testing.T) {
	parts := [][]byte{[]byte("whole-a"), []byte("whole-b")}
	manifest := windowsStreamManifest("whole_mismatch", parts...)
	manifest.SHA256 = strings.Repeat("a", sha256.Size*2)
	manifest.ETag = `"sha256-` + manifest.SHA256 + `"`
	fetcher := newFakeWindowsStreamRangeFetcher(manifest.ETag)
	populateStreamFetcher(fetcher, manifest, parts...)
	cache, err := NewWindowsStreamChunkCache(t.TempDir(), []byte("0123456789abcdef"), fetcher)
	if err != nil {
		t.Fatal(err)
	}
	for index := range manifest.Chunks {
		if _, err := cache.Get(context.Background(), manifest, index); err != nil {
			t.Fatal(err)
		}
	}
	if err := cache.VerifyWhole(manifest); err == nil {
		t.Fatal("invalid whole-object digest was accepted")
	} else if stage, code := windowsStreamFailureCode(err); stage != "integrity" || code != "whole_hash_mismatch" {
		t.Fatalf("whole mismatch error=%s:%s (%v)", stage, code, err)
	}
	if stats := cache.Stats(); stats.Bytes != 0 || stats.IntegrityFailures != 1 {
		t.Fatalf("whole mismatch did not purge candidate: %+v", stats)
	}
}

func TestWindowsStreamHTTPRangeFetcherOriginAuthAndFailClosedResponses(t *testing.T) {
	body := []byte("range-body")
	digest := sha256.Sum256(body)
	etag := `"sha256-` + fmt.Sprintf("%x", digest) + `"`
	var gotAuthorization, gotRange, gotIfRange, gotQuery string
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		gotAuthorization = request.Header.Get("Authorization")
		gotRange = request.Header.Get("Range")
		gotIfRange = request.Header.Get("If-Range")
		gotQuery = request.URL.RawQuery
		writer.Header().Set("ETag", etag)
		writer.Header().Set("Content-Range", fmt.Sprintf("bytes 0-%d/%d", len(body)-1, len(body)))
		writer.WriteHeader(http.StatusPartialContent)
		_, _ = writer.Write(body)
	}))
	defer server.Close()
	fetcher, err := NewWindowsStreamHTTPRangeFetcher("node-secret", strings.Replace(server.URL, "http", "ws", 1)+"/v1/ws")
	if err != nil {
		t.Fatal(err)
	}
	got, returnedETag, err := fetcher.FetchRange(
		context.Background(), "/v1/media/m_http/variants/sv_http", etag, 0, int64(len(body)-1),
	)
	if err != nil || string(got) != string(body) || returnedETag != etag {
		t.Fatalf("range got=%q etag=%q err=%v", got, returnedETag, err)
	}
	if gotAuthorization != "Bearer node-secret" || gotRange != fmt.Sprintf("bytes=0-%d", len(body)-1) ||
		gotIfRange != etag || gotQuery != "" {
		t.Fatalf("headers auth=%q range=%q if-range=%q query=%q", gotAuthorization, gotRange, gotIfRange, gotQuery)
	}
	if _, _, err := fetcher.FetchRange(context.Background(), "https://evil.test/v1/media/x", etag, 0, 1); err == nil {
		t.Fatal("absolute credential-bearing origin accepted")
	}
}

func TestWindowsStreamManifestRejectsNonContiguousOrUnalignedMetadata(t *testing.T) {
	manifest := windowsStreamManifest("invalid", []byte("abcd"), []byte("efgh"))
	load := protocolStreamLoad(manifest, 1, 0, nowMS()+5000)
	if err := validateWindowsStreamManifest(load, manifest); err != nil {
		t.Fatal(err)
	}
	bad := manifest
	bad.Chunks = append([]WindowsStreamChunk(nil), manifest.Chunks...)
	bad.Chunks[1].Start++
	if err := validateWindowsStreamManifest(load, bad); err == nil {
		t.Fatal("non-contiguous chunks accepted")
	}
	bad = manifest
	bad.SeekMap = append([]WindowsStreamSeekPoint(nil), manifest.SeekMap...)
	bad.SeekMap[1].Offset++
	if err := validateWindowsStreamManifest(load, bad); err == nil {
		t.Fatal("unaligned seek point accepted")
	}
}

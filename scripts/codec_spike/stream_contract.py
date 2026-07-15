#!/usr/bin/env python3
"""Candidate-neutral stream manifest, range client and bounded private cache."""

from __future__ import annotations

import argparse
import contextlib
import hashlib
import hmac
import json
import os
import pathlib
import re
import sqlite3
import tempfile
import threading
import urllib.error
import urllib.request


ROOT = pathlib.Path(__file__).resolve().parents[2]
CONTRACT_PATH = ROOT / "acceptance" / "codec-spike" / "stream-contract-v1.json"
LOWER_SHA256 = re.compile(r"^[0-9a-f]{64}$")
SAFE_ID = re.compile(r"^[A-Za-z0-9][A-Za-z0-9._~-]{0,127}$")


class ContractError(ValueError):
    pass


class AccessRevoked(PermissionError):
    pass


class VersionChanged(RuntimeError):
    pass


class CacheFull(RuntimeError):
    pass


def load_contract() -> dict:
    return json.loads(CONTRACT_PATH.read_text(encoding="utf-8"))


def require(condition: bool, message: str) -> None:
    if not condition:
        raise ContractError(message)


def validate_contract(contract: dict) -> None:
    require(contract.get("schemaVersion") == 1, "unsupported stream contract schema")
    require(contract.get("contract") == "p2-stream-variants-range-cache.v1", "wrong contract")
    require(contract.get("rubric") == "p2-codec-spike-rubric.v1", "wrong parent rubric")
    cache = contract.get("cache", {})
    require(cache.get("globalCeilingBytes") == 512 << 20, "global cache ceiling moved")
    require(cache.get("perVariantCeilingBytes") == 64 << 20, "variant cache ceiling moved")
    require(cache.get("pinnedCeilingBytes") == 128 << 20, "pin ceiling moved")
    require(cache.get("maximumChunkBytes") == 1 << 20, "chunk ceiling moved")
    require(cache.get("maximumNetworkReadBytes") == 1 << 20, "network read ceiling moved")
    http = contract.get("http", {})
    require(http.get("authorization") ==
            "bearer-header-plus-immutable-target-snapshot-on-every-request",
            "range authorization weakened")
    require(http.get("credentialsInUrl") is False, "URL credentials enabled")
    require(http.get("missingUnauthorizedRevokedStatus") == 404, "ACL response discloses state")
    require(http.get("singleRangeOnly") is True, "multiple ranges enabled")
    require(http.get("cacheControl") == "private, no-store", "shared cache protection moved")
    require(set(http.get("vary", [])) == {"Authorization", "X-Codec-Spike-Target"},
            "tenant cache vary contract moved")
    row_fields = set(contract.get("variantRow", {}).get("requiredFields", []))
    require({"media_id", "codec", "container", "etag", "sha256", "chunk_manifest_json",
             "seek_map_json", "revoked_at"}.issubset(row_fields), "variant row is incomplete")
    integrity = contract.get("integrity", {})
    require(integrity.get("decodeBeforeIntegrity") is False, "decode may precede integrity")
    seek = contract.get("seekMap", {})
    require(seek.get("maximumPointSpacingMS") == 10000, "seek map spacing moved")
    require(seek.get("offsetMustStartChunk") is True, "seek offsets are not chunk aligned")
    require(contract.get("candidateInputs", {}).get("identicalAcrossCandidates") is True,
            "candidate inputs may diverge")


def digest_bytes(data: bytes) -> str:
    return hashlib.sha256(data).hexdigest()


def strong_etag(digest: str) -> str:
    if not LOWER_SHA256.fullmatch(digest):
        raise ContractError("invalid whole-object digest")
    return f'"sha256-{digest}"'


def build_variant_manifest(record: dict, data: bytes, chunk_size: int = 1 << 20) -> dict:
    if not SAFE_ID.fullmatch(str(record.get("id", ""))):
        raise ContractError("unsafe variant id")
    if not SAFE_ID.fullmatch(str(record.get("mediaId", ""))):
        raise ContractError("unsafe media id")
    if chunk_size <= 0 or chunk_size > 1 << 20:
        raise ContractError("chunk size exceeds frozen ceiling")
    if not data:
        raise ContractError("variant bytes are empty")
    duration_ms = int(record.get("durationMS", 0))
    if duration_ms <= 0:
        raise ContractError("variant duration is invalid")
    whole = digest_bytes(data)
    chunks = []
    for index, start in enumerate(range(0, len(data), chunk_size)):
        body = data[start:start + chunk_size]
        chunks.append({
            "index": index,
            "start": start,
            "end": start + len(body) - 1,
            "bytes": len(body),
            "sha256": digest_bytes(body),
        })
    # The prototype map is deliberately container-neutral: candidate-specific
    # probes can replace points with parsed keyframe/page positions, but must
    # keep the same monotonic/chunk-aligned contract.
    seek_points = []
    for at_ms in range(0, duration_ms + 1, 10000):
        proportional = min(len(data) - 1, (at_ms * len(data)) // duration_ms)
        offset = (proportional // chunk_size) * chunk_size
        point = {"timeMS": at_ms, "offset": offset}
        if not seek_points or point != seek_points[-1]:
            seek_points.append(point)
    if seek_points[0] != {"timeMS": 0, "offset": 0}:
        raise ContractError("seek map lacks zero point")
    manifest = {
        "schemaVersion": 1,
        "contract": "p2-stream-variants-range-cache.v1",
        "id": record["id"],
        "mediaId": record["mediaId"],
        "purpose": record.get("purpose", "canonical"),
        "codec": record.get("codec", ""),
        "container": record.get("container", ""),
        "mime": record.get("mime", "application/octet-stream"),
        "rateMode": record.get("rateMode", "unknown"),
        "bitrateBPS": int(record.get("bitrateBPS", 0)),
        "sampleRateHz": int(record.get("sampleRateHz", 48000)),
        "channels": int(record.get("channels", 2)),
        "durationMS": duration_ms,
        "sizeBytes": len(data),
        "sha256": whole,
        "etag": strong_etag(whole),
        "storageKey": f"stream/v1/{whole}",
        "chunkSizeBytes": chunk_size,
        "chunks": chunks,
        "seekMap": seek_points,
    }
    validate_variant_manifest(manifest)
    return manifest


def validate_variant_manifest(manifest: dict) -> None:
    require(manifest.get("schemaVersion") == 1, "unsupported manifest schema")
    require(manifest.get("contract") == "p2-stream-variants-range-cache.v1", "manifest contract mismatch")
    require(SAFE_ID.fullmatch(str(manifest.get("id", ""))) is not None, "unsafe variant id")
    require(SAFE_ID.fullmatch(str(manifest.get("mediaId", ""))) is not None, "unsafe media id")
    digest = manifest.get("sha256", "")
    require(LOWER_SHA256.fullmatch(digest) is not None, "invalid manifest digest")
    require(manifest.get("etag") == strong_etag(digest), "manifest etag mismatch")
    require(manifest.get("storageKey") == f"stream/v1/{digest}", "storage key is not content addressed")
    require(manifest.get("purpose") in ("original", "canonical"), "variant purpose is invalid")
    codec = manifest.get("codec")
    container = manifest.get("container")
    require(codec in ("mp3", "aac-lc", "opus"), "variant codec is unsupported")
    require(container in ("mp3", "m4a-faststart", "adts", "ogg"), "variant container is unsupported")
    require((codec, container) in {
        ("mp3", "mp3"), ("aac-lc", "m4a-faststart"), ("aac-lc", "adts"), ("opus", "ogg"),
    }, "codec and container do not match")
    require(manifest.get("mime") == {
        "mp3": "audio/mpeg", "m4a-faststart": "audio/mp4",
        "adts": "audio/aac", "ogg": "audio/ogg",
    }[container], "variant MIME does not match its container")
    require(manifest.get("rateMode") in ("cbr", "vbr"), "variant rate mode is invalid")
    require(int(manifest.get("bitrateBPS", 0)) > 0, "variant bitrate is invalid")
    require(manifest.get("sampleRateHz") == 48000 and manifest.get("channels") == 2,
            "variant decoded sample shape is not 48 kHz stereo")
    duration_ms = int(manifest.get("durationMS", 0))
    require(0 < duration_ms <= 7200000, "variant duration exceeds two hours")
    size = int(manifest.get("sizeBytes", 0))
    chunk_size = int(manifest.get("chunkSizeBytes", 0))
    require(size > 0 and 0 < chunk_size <= 1 << 20, "manifest sizes are invalid")
    require(size <= 500 << 20, "variant exceeds the frozen input ceiling")
    chunks = manifest.get("chunks", [])
    require(chunks, "chunk manifest is empty")
    cursor = 0
    for index, chunk in enumerate(chunks):
        require(chunk.get("index") == index and chunk.get("start") == cursor,
                "chunk manifest is not contiguous")
        length = int(chunk.get("bytes", 0))
        require(0 < length <= chunk_size, "chunk length is invalid")
        require(chunk.get("end") == cursor + length - 1, "chunk end mismatch")
        require(LOWER_SHA256.fullmatch(str(chunk.get("sha256", ""))) is not None,
                "chunk digest is invalid")
        cursor += length
    require(cursor == size, "chunk manifest size mismatch")
    points = manifest.get("seekMap", [])
    require(points and points[0] == {"timeMS": 0, "offset": 0}, "seek map lacks zero point")
    previous_time = -1
    previous_offset = -1
    for point in points:
        at_ms, offset = int(point.get("timeMS", -1)), int(point.get("offset", -1))
        require(at_ms > previous_time and offset >= previous_offset, "seek map is not monotonic")
        require(offset % chunk_size == 0 and offset < size, "seek offset is not a chunk start")
        if previous_time >= 0:
            require(at_ms - previous_time <= 10000, "seek map point spacing exceeds 10 seconds")
        previous_time, previous_offset = at_ms, offset
    require(duration_ms - previous_time <= 10000, "seek map does not cover the variant tail")


def seek_offset(manifest: dict, requested_ms: int) -> int:
    validate_variant_manifest(manifest)
    requested_ms = max(0, min(requested_ms, int(manifest["durationMS"])))
    chosen = 0
    for point in manifest["seekMap"]:
        if point["timeMS"] > requested_ms:
            break
        chosen = point["offset"]
    return chosen


STREAM_VARIANTS_SCHEMA = """
CREATE TABLE stream_variants(
  id TEXT PRIMARY KEY,
  media_id TEXT NOT NULL,
  purpose TEXT NOT NULL CHECK(purpose IN ('original','canonical')),
  codec TEXT NOT NULL,
  container TEXT NOT NULL,
  mime TEXT NOT NULL,
  rate_mode TEXT NOT NULL,
  bitrate_bps INTEGER NOT NULL CHECK(bitrate_bps >= 0),
  sample_rate_hz INTEGER NOT NULL CHECK(sample_rate_hz > 0),
  channels INTEGER NOT NULL CHECK(channels > 0),
  duration_ms INTEGER NOT NULL CHECK(duration_ms > 0),
  size_bytes INTEGER NOT NULL CHECK(size_bytes > 0),
  sha256 TEXT NOT NULL CHECK(length(sha256) = 64),
  etag TEXT NOT NULL,
  storage_key TEXT NOT NULL UNIQUE,
  chunk_size_bytes INTEGER NOT NULL CHECK(chunk_size_bytes BETWEEN 1 AND 1048576),
  chunk_manifest_json TEXT NOT NULL,
  seek_map_json TEXT NOT NULL,
  status TEXT NOT NULL CHECK(status IN ('staged','ready','revoked')),
  revision INTEGER NOT NULL CHECK(revision > 0),
  published_at INTEGER NOT NULL CHECK(published_at > 0),
  revoked_at INTEGER NOT NULL DEFAULT 0 CHECK(revoked_at >= 0),
  UNIQUE(media_id,codec,container,rate_mode,bitrate_bps),
  CHECK((status = 'revoked') = (revoked_at > 0))
);
"""


def materialize_catalog(fixtures: pathlib.Path, lock_path: pathlib.Path,
                        database: pathlib.Path, chunk_size: int = 1 << 20) -> list[dict]:
    root = fixtures.resolve()
    lock = json.loads(lock_path.read_text(encoding="utf-8"))
    manifests = []
    for item in lock.get("files", []):
        recipe = item.get("recipe")
        if not recipe or item.get("hostile"):
            continue
        relative = pathlib.Path(item.get("path", ""))
        candidate = root / relative
        path = candidate.resolve()
        if (relative.is_absolute() or len(relative.parts) != 1 or root not in path.parents or
                candidate.is_symlink() or not path.is_file()):
            raise ContractError("fixture lock contains an unsafe path")
        data = path.read_bytes()
        if len(data) != item.get("bytes") or digest_bytes(data) != item.get("sha256"):
            raise ContractError("fixture lock does not match bytes")
        probe = item.get("probe", {})
        container = recipe.get("container", "")
        mime = {"mp3": "audio/mpeg", "m4a-faststart": "audio/mp4",
                "adts": "audio/aac", "ogg": "audio/ogg"}.get(container, "")
        duration_ms = round(float(
            probe.get("durationSeconds", recipe.get("durationSeconds", 0))
        ) * 1000)
        bitrate_bps = int(recipe.get("bitrateKbps", 0)) * 1000
        if bitrate_bps <= 0 and duration_ms > 0:
            bitrate_bps = max(1, round(len(data) * 8 * 1000 / duration_ms))
        manifest = build_variant_manifest({
            "id": "sv-" + item["id"],
            "mediaId": "fixture-" + item["id"],
            "purpose": "canonical",
            "codec": recipe.get("codec", ""),
            "container": container,
            "mime": mime,
            "rateMode": recipe.get("rateMode", "unknown"),
            "bitrateBPS": bitrate_bps,
            "sampleRateHz": int(probe.get("sampleRateHz", 48000)),
            "channels": int(probe.get("channels", 2)),
            "durationMS": duration_ms,
        }, data, chunk_size)
        manifests.append(manifest)
    if not manifests:
        raise ContractError("fixture lock has no canonical variant recipes")
    database.parent.mkdir(parents=True, exist_ok=True)
    if database.exists():
        raise ContractError("catalog database already exists")
    connection = sqlite3.connect(database)
    try:
        connection.executescript(STREAM_VARIANTS_SCHEMA)
        for manifest in manifests:
            connection.execute("""INSERT INTO stream_variants VALUES(
                ?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)""", (
                manifest["id"], manifest["mediaId"], manifest["purpose"], manifest["codec"],
                manifest["container"], manifest["mime"], manifest["rateMode"], manifest["bitrateBPS"],
                manifest["sampleRateHz"], manifest["channels"], manifest["durationMS"],
                manifest["sizeBytes"], manifest["sha256"], manifest["etag"], manifest["storageKey"],
                manifest["chunkSizeBytes"], json.dumps(manifest["chunks"], separators=(",", ":")),
                json.dumps(manifest["seekMap"], separators=(",", ":")), "ready", 1, 1, 0,
            ))
        connection.commit()
        violations = connection.execute("PRAGMA integrity_check").fetchone()[0]
        if violations != "ok":
            raise ContractError("stream variant catalog failed integrity check")
    finally:
        connection.close()
    return manifests


def fetch_chunk(base_url: str, token: str, target: str, manifest: dict, index: int,
                timeout: float = 5.0) -> bytes:
    validate_variant_manifest(manifest)
    if not (0 <= index < len(manifest["chunks"])):
        raise ContractError("chunk index out of range")
    chunk = manifest["chunks"][index]
    request = urllib.request.Request(base_url, headers={
        "Authorization": f"Bearer {token}",
        "X-Codec-Spike-Target": target,
        "Range": f"bytes={chunk['start']}-{chunk['end']}",
        "If-Range": manifest["etag"],
        "Accept": manifest["mime"],
    })
    try:
        response = urllib.request.urlopen(request, timeout=timeout)
    except urllib.error.HTTPError as error:
        if error.code in (401, 403, 404, 410):
            raise AccessRevoked("variant is unavailable") from None
        raise
    with response:
        if response.status == 200:
            raise VersionChanged("variant etag changed")
        expected_range = f"bytes {chunk['start']}-{chunk['end']}/{manifest['sizeBytes']}"
        vary = {value.strip() for value in response.headers.get("Vary", "").split(",")}
        if (response.status != 206 or response.headers.get("Content-Range") != expected_range or
                response.headers.get("Content-Length") != str(chunk["bytes"]) or
                response.headers.get_content_type() != manifest["mime"] or
                response.headers.get("Accept-Ranges") != "bytes" or
                response.headers.get("ETag") != manifest["etag"] or
                response.headers.get("X-Content-SHA256") != manifest["sha256"] or
                response.headers.get("X-Content-Type-Options") != "nosniff" or
                response.headers.get("Cache-Control") != "private, no-store" or
                vary != {"Authorization", "X-Codec-Spike-Target"}):
            raise ContractError("range response violates the frozen contract")
        body = response.read(chunk["bytes"])
    if len(body) != chunk["bytes"] or digest_bytes(body) != chunk["sha256"]:
        raise ContractError("chunk integrity mismatch")
    return body


class BoundedPrivateChunkCache:
    """Crash-reconciled HMAC-keyed cache with hard byte and pin ceilings."""

    def __init__(self, root: pathlib.Path, secret: bytes, capacity: int = 512 << 20,
                 per_variant: int = 64 << 20, pin_capacity: int = 128 << 20,
                 max_chunk: int = 1 << 20):
        if len(secret) < 32 or min(capacity, per_variant, pin_capacity, max_chunk) <= 0:
            raise ContractError("invalid cache configuration")
        if max_chunk > 1 << 20 or per_variant > capacity or pin_capacity > capacity:
            raise ContractError("cache configuration exceeds frozen limits")
        if root.exists() and root.is_symlink():
            raise ContractError("cache root may not be a symlink")
        self.root = root.resolve()
        self.root.mkdir(parents=True, exist_ok=True, mode=0o700)
        os.chmod(self.root, 0o700)
        self.secret = secret
        self.capacity = capacity
        self.per_variant = per_variant
        self.pin_capacity = pin_capacity
        self.max_chunk = max_chunk
        self.lock = threading.RLock()
        self.pins: dict[str, int] = {}
        self.clock = 0
        index_path = self.root / "index.sqlite3"
        if index_path.is_symlink():
            raise ContractError("cache index may not be a symlink")
        self.db = sqlite3.connect(index_path, check_same_thread=False)
        self.db.execute("PRAGMA journal_mode=WAL")
        self.db.execute("PRAGMA foreign_keys=ON")
        self.db.execute("""CREATE TABLE IF NOT EXISTS chunks(
            cache_key TEXT PRIMARY KEY, namespace_key TEXT NOT NULL, variant_key TEXT NOT NULL,
            etag_key TEXT NOT NULL, chunk_index INTEGER NOT NULL, size_bytes INTEGER NOT NULL,
            sha256 TEXT NOT NULL, relative_path TEXT NOT NULL UNIQUE, last_used INTEGER NOT NULL,
            tombstoned INTEGER NOT NULL DEFAULT 0 CHECK(tombstoned IN (0,1)))""")
        self.db.execute("""CREATE TABLE IF NOT EXISTS invalidations(
            namespace_key TEXT NOT NULL, variant_key TEXT NOT NULL,
            invalidated_at INTEGER NOT NULL,
            PRIMARY KEY(namespace_key,variant_key))""")
        self.db.commit()
        self._reconcile()

    def close(self) -> None:
        with self.lock:
            self.db.close()

    def _digest(self, label: str, value: str) -> str:
        return hmac.new(self.secret, (label + "\0" + value).encode(), hashlib.sha256).hexdigest()

    def _identity(self, namespace: str, variant: str, etag: str, index: int) -> tuple[str, str, str]:
        if not namespace or not variant or not etag or index < 0:
            raise ContractError("invalid cache identity")
        namespace_key = self._digest("namespace", namespace)
        variant_key = self._digest("variant", namespace + "\0" + variant)
        cache_key = self._digest("chunk", namespace + "\0" + variant + "\0" + etag + f"\0{index}")
        return namespace_key, variant_key, cache_key

    def _chunk_path(self, cache_key: str, relative: str) -> pathlib.Path | None:
        if not LOWER_SHA256.fullmatch(cache_key) or relative != cache_key + ".chunk":
            return None
        return self.root / relative

    def _remove_row(self, cache_key: str, relative: str) -> None:
        path = self._chunk_path(cache_key, relative)
        if path is not None:
            path.unlink(missing_ok=True)
        self.db.execute("DELETE FROM chunks WHERE cache_key=?", (cache_key,))

    def _reconcile(self) -> None:
        with self.lock:
            for part in self.root.glob("*.part"):
                part.unlink(missing_ok=True)
            rows = list(self.db.execute("SELECT cache_key,relative_path,size_bytes,sha256 FROM chunks"))
            known = set()
            for key, relative, size, digest in rows:
                path = self._chunk_path(key, relative)
                if path is not None:
                    known.add(relative)
                if (path is None or not path.is_file() or path.is_symlink() or path.stat().st_size != size or
                        digest_bytes(path.read_bytes()) != digest):
                    self._remove_row(key, relative)
            for path in self.root.glob("*.chunk"):
                if path.name not in known:
                    path.unlink(missing_ok=True)
            self.db.commit()
            self.clock = int(self.db.execute(
                "SELECT COALESCE(MAX(last_used),0) FROM chunks"
            ).fetchone()[0])
            self._evict(0, "")

    def _total(self, variant_key: str = "") -> int:
        if variant_key:
            row = self.db.execute("SELECT COALESCE(SUM(size_bytes),0) FROM chunks WHERE variant_key=?",
                                  (variant_key,)).fetchone()
        else:
            row = self.db.execute("SELECT COALESCE(SUM(size_bytes),0) FROM chunks").fetchone()
        return int(row[0])

    def _pinned_bytes(self) -> int:
        total = 0
        for key, count in self.pins.items():
            if count:
                row = self.db.execute("SELECT size_bytes FROM chunks WHERE cache_key=?", (key,)).fetchone()
                if row:
                    total += int(row[0])
        return total

    def _evict(self, incoming: int, variant_key: str) -> None:
        while (self._total() + incoming > self.capacity or
               (variant_key and self._total(variant_key) + incoming > self.per_variant)):
            variant_over = bool(variant_key and self._total(variant_key) + incoming > self.per_variant)
            if variant_over:
                rows = self.db.execute("""SELECT cache_key,relative_path FROM chunks
                    WHERE tombstoned=0 AND variant_key=? ORDER BY last_used,cache_key""",
                                       (variant_key,)).fetchall()
            else:
                rows = self.db.execute("""SELECT cache_key,relative_path FROM chunks
                    WHERE tombstoned=0 ORDER BY last_used,cache_key""").fetchall()
            victim = next((row for row in rows if self.pins.get(row[0], 0) == 0), None)
            if victim is None:
                raise CacheFull("cache is pinned at its hard ceiling")
            self._remove_row(victim[0], victim[1])
            self.db.commit()

    def put(self, namespace: str, variant: str, etag: str, index: int,
            data: bytes, expected_sha256: str) -> str:
        if not data or len(data) > self.max_chunk or digest_bytes(data) != expected_sha256:
            raise ContractError("cache chunk is oversized or corrupt")
        namespace_key, variant_key, cache_key = self._identity(namespace, variant, etag, index)
        relative = cache_key + ".chunk"
        with self.lock:
            revoked = self.db.execute("""SELECT 1 FROM invalidations
                WHERE namespace_key=? AND variant_key IN ('*',?) LIMIT 1""",
                                      (namespace_key, variant_key)).fetchone()
            if revoked:
                raise AccessRevoked("cache namespace is invalidated")
            old = self.db.execute("SELECT size_bytes FROM chunks WHERE cache_key=?", (cache_key,)).fetchone()
            incoming = len(data) - (int(old[0]) if old else 0)
            self._evict(max(0, incoming), variant_key)
            fd, temporary = tempfile.mkstemp(prefix=cache_key + ".", suffix=".part", dir=self.root)
            try:
                os.chmod(temporary, 0o600)
                with os.fdopen(fd, "wb") as stream:
                    stream.write(data)
                    stream.flush()
                    os.fsync(stream.fileno())
                os.replace(temporary, self.root / relative)
                directory_fd = os.open(self.root, os.O_RDONLY)
                try:
                    os.fsync(directory_fd)
                finally:
                    os.close(directory_fd)
            finally:
                pathlib.Path(temporary).unlink(missing_ok=True)
            self.clock += 1
            self.db.execute("""INSERT INTO chunks(cache_key,namespace_key,variant_key,etag_key,
                chunk_index,size_bytes,sha256,relative_path,last_used,tombstoned)
                VALUES(?,?,?,?,?,?,?,?,?,0) ON CONFLICT(cache_key) DO UPDATE SET
                size_bytes=excluded.size_bytes,sha256=excluded.sha256,
                relative_path=excluded.relative_path,last_used=excluded.last_used,tombstoned=0""",
                (cache_key, namespace_key, variant_key, self._digest("etag", etag), index,
                 len(data), expected_sha256, relative, self.clock))
            self.db.commit()
            return cache_key

    @contextlib.contextmanager
    def open(self, namespace: str, variant: str, etag: str, index: int, expected_sha256: str):
        _, _, cache_key = self._identity(namespace, variant, etag, index)
        stream = None
        miss = False
        with self.lock:
            row = self.db.execute("SELECT relative_path,size_bytes,sha256,tombstoned FROM chunks WHERE cache_key=?",
                                  (cache_key,)).fetchone()
            if not row or row[3] or row[2] != expected_sha256:
                miss = True
            else:
                if self._pinned_bytes() + int(row[1]) > self.pin_capacity and not self.pins.get(cache_key):
                    raise CacheFull("cache pin ceiling reached")
                path = self._chunk_path(cache_key, row[0])
                try:
                    if path is None:
                        raise OSError("unsafe cache path")
                    stream = path.open("rb")
                except OSError:
                    self._remove_row(cache_key, row[0])
                    self.db.commit()
                    miss = True
                if stream is not None and digest_bytes(stream.read()) != expected_sha256:
                    stream.close()
                    stream = None
                    self._remove_row(cache_key, row[0])
                    self.db.commit()
                    miss = True
                if stream is not None:
                    stream.seek(0)
                    self.pins[cache_key] = self.pins.get(cache_key, 0) + 1
                    self.clock += 1
                    self.db.execute("UPDATE chunks SET last_used=? WHERE cache_key=?", (self.clock, cache_key))
                    self.db.commit()
        if miss:
            yield None
            return
        try:
            yield stream
        finally:
            if stream:
                stream.close()
            with self.lock:
                self.pins[cache_key] = max(0, self.pins.get(cache_key, 1) - 1)
                row = self.db.execute("SELECT relative_path,tombstoned FROM chunks WHERE cache_key=?",
                                      (cache_key,)).fetchone()
                if row and row[1] and self.pins[cache_key] == 0:
                    self._remove_row(cache_key, row[0])
                    self.db.commit()

    def invalidate(self, namespace: str, variant: str | None = None) -> int:
        namespace_key = self._digest("namespace", namespace)
        parameters: tuple = (namespace_key,)
        query = "SELECT cache_key,relative_path FROM chunks WHERE namespace_key=?"
        if variant is not None:
            variant_key = self._digest("variant", namespace + "\0" + variant)
            query += " AND variant_key=?"
            parameters = (namespace_key, variant_key)
        with self.lock:
            rows = self.db.execute(query, parameters).fetchall()
            self.clock += 1
            self.db.execute("""INSERT INTO invalidations(namespace_key,variant_key,invalidated_at)
                VALUES(?,?,?) ON CONFLICT(namespace_key,variant_key) DO UPDATE SET
                invalidated_at=excluded.invalidated_at""",
                (namespace_key, variant_key if variant is not None else "*", self.clock))
            for key, relative in rows:
                if self.pins.get(key, 0):
                    self.db.execute("UPDATE chunks SET tombstoned=1 WHERE cache_key=?", (key,))
                else:
                    self._remove_row(key, relative)
            self.db.commit()
            return len(rows)

    def total_bytes(self) -> int:
        with self.lock:
            return self._total()


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--validate", action="store_true")
    parser.add_argument("--fixtures", type=pathlib.Path)
    parser.add_argument("--lock", type=pathlib.Path)
    parser.add_argument("--database", type=pathlib.Path)
    args = parser.parse_args()
    validate_contract(load_contract())
    if args.validate and not any((args.fixtures, args.lock, args.database)):
        print(f"stream contract valid: {CONTRACT_PATH.relative_to(ROOT)}")
        return 0
    if not all((args.fixtures, args.lock, args.database)):
        parser.error("use --validate or provide --fixtures, --lock and --database")
    manifests = materialize_catalog(args.fixtures, args.lock, args.database)
    print(json.dumps({"database": str(args.database), "variants": len(manifests)}, sort_keys=True))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())

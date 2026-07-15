#!/usr/bin/env python3
"""Authenticated RFC-range and deterministic fault server for codec probes."""

from __future__ import annotations

import argparse
import hashlib
import json
import os
import pathlib
import re
import socket
import threading
import time
from http import HTTPStatus
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from urllib.parse import parse_qs, unquote, urlsplit


ALLOWED_PROFILES = {
    "normal", "no_range", "slow_256kbit", "reset_mid_body", "truncate_body",
    "corrupt_chunk", "etag_flip", "revoked",
}
RANGE = re.compile(r"^bytes=(\d*)-(\d*)$")


def sha256(path: pathlib.Path) -> str:
    digest = hashlib.sha256()
    with path.open("rb") as stream:
        for block in iter(lambda: stream.read(1024 * 1024), b""):
            digest.update(block)
    return digest.hexdigest()


class FixtureCatalog:
    def __init__(self, root: pathlib.Path, lock_path: pathlib.Path):
        self.root = root.resolve()
        lock = json.loads(lock_path.read_text(encoding="utf-8"))
        self.items: dict[str, dict] = {}
        for record in lock.get("files", []):
            fixture_id = record.get("id", "")
            relative = pathlib.Path(record.get("path", ""))
            if not fixture_id or relative.is_absolute() or len(relative.parts) != 1:
                raise ValueError("fixture lock contains an unsafe path")
            candidate = self.root / relative
            path = candidate.resolve()
            if self.root not in path.parents or candidate.is_symlink() or not path.is_file():
                raise ValueError(f"fixture is missing or unsafe: {fixture_id}")
            if path.stat().st_size != record.get("bytes") or sha256(path) != record.get("sha256"):
                raise ValueError(f"fixture lock mismatch: {fixture_id}")
            if fixture_id in self.items:
                raise ValueError(f"duplicate fixture id: {fixture_id}")
            self.items[fixture_id] = {**record, "resolved": path}
        if not self.items:
            raise ValueError("fixture lock is empty")


class RangeServer(ThreadingHTTPServer):
    daemon_threads = True

    def __init__(self, address, catalog: FixtureCatalog, token: str, target: str,
                 log_path: pathlib.Path, slow_bps: int = 256000):
        super().__init__(address, RangeHandler)
        self.catalog = catalog
        self.token = token
        self.target = target
        self.log_path = log_path
        self.slow_bps = slow_bps
        self._lock = threading.Lock()
        self._request_counts: dict[tuple[str, str], int] = {}

    def next_count(self, fixture_id: str, profile: str) -> int:
        with self._lock:
            key = (fixture_id, profile)
            self._request_counts[key] = self._request_counts.get(key, 0) + 1
            return self._request_counts[key]

    def record(self, event: dict) -> None:
        encoded = json.dumps(event, sort_keys=True, separators=(",", ":")) + "\n"
        with self._lock:
            self.log_path.parent.mkdir(parents=True, exist_ok=True)
            with self.log_path.open("a", encoding="utf-8") as stream:
                stream.write(encoded)


def parse_range(value: str | None, size: int) -> tuple[int, int] | None:
    if value is None:
        return None
    match = RANGE.fullmatch(value.strip())
    if not match or "," in value:
        raise ValueError("invalid or multiple range")
    first, last = match.groups()
    if not first and not last:
        raise ValueError("empty range")
    if first:
        start = int(first)
        end = int(last) if last else size - 1
        if start >= size or end < start:
            raise ValueError("unsatisfiable range")
        return start, min(end, size - 1)
    length = int(last)
    if length <= 0:
        raise ValueError("invalid suffix")
    length = min(length, size)
    return size - length, size - 1


class RangeHandler(BaseHTTPRequestHandler):
    server: RangeServer
    protocol_version = "HTTP/1.1"

    def log_message(self, _format: str, *_args) -> None:
        return

    def error(self, status: HTTPStatus, fixture_id: str = "", profile: str = "") -> None:
        body = json.dumps({"error": status.phrase.lower().replace(" ", "_")}).encode()
        self.send_response(status)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(body)))
        self.send_header("Cache-Control", "no-store")
        self.send_header("Vary", "Authorization, X-Codec-Spike-Target")
        self.send_header("X-Content-Type-Options", "nosniff")
        self.end_headers()
        if self.command != "HEAD":
            self.wfile.write(body)
        self.server.record({
            "method": self.command, "fixtureId": fixture_id, "profile": profile,
            "range": self.headers.get("Range", ""), "status": int(status), "bytes": len(body),
        })

    def do_HEAD(self) -> None:
        self.serve_fixture()

    def do_GET(self) -> None:
        self.serve_fixture()

    def serve_fixture(self) -> None:
        authorization = self.headers.get("Authorization", "")
        target = self.headers.get("X-Codec-Spike-Target", "")
        if authorization != f"Bearer {self.server.token}" or target != self.server.target:
            self.error(HTTPStatus.NOT_FOUND)
            return
        parsed = urlsplit(self.path)
        prefix = "/v1/fixtures/"
        if not parsed.path.startswith(prefix):
            self.error(HTTPStatus.NOT_FOUND)
            return
        fixture_id = unquote(parsed.path[len(prefix):])
        if "/" in fixture_id or fixture_id not in self.server.catalog.items:
            self.error(HTTPStatus.NOT_FOUND)
            return
        query = parse_qs(parsed.query, keep_blank_values=True)
        profile = query.get("profile", ["normal"])[0]
        if (profile not in ALLOWED_PROFILES or set(query) - {"profile"} or
                len(query.get("profile", ["normal"])) != 1):
            self.error(HTTPStatus.BAD_REQUEST, fixture_id, profile)
            return
        count = self.server.next_count(fixture_id, profile)
        if profile == "revoked":
            self.error(HTTPStatus.GONE, fixture_id, profile)
            return

        item = self.server.catalog.items[fixture_id]
        path: pathlib.Path = item["resolved"]
        size = item["bytes"]
        etag_suffix = "-rotated" if profile == "etag_flip" and count > 1 else ""
        etag = f'"sha256-{item["sha256"]}{etag_suffix}"'
        requested_range = self.headers.get("Range")
        if self.headers.get("If-None-Match") == etag and not requested_range:
            self.send_response(HTTPStatus.NOT_MODIFIED)
            self.send_header("ETag", etag)
            self.send_header("Cache-Control", "private, no-store")
            self.send_header("Vary", "Authorization, X-Codec-Spike-Target")
            self.send_header("Content-Length", "0")
            self.end_headers()
            self.server.record({"method": self.command, "fixtureId": fixture_id, "profile": profile,
                                "range": "", "status": 304, "bytes": 0})
            return
        if requested_range and self.headers.get("If-Range") not in (None, etag):
            requested_range = None
        if profile == "no_range":
            requested_range = None
        try:
            bounds = parse_range(requested_range, size)
        except ValueError:
            self.send_response(HTTPStatus.REQUESTED_RANGE_NOT_SATISFIABLE)
            self.send_header("Content-Range", f"bytes */{size}")
            self.send_header("Cache-Control", "private, no-store")
            self.send_header("Vary", "Authorization, X-Codec-Spike-Target")
            self.send_header("Content-Length", "0")
            self.end_headers()
            self.server.record({"method": self.command, "fixtureId": fixture_id, "profile": profile,
                                "range": self.headers.get("Range", ""), "status": 416, "bytes": 0})
            return
        start, end = bounds if bounds else (0, size - 1)
        content_length = end - start + 1
        status = HTTPStatus.PARTIAL_CONTENT if bounds else HTTPStatus.OK
        self.send_response(status)
        self.send_header("Accept-Ranges", "none" if profile == "no_range" else "bytes")
        self.send_header("Cache-Control", "private, no-store")
        self.send_header("Vary", "Authorization, X-Codec-Spike-Target")
        self.send_header("ETag", etag)
        self.send_header("X-Content-SHA256", item["sha256"])
        self.send_header("X-Content-Type-Options", "nosniff")
        container = item.get("recipe", {}).get("container", "")
        content_type = {
            "mp3": "audio/mpeg", "m4a-faststart": "audio/mp4",
            "adts": "audio/aac", "ogg": "audio/ogg",
        }.get(container, "application/octet-stream")
        self.send_header("Content-Type", content_type)
        self.send_header("Content-Length", str(content_length))
        if bounds:
            self.send_header("Content-Range", f"bytes {start}-{end}/{size}")
        self.end_headers()
        sent = 0
        if self.command != "HEAD":
            send_limit = content_length
            if profile in ("reset_mid_body", "truncate_body"):
                send_limit = max(1, content_length // 2)
            corrupt_at = content_length // 2
            with path.open("rb") as stream:
                stream.seek(start)
                while sent < send_limit:
                    chunk = bytearray(stream.read(min(8192, send_limit - sent)))
                    if not chunk:
                        break
                    if profile == "corrupt_chunk" and sent <= corrupt_at < sent + len(chunk):
                        chunk[corrupt_at - sent] ^= 0xA5
                    self.wfile.write(chunk)
                    self.wfile.flush()
                    sent += len(chunk)
                    if profile == "slow_256kbit":
                        time.sleep(len(chunk) * 8 / self.server.slow_bps)
            if profile == "reset_mid_body":
                self.connection.shutdown(socket.SHUT_RDWR)
                self.connection.close()
            elif profile == "truncate_body":
                self.close_connection = True
        self.server.record({
            "method": self.command, "fixtureId": fixture_id, "profile": profile,
            "range": self.headers.get("Range", ""), "status": int(status), "bytes": sent,
            "declaredBytes": content_length, "requestOrdinal": count,
        })


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--fixtures", type=pathlib.Path, required=True)
    parser.add_argument("--lock", type=pathlib.Path, required=True)
    parser.add_argument("--target", required=True)
    parser.add_argument("--token-env", default="CODEC_SPIKE_TOKEN")
    parser.add_argument("--bind", default="127.0.0.1")
    parser.add_argument("--port", type=int, default=0)
    parser.add_argument("--ready-file", type=pathlib.Path, required=True)
    parser.add_argument("--log", type=pathlib.Path, required=True)
    args = parser.parse_args()
    token = os.environ.get(args.token_env, "")
    if len(token) < 32:
        raise SystemExit(f"{args.token_env} must contain at least 32 characters")
    if not re.fullmatch(r"[A-Za-z0-9._~-]{1,128}", args.target):
        raise SystemExit("target must be an opaque safe token")
    catalog = FixtureCatalog(args.fixtures, args.lock)
    server = RangeServer((args.bind, args.port), catalog, token, args.target, args.log)
    host, port = server.server_address[:2]
    args.ready_file.parent.mkdir(parents=True, exist_ok=True)
    args.ready_file.write_text(json.dumps({
        "schemaVersion": 1, "baseUrl": f"http://{host}:{port}",
        "fixtureCount": len(catalog.items), "tokenIncluded": False,
    }, indent=2, sort_keys=True) + "\n", encoding="utf-8")
    try:
        server.serve_forever()
    finally:
        server.server_close()
    return 0


if __name__ == "__main__":
    raise SystemExit(main())

#!/usr/bin/env python3
"""Build and run the exact local libopus benchmark without downloading code."""

from __future__ import annotations

import argparse
import hashlib
import json
import os
import pathlib
import platform
import subprocess
import tempfile


ROOT = pathlib.Path(__file__).resolve().parents[2]
SOURCE = ROOT / "scripts/live_ptt/opus_benchmark.c"


def command(*args: str) -> str:
    return subprocess.run(args, check=True, text=True, capture_output=True).stdout.strip()


def sha256(path: pathlib.Path) -> str:
    return hashlib.sha256(path.read_bytes()).hexdigest()


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--output", type=pathlib.Path, required=True)
    parser.add_argument("--frames", type=int, default=12000)
    args = parser.parse_args()

    prefix = pathlib.Path(os.environ.get("OPUS_PREFIX") or command("brew", "--prefix", "opus"))
    pkg_config = prefix / "lib/pkgconfig/opus.pc"
    version = pkg_config.read_text(encoding="utf-8")
    if "Version: 1.6.1" not in version:
        raise SystemExit(f"expected libopus 1.6.1 under {prefix}")
    library_candidates = sorted((prefix / "lib").glob("libopus.*"))
    library = next((path.resolve() for path in library_candidates if path.suffix in (".dylib", ".so")), None)
    if library is None or not library.is_file():
        raise SystemExit(f"could not resolve libopus shared library under {prefix}")

    with tempfile.TemporaryDirectory(prefix="barycenter-live-opus-") as directory:
        binary = pathlib.Path(directory) / "opus-benchmark"
        subprocess.run(
            [
                "cc", "-std=c11", "-O2", "-Wall", "-Wextra", "-Werror",
                f"-I{prefix / 'include/opus'}", str(SOURCE),
                f"-L{prefix / 'lib'}", f"-Wl,-rpath,{prefix / 'lib'}", "-lopus", "-lm",
                "-o", str(binary),
            ],
            check=True,
        )
        profiles = []
        for frame_ms in (10, 20):
            raw = command(str(binary), "--frame-ms", str(frame_ms), "--frames", str(args.frames))
            profiles.append(json.loads(raw))

    artifact = {
        "schemaVersion": 1,
        "contract": "p3-live-opus-local-benchmark.v1",
        "claimBoundary": "single-host engineering evidence; not Windows, signed-package, real-app, listening, or physical-network acceptance",
        "host": {
            "system": platform.system(),
            "release": platform.release(),
            "machine": platform.machine(),
            "python": platform.python_version(),
            "compiler": command("cc", "--version").splitlines()[0],
            "opusPrefix": str(prefix),
            "libraryPathResolved": str(library),
            "librarySha256": sha256(library),
            "pkgConfigSha256": sha256(pkg_config),
        },
        "sourceSha256": sha256(SOURCE),
        "profiles": profiles,
    }
    args.output.parent.mkdir(parents=True, exist_ok=True)
    args.output.write_text(json.dumps(artifact, indent=2) + "\n", encoding="utf-8")
    print(args.output)
    return 0


if __name__ == "__main__":
    raise SystemExit(main())

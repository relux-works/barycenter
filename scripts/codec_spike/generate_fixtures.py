#!/usr/bin/env python3
"""Generate the frozen codec corpus and a content-addressed fixture lock.

The encoded corpus is intentionally not committed. Every candidate consumes
one generated directory plus its exact lock file; evidence binds the lock hash.
"""

from __future__ import annotations

import argparse
import hashlib
import json
import pathlib
import shutil
import subprocess
import sys


ROOT = pathlib.Path(__file__).resolve().parents[2]
RUBRIC_PATH = ROOT / "acceptance" / "codec-spike" / "rubric-v1.json"
EXTENSIONS = {"mp3": ".mp3", "m4a-faststart": ".m4a", "adts": ".aac", "ogg": ".opus"}


def sha256(path: pathlib.Path) -> str:
    digest = hashlib.sha256()
    with path.open("rb") as stream:
        for block in iter(lambda: stream.read(1024 * 1024), b""):
            digest.update(block)
    return digest.hexdigest()


def rubric() -> dict:
    return json.loads(RUBRIC_PATH.read_text(encoding="utf-8"))


def version_output(tool: str) -> str:
    return subprocess.check_output((tool, "-version"), text=True, stderr=subprocess.STDOUT)


def require_toolchain(contract: dict) -> tuple[str, str]:
    ffmpeg = shutil.which("ffmpeg")
    ffprobe = shutil.which("ffprobe")
    if not ffmpeg or not ffprobe:
        raise RuntimeError("ffmpeg and ffprobe 8.1.2 are required; no automatic download is allowed")
    ffmpeg_version = version_output(ffmpeg)
    ffprobe_version = version_output(ffprobe)
    expected = f"ffmpeg version {contract['fixtureToolchain']['version']}"
    if not ffmpeg_version.startswith(expected) or not ffprobe_version.startswith(
        f"ffprobe version {contract['fixtureToolchain']['version']}"
    ):
        raise RuntimeError(f"fixture toolchain must be exactly {expected}")
    return ffmpeg_version, ffprobe_version


def audio_source(duration: int) -> str:
    # Six ten-second marker bands repeat every minute. Seek verification can
    # identify the expected band without depending on speech or copyrighted audio.
    left = r"0.12*sin(2*PI*(440+110*floor(mod(t\,60)/10))*t)"
    right = r"0.12*sin(2*PI*(660+110*floor(mod(t\,60)/10))*t)"
    return f"aevalsrc=exprs={left}|{right}:s=48000:d={duration}"


def encoder_args(item: dict) -> list[str]:
    container = item["container"]
    mode = item["rateMode"]
    if container == "mp3":
        args = ["-c:a", "libmp3lame", "-write_xing", "1", "-id3v2_version", "3"]
        args += ["-b:a", f"{item['bitrateKbps']}k"] if mode == "cbr" else ["-q:a", str(item["quality"])]
        return args + ["-f", "mp3"]
    if container in ("m4a-faststart", "adts"):
        args = ["-c:a", "aac", "-profile:a", "aac_low"]
        args += ["-b:a", f"{item['bitrateKbps']}k"] if mode == "cbr" else ["-q:a", str(item["quality"])]
        if container == "m4a-faststart":
            return args + ["-movflags", "+faststart", "-f", "mp4"]
        return args + ["-f", "adts"]
    if container == "ogg":
        return [
            "-c:a", "libopus", "-application", "audio", "-frame_duration", "20",
            "-b:a", f"{item['bitrateKbps']}k", "-vbr", "off" if mode == "cbr" else "on", "-f", "ogg",
        ]
    raise ValueError(f"unknown container {container}")


def recipes(contract: dict, smoke_only: bool) -> list[dict]:
    result: list[dict] = []
    smoke_shapes = [
        ("mp3_cbr_12s", "mp3", "mp3", "cbr", 192, None),
        ("mp3_vbr_12s", "mp3", "mp3", "vbr", None, 4),
        ("aac_m4a_12s", "aac-lc", "m4a-faststart", "cbr", 160, None),
        ("aac_adts_12s", "aac-lc", "adts", "vbr", None, 2),
        ("opus_ogg_cbr_12s", "opus", "ogg", "cbr", 128, None),
        ("opus_ogg_vbr_12s", "opus", "ogg", "vbr", 128, None),
    ]
    for fixture_id, codec, container, mode, bitrate, quality in smoke_shapes:
        item = {"id": fixture_id, "codec": codec, "container": container,
                "durationSeconds": 12, "rateMode": mode}
        if bitrate is not None:
            item["bitrateKbps"] = bitrate
        if quality is not None:
            item["quality"] = quality
        result.append(item)
    if not smoke_only:
        result.extend(contract["fixtureClasses"])
    return result


def mutate(base: bytes, mutation: str) -> bytes:
    if mutation.startswith("truncate:"):
        return base[: int(mutation.split(":", 1)[1])]
    if mutation == "prefix:max-synchsafe-id3":
        return b"ID3\x04\x00\x00\x7f\x7f\x7f\x7f" + base
    if mutation == "prefix:overflowing-mp4-atom":
        return b"\xff\xff\xff\xfffree" + base[:32]
    if mutation == "xor:middle:0x5a":
        data = bytearray(base)
        data[len(data) // 2] ^= 0x5A
        return bytes(data)
    if mutation == "xor:ogg-crc:0xff":
        data = bytearray(base)
        if len(data) < 26:
            raise ValueError("Ogg base too short")
        data[22] ^= 0xFF
        return bytes(data)
    raise ValueError(f"unknown mutation {mutation}")


def probe(ffprobe: str, path: pathlib.Path) -> dict:
    raw = subprocess.check_output((
        ffprobe, "-v", "error", "-select_streams", "a:0",
        "-show_entries", "format=duration:stream=codec_name,sample_rate,channels",
        "-of", "json", str(path),
    ), text=True)
    parsed = json.loads(raw)
    stream = parsed["streams"][0]
    return {
        "codec": stream["codec_name"],
        "sampleRateHz": int(stream["sample_rate"]),
        "channels": int(stream["channels"]),
        "durationSeconds": float(parsed["format"]["duration"]),
    }


def generate(args: argparse.Namespace) -> dict:
    contract = rubric()
    ffmpeg_version, ffprobe_version = require_toolchain(contract)
    ffmpeg = shutil.which("ffmpeg")
    ffprobe = shutil.which("ffprobe")
    assert ffmpeg and ffprobe
    args.output.mkdir(parents=True, exist_ok=False)
    records: list[dict] = []
    by_id: dict[str, pathlib.Path] = {}
    for item in recipes(contract, args.smoke_only):
        extension = EXTENSIONS[item["container"]]
        path = args.output / f"{item['id']}{extension}"
        command = [
            ffmpeg, "-hide_banner", "-loglevel", "error", "-nostdin", "-y", "-threads", "1",
            "-f", "lavfi", "-i", audio_source(item["durationSeconds"]), "-map_metadata", "-1",
        ] + encoder_args(item) + [str(path)]
        subprocess.run(command, check=True)
        measured = probe(ffprobe, path)
        if measured["sampleRateHz"] != 48000 or measured["channels"] != 2:
            raise RuntimeError(f"unexpected decoded shape for {item['id']}: {measured}")
        record = {
            "id": item["id"], "path": path.name, "bytes": path.stat().st_size,
            "sha256": sha256(path), "recipe": item, "probe": measured,
        }
        records.append(record)
        by_id[item["id"]] = path
    for hostile in contract["hostileFixtures"]:
        base_path = by_id[hostile["base"]]
        suffix = base_path.suffix
        path = args.output / f"{hostile['id']}{suffix}"
        path.write_bytes(mutate(base_path.read_bytes(), hostile["mutation"]))
        records.append({
            "id": hostile["id"], "path": path.name, "bytes": path.stat().st_size,
            "sha256": sha256(path), "hostile": True, "base": hostile["base"],
            "mutation": hostile["mutation"], "expected": "reject-without-crash",
        })
    lock = {
        "schemaVersion": 1,
        "rubric": contract["contract"],
        "toolchain": {
            "ffmpegVersion": ffmpeg_version.splitlines()[0],
            "ffprobeVersion": ffprobe_version.splitlines()[0],
            "ffmpegConfigurationSha256": hashlib.sha256(ffmpeg_version.encode()).hexdigest(),
        },
        "files": sorted(records, key=lambda item: item["id"]),
    }
    args.lock.parent.mkdir(parents=True, exist_ok=True)
    args.lock.write_text(json.dumps(lock, indent=2, sort_keys=True) + "\n", encoding="utf-8")
    return lock


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--output", type=pathlib.Path)
    parser.add_argument("--lock", type=pathlib.Path)
    parser.add_argument("--smoke-only", action="store_true")
    parser.add_argument("--plan", action="store_true")
    args = parser.parse_args()
    contract = rubric()
    if args.plan:
        print(json.dumps({
            "toolchain": contract["fixtureToolchain"],
            "recipes": recipes(contract, args.smoke_only),
            "hostileFixtures": contract["hostileFixtures"],
        }, indent=2, sort_keys=True))
        return 0
    if args.output is None or args.lock is None:
        parser.error("--output and --lock are required unless --plan is used")
    lock = generate(args)
    print(json.dumps({"files": len(lock["files"]), "lock": str(args.lock)}, sort_keys=True))
    return 0


if __name__ == "__main__":
    try:
        raise SystemExit(main())
    except (OSError, ValueError, RuntimeError, subprocess.SubprocessError) as error:
        print(f"codec fixture generation: {error}", file=sys.stderr)
        raise SystemExit(1)

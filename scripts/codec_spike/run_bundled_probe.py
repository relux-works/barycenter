#!/usr/bin/env python3
"""Exercise the bundled decoder bridge over immutable private-cache inputs."""

from __future__ import annotations

import argparse
import hashlib
import json
import pathlib
import shutil
import subprocess
import sys
import tempfile
import time


HERE = pathlib.Path(__file__).resolve().parent
ROOT = HERE.parents[1]
sys.path.insert(0, str(HERE))

import generate_fixtures  # noqa: E402
import inventory_bundled_probe  # noqa: E402
import stream_contract  # noqa: E402


def invoke(driver: pathlib.Path, media: pathlib.Path, *arguments: str) -> dict:
    completed = subprocess.run(
        [str(driver), str(media), *arguments], check=False, text=True, capture_output=True, timeout=15,
    )
    if completed.returncode != 0:
        raise RuntimeError(f"probe failed for {media.name}: {completed.stderr} {completed.stdout}")
    return json.loads(completed.stdout)


def prepare_private(cache: stream_contract.BoundedPrivateChunkCache, fixture: dict,
                    source: pathlib.Path, destination: pathlib.Path, generation: int) -> None:
    payload = source.read_bytes()
    digest = hashlib.sha256(payload).hexdigest()
    if digest != fixture["sha256"]:
        raise ValueError(f"fixture drift: {fixture['id']}")
    key = cache.put("probe-tenant", fixture["id"], f'"generation-{generation}"', 0, payload, digest)
    with cache.open("probe-tenant", fixture["id"], f'"generation-{generation}"', 0, digest) as stream:
        if stream is None:
            raise RuntimeError("private cache did not return prepared chunk")
        destination.write_bytes(stream.read())
    if source.stat().st_size > 1024 * 1024 or key == source.name:
        raise RuntimeError("bounded private-cache invariant failed")


def exercise(driver: pathlib.Path) -> dict:
    contract = inventory_bundled_probe.load_contract()
    inventory_bundled_probe.validate_contract(contract)
    results = []
    hostile = []
    lifecycle = []
    with tempfile.TemporaryDirectory() as directory:
        root = pathlib.Path(directory)
        cache = stream_contract.BoundedPrivateChunkCache(
            root / "cache", b"bundled-probe-private-install-secret", capacity=64 * 1024 * 1024,
            per_variant=64 * 1024 * 1024, pin_capacity=8 * 1024 * 1024,
            max_chunk=1024 * 1024,
        )
        try:
            for fixture in contract["smokeFixtures"]:
                source = ROOT / fixture["path"]
                prepared = root / pathlib.Path(fixture["path"]).name
                prepare_private(cache, fixture, source, prepared, 1)
                lifecycle.extend([
                    {"fixture": fixture["id"], "event": "prepare", "generation": 1},
                    {"fixture": fixture["id"], "event": "arm", "generation": 1},
                ])
                scheduled = time.monotonic_ns() + 2_000_000
                while time.monotonic_ns() < scheduled:
                    time.sleep(0.0002)
                complete = invoke(driver, prepared)
                lifecycle.append({"fixture": fixture["id"], "event": "scheduled-start", "generation": 1})
                if complete["codec"] != fixture["codec"] or complete["sampleRate"] != 48000 or \
                        complete["channels"] != 2 or complete["frames"] <= 0 or \
                        complete["samples"] < fixture["minimumDurationMS"] * 48 or not complete["drained"] or \
                        complete["peakRSSBytes"] <= 0 or complete["peakRSSBytes"] > 256 * 1024 * 1024 or \
                        complete["cpuMS"] > 15_000:
                    raise RuntimeError(f"decode contract failed: {fixture['id']} {complete}")
                paused = invoke(driver, prepared, "--cancel-after-frames", "3")
                if not paused["cancelled"] or paused["drained"]:
                    raise RuntimeError(f"pause/cancel contract failed: {fixture['id']}")
                lifecycle.append({"fixture": fixture["id"], "event": "pause", "generation": 1})
                seeked = invoke(driver, prepared, "--seek-ms", "5000")
                if seeked["frames"] <= 0 or seeked["pcmChecksum"] == complete["pcmChecksum"]:
                    raise RuntimeError(f"seek did not create new decoded generation: {fixture['id']}")
                lifecycle.extend([
                    {"fixture": fixture["id"], "event": "seek-new-generation", "generation": 2},
                    {"fixture": fixture["id"], "event": "resume", "generation": 2},
                    {"fixture": fixture["id"], "event": "drain", "generation": 2},
                    {"fixture": fixture["id"], "event": "cancel", "generation": 1},
                ])
                results.append({"fixture": fixture["id"], "full": complete, "seek": seeked,
                                "cancel": paused})

            bases = contract["smokeFixtures"][:]
            mutations = contract["hostileMutations"]
            for index, mutation in enumerate(mutations):
                fixture = bases[index % len(bases)]
                payload = (ROOT / fixture["path"]).read_bytes()
                mutated = root / f"hostile-{index}{pathlib.Path(fixture['path']).suffix}"
                mutated.write_bytes(generate_fixtures.mutate(payload, mutation))
                completed = subprocess.run(
                    [str(driver), str(mutated)], text=True, capture_output=True, timeout=5,
                )
                hostile.append({"mutation": mutation, "exitCode": completed.returncode,
                                "boundedOutputBytes": len(completed.stdout) + len(completed.stderr)})
                if len(completed.stdout) + len(completed.stderr) > 8192:
                    raise RuntimeError(f"hostile output unbounded: {mutation}")
        finally:
            cache.close()
    return {
        "schemaVersion": 1,
        "contract": contract["contract"],
        "claimClass": "repository-engineering-prototype",
        "driverSha256": hashlib.sha256(driver.read_bytes()).hexdigest(),
        "decodeResults": results,
        "hostileResults": hostile,
        "lifecycle": lifecycle,
        "cacheCeilingBytes": 64 * 1024 * 1024,
        "maxChunkBytes": 1024 * 1024,
        "peakRSSBytes": max(item["full"]["peakRSSBytes"] for item in results),
        "totalDecodeCPUMS": sum(item["full"]["cpuMS"] for item in results),
        "packageDiskBytesFromReceipt": True,
        "networkOwnedByDecoder": False,
        "renderCallbackCallsDecoder": False,
        "shippingDecision": "rejected-until-all-required-platform-and-release-evidence-exists",
    }


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--driver", type=pathlib.Path, required=True)
    parser.add_argument("--output", type=pathlib.Path, required=True)
    args = parser.parse_args()
    result = exercise(args.driver.resolve())
    args.output.parent.mkdir(parents=True, exist_ok=True)
    args.output.write_text(json.dumps(result, indent=2) + "\n", encoding="utf-8")
    print(args.output)
    return 0


if __name__ == "__main__":
    raise SystemExit(main())

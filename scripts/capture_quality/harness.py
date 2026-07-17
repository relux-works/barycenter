#!/usr/bin/env python3
"""Deterministic synthetic capture-quality corpus and conformance evaluator.

This tool never records audio.  It generates non-speech float32 fixtures, binds
them to a checked-in content lock, and evaluates an adapter-produced bundle.
Synthetic receipts exercise the harness itself; they are not hardware evidence.
"""

from __future__ import annotations

import argparse
import hashlib
import json
import math
import pathlib
import statistics
import struct
import sys
from typing import Iterable


ROOT = pathlib.Path(__file__).resolve().parents[2]
CONTRACT_PATH = ROOT / "acceptance/phase3/capture-quality-harness-v1.json"
LOCK_PATH = ROOT / "acceptance/phase3/capture-quality-harness-lock-v1.json"
PARENT_CONTRACT_PATH = ROOT / "acceptance/phase3/capture-quality-contract-v1.json"
TAU = 2.0 * math.pi


class HarnessError(RuntimeError):
    pass


def sha256_bytes(data: bytes) -> str:
    return hashlib.sha256(data).hexdigest()


def sha256_file(path: pathlib.Path) -> str:
    digest = hashlib.sha256()
    with path.open("rb") as stream:
        for block in iter(lambda: stream.read(1024 * 1024), b""):
            digest.update(block)
    return digest.hexdigest()


def load_json(path: pathlib.Path) -> dict:
    value = json.loads(path.read_text(encoding="utf-8"))
    if not isinstance(value, dict):
        raise HarnessError(f"{path}: expected JSON object")
    return value


def contract() -> dict:
    value = load_json(CONTRACT_PATH)
    if value.get("contract") != "p3-capture-quality-harness.v1":
        raise HarnessError("unexpected capture-quality harness contract")
    ids = [item.get("id") for item in value.get("fixtures", [])]
    if len(ids) != 14 or len(set(ids)) != len(ids):
        raise HarnessError("contract must contain 14 unique fixtures")
    if value.get("manualBoundary", {}).get("status") != "not-run":
        raise HarnessError("synthetic contract must not claim manual evidence")
    parent = load_json(PARENT_CONTRACT_PATH)
    rubric = parent.get("objectiveRubric", {}).get("metrics", {})
    thresholds = value.get("thresholds", {})
    inherited = {
        "farEndOnly": {key: thresholds.get("farEndOnly", {}).get(key) for key in ("convergenceMS", "medianERLEMinDB", "p10ERLEMinDB", "residualRMSMaxDBFS")},
        "nearEndOnly": {
            "absoluteLevelChangeMaxDB": thresholds.get("nearEndOnly", {}).get("absoluteLevelChangeMaxDB"),
            "stoiDeltaMin": thresholds.get("nearEndOnly", {}).get("finalSTOIDeltaMin"),
        },
        "doubleTalk": {
            "nearEndAttenuationMaxDB": thresholds.get("doubleTalk", {}).get("nearEndAttenuationMaxDB"),
            "stoiDeltaMin": thresholds.get("doubleTalk", {}).get("finalSTOIDeltaMin"),
            "medianERLEMinDB": thresholds.get("doubleTalk", {}).get("medianERLEMinDB"),
        },
        "noiseSuppression": {key: thresholds.get("noiseSuppression", {}).get(key) for key in ("snrImprovementMinDB", "nearEndAttenuationMaxDB")},
        "inputAGC": {
            "targetRMSDBFS": thresholds.get("inputAGC", {}).get("targetRMSDBFS"),
            "toleranceDB": thresholds.get("inputAGC", {}).get("targetToleranceDB"),
            "peakMaxDBFS": thresholds.get("inputAGC", {}).get("peakMaxDBFS"),
            "maximumGainDB": thresholds.get("inputAGC", {}).get("maximumGainDB"),
            "maximumGainChangeDBPerSecond": thresholds.get("inputAGC", {}).get("maximumGainChangeDBPerSecond"),
        },
        "clipping": {"clippedSampleFractionMax": thresholds.get("clipping", {}).get("clippedSampleFractionMax")},
        "processor": {key: thresholds.get("processor", {}).get(key) for key in ("addedLatencyP95MaxMS", "callbackAllocationMax", "callbackBlockingWaitMax")},
        "routeChange": {key: thresholds.get("routeChange", {}).get(key) for key in ("committedSamplesDuringReconfigureMax", "maximumRearmMS")},
        "clockDrift": {key: thresholds.get("clockDrift", {}).get(key) for key in ("fixtureRangePPM", "referenceAgeMaxMS")},
    }
    if inherited != rubric:
        raise HarnessError("harness thresholds drift from the frozen parent C3 rubric")
    return value


class Noise:
    def __init__(self, seed: int):
        self.state = seed & 0xFFFFFFFF

    def sample(self) -> float:
        self.state = (1664525 * self.state + 1013904223) & 0xFFFFFFFF
        return ((self.state >> 8) / 8388607.5 - 1.0)


def tone(count: int, rate: int, frequency: int, amplitude: float, phase: int = 0) -> list[float]:
    # Integer phase selection makes recipe choices explicit and repeatable.
    return [amplitude * math.sin(TAU * ((frequency * index + phase) % rate) / rate) for index in range(count)]


def speech_like(count: int, rate: int, seed: int) -> list[float]:
    a = tone(count, rate, 317, 0.045, seed % rate)
    b = tone(count, rate, 613, 0.030, (seed * 3) % rate)
    c = tone(count, rate, 997, 0.020, (seed * 7) % rate)
    # A deterministic envelope avoids a stationary single-tone sentinel.
    return [(a[i] + b[i] + c[i]) * (0.72 + 0.28 * math.sin(TAU * 3 * i / rate)) for i in range(count)]


def noise(count: int, seed: int, amplitude: float) -> list[float]:
    source = Noise(seed)
    return [amplitude * source.sample() for _ in range(count)]


def mix(*signals: Iterable[float]) -> list[float]:
    arrays = [list(signal) for signal in signals]
    if not arrays:
        return []
    if len({len(item) for item in arrays}) != 1:
        raise HarnessError("cannot mix signals of different lengths")
    return [sum(item[index] for item in arrays) for index in range(len(arrays[0]))]


def gain(signal: Iterable[float], factor: float) -> list[float]:
    return [sample * factor for sample in signal]


def clamp(signal: Iterable[float]) -> list[float]:
    return [max(-1.0, min(1.0, sample)) for sample in signal]


def encode(signal: Iterable[float]) -> bytes:
    values = list(signal)
    return struct.pack(f"<{len(values)}f", *values)


def decode(path: pathlib.Path, expected_samples: int) -> list[float]:
    raw = path.read_bytes()
    expected_bytes = expected_samples * 4
    if len(raw) != expected_bytes:
        raise HarnessError(f"{path.name}: expected {expected_bytes} bytes, got {len(raw)}")
    values = list(struct.unpack(f"<{expected_samples}f", raw))
    if not all(math.isfinite(item) and abs(item) <= 1.0 for item in values):
        raise HarnessError(f"{path.name}: non-finite or out-of-range sample")
    return values


def fixture_signals(item: dict, spec: dict) -> dict[str, list[float]]:
    rate = spec["sampleFormat"]["sampleRateHz"]
    count = rate * item["durationMS"] // 1000
    seed = spec["determinism"]["seed"] + sum(ord(char) for char in item["id"])
    far = tone(count, rate, 431, 0.10, seed % rate)
    near = speech_like(count, rate, seed)
    bed = noise(count, seed, 0.012)
    echo = gain(far, 0.25)
    capture = [0.0] * count
    fixture_id = item["id"]
    if fixture_id in {"far_end_only", "clock_drift", "echo_path_change"}:
        capture = echo
        if fixture_id == "echo_path_change":
            pivot = count // 2
            capture = gain(far[:pivot], 0.18) + gain(far[pivot:], 0.36)
    elif fixture_id == "near_end_only":
        capture = mix(near, bed)
    elif fixture_id == "double_talk":
        capture = mix(near, bed, echo)
    elif fixture_id == "clipping":
        capture = clamp(gain(near, 20.0))
    elif fixture_id == "too_quiet":
        near = gain(near, 0.08)
        capture = mix(near, gain(bed, 0.08))
    elif fixture_id == "missing_reference":
        far = [0.0] * count
        capture = mix(near, bed)
    elif fixture_id in {"silence", "device_loss", "route_change", "processor_overrun", "effect_failure", "live_packet_cancel"}:
        near = [0.0] * count
        far = [0.0] * count
        capture = [0.0] * count
        bed = [0.0] * count
    return {"far": far, "near": near, "noise": bed, "capture": capture}


def generate_corpus(output: pathlib.Path, lock_path: pathlib.Path | None) -> dict:
    spec = contract()
    if output.exists():
        raise HarnessError(f"output already exists: {output}")
    output.mkdir(parents=True)
    records: list[dict] = []
    for item in spec["fixtures"]:
        signals = fixture_signals(item, spec)
        files: dict[str, dict] = {}
        for role, signal in sorted(signals.items()):
            name = f"{item['id']}.{role}.f32le"
            data = encode(signal)
            (output / name).write_bytes(data)
            files[role] = {"path": name, "bytes": len(data), "sha256": sha256_bytes(data)}
        records.append({
            "id": item["id"], "durationMS": item["durationMS"],
            "samples": len(signals["capture"]), "files": files,
        })
    lock = {
        "schemaVersion": 1,
        "contract": spec["contract"],
        "generator": "scripts/capture_quality/harness.py",
        "seed": spec["determinism"]["seed"],
        "sampleFormat": spec["sampleFormat"],
        "fixtures": records,
    }
    if lock_path is not None:
        lock_path.parent.mkdir(parents=True, exist_ok=True)
        lock_path.write_bytes(json.dumps(lock, indent=2, sort_keys=True).encode() + b"\n")
    return lock


def verify_corpus(corpus: pathlib.Path, lock: dict) -> None:
    for fixture in lock.get("fixtures", []):
        for record in fixture.get("files", {}).values():
            path = safe_child(corpus, record["path"])
            if path.stat().st_size != record["bytes"] or sha256_file(path) != record["sha256"]:
                raise HarnessError(f"fixture lock mismatch: {record['path']}")


def safe_child(root: pathlib.Path, relative: str) -> pathlib.Path:
    value = pathlib.PurePosixPath(relative)
    if value.is_absolute() or ".." in value.parts or value.as_posix() != relative:
        raise HarnessError(f"unsafe relative path: {relative}")
    path = root / pathlib.Path(*value.parts)
    if path.is_symlink() or root.resolve() not in path.resolve().parents:
        raise HarnessError(f"path escapes artifact root: {relative}")
    return path


def rms(signal: Iterable[float]) -> float:
    values = list(signal)
    return math.sqrt(sum(item * item for item in values) / max(1, len(values)))


def db(value: float) -> float:
    return 20.0 * math.log10(max(value, 1e-12))


def level_change(reference: list[float], output: list[float]) -> float:
    return db(rms(output)) - db(rms(reference))


def correlation(reference: list[float], output: list[float]) -> float:
    dot = sum(a * b for a, b in zip(reference, output))
    denominator = math.sqrt(sum(a * a for a in reference) * sum(b * b for b in output))
    if denominator > 1e-15:
        return dot / denominator
    return 1.0 if rms(reference) < 1e-12 and rms(output) < 1e-12 else 0.0


def projected_residual(reference: list[float], output: list[float]) -> tuple[float, float]:
    energy = sum(item * item for item in reference)
    coefficient = sum(a * b for a, b in zip(reference, output)) / energy if energy else 0.0
    residual = [b - coefficient * a for a, b in zip(reference, output)]
    return coefficient, rms(residual)


def percentile(values: list[float], fraction: float) -> float:
    ordered = sorted(values)
    if not ordered:
        raise HarnessError("empty percentile")
    return ordered[min(len(ordered) - 1, max(0, math.floor((len(ordered) - 1) * fraction)))]


def numeric_series(value: object, name: str) -> list[float]:
    if not isinstance(value, list) or not value or any(
        not isinstance(item, (int, float)) or isinstance(item, bool) or not math.isfinite(item)
        for item in value
    ):
        raise HarnessError(f"runtime {name} must be a non-empty finite numeric array")
    return [float(item) for item in value]


def nonnegative_number(value: object, name: str) -> float:
    if (
        not isinstance(value, (int, float)) or isinstance(value, bool)
        or not math.isfinite(value) or value < 0
    ):
        raise HarnessError(f"runtime {name} must be a finite non-negative number")
    return float(value)


def require(condition: bool, failures: list[str], message: str) -> None:
    if not condition:
        failures.append(message)


def evaluate_audio(case_id: str, signals: dict[str, list[float]], output: list[float], spec: dict) -> tuple[dict, list[str]]:
    thresholds = spec["thresholds"]
    capture, near, far = signals["capture"], signals["near"], signals["far"]
    metrics: dict[str, float] = {
        "outputRMSDBFS": round(db(rms(output)), 4),
        "peakDBFS": round(db(max((abs(item) for item in output), default=0.0)), 4),
        "clippedFraction": round(sum(abs(item) >= 0.999 for item in output) / max(1, len(output)), 8),
    }
    failures: list[str] = []
    if case_id in {"far_end_only", "echo_path_change", "clock_drift"}:
        block = spec["sampleFormat"]["sampleRateHz"] * thresholds["farEndOnly"]["blockMS"] // 1000
        erle = []
        for start in range(0, len(output), block):
            before, after = rms(capture[start:start + block]), rms(output[start:start + block])
            erle.append(db(before) - db(after))
        convergence_blocks = thresholds["farEndOnly"]["convergenceMS"] // thresholds["farEndOnly"]["blockMS"]
        steady = erle[min(convergence_blocks, len(erle) - 1):]
        metrics.update({"medianERLEDB": round(statistics.median(steady), 4),
                        "p10ERLEDB": round(percentile(steady, 0.10), 4)})
        require(statistics.median(steady) >= thresholds["farEndOnly"]["medianERLEMinDB"], failures, "fake bypass: median ERLE below floor")
        require(percentile(steady, 0.10) >= thresholds["farEndOnly"]["p10ERLEMinDB"], failures, "unstable cancellation: p10 ERLE below floor")
        require(db(rms(output)) <= thresholds["farEndOnly"]["residualRMSMaxDBFS"], failures, "far-end residual exceeds ceiling")
    if case_id in {"near_end_only", "double_talk"}:
        coefficient, residual = projected_residual(near, output)
        corr = correlation(near, output)
        attenuation = -db(abs(coefficient))
        metrics.update({"nearCorrelation": round(corr, 6), "nearAttenuationDB": round(attenuation, 4),
                        "unexplainedResidualDBFS": round(db(residual), 4)})
        key = "nearEndOnly" if case_id == "near_end_only" else "doubleTalk"
        require(corr >= thresholds[key]["deterministicCorrelationMin"], failures, "near-end destruction sentinel failed")
        require(attenuation <= thresholds[key].get("nearEndAttenuationMaxDB", thresholds[key].get("absoluteLevelChangeMaxDB")), failures, "near-end attenuation exceeds ceiling")
        require(abs(level_change(near, output)) <= thresholds["nearEndOnly"]["absoluteLevelChangeMaxDB"] if case_id == "near_end_only" else True, failures, "near-end level change exceeds ceiling")
        if case_id == "near_end_only":
            input_residual = projected_residual(near, capture)[1]
            improvement = db(input_residual) - db(residual)
            metrics["noiseSuppressionImprovementDB"] = round(improvement, 4)
            require(improvement >= thresholds["noiseSuppression"]["snrImprovementMinDB"], failures, "noise suppression improvement below floor")
        else:
            coefficient_far, _ = projected_residual(far, [out - n for out, n in zip(output, near)])
            echo_residual = rms(gain(far, coefficient_far))
            erle = db(rms(gain(far, 0.25))) - db(echo_residual)
            metrics["doubleTalkERLEDB"] = round(erle, 4)
            require(erle >= thresholds["doubleTalk"]["medianERLEMinDB"], failures, "double-talk ERLE below floor")
    if case_id in {"clipping", "too_quiet"}:
        change = level_change(capture, output)
        metrics["gainChangeDB"] = round(change, 4)
        require(change <= thresholds["inputAGC"]["maximumGainDB"] + 0.01, failures, "AGC maximum gain exceeded")
        require(metrics["peakDBFS"] <= thresholds["inputAGC"]["peakMaxDBFS"] + 0.01, failures, "input AGC peak ceiling exceeded")
        require(metrics["clippedFraction"] <= thresholds["clipping"]["clippedSampleFractionMax"], failures, "clipped sample fraction exceeded")
        if case_id == "clipping":
            require(abs(metrics["outputRMSDBFS"] - thresholds["inputAGC"]["targetRMSDBFS"]) <= thresholds["inputAGC"]["targetToleranceDB"], failures, "input AGC target window missed")
    if case_id == "silence":
        require(rms(output) <= 1e-7, failures, "silence generated non-silent output")
    return metrics, failures


def validate_events(case_id: str, events: dict, spec: dict) -> list[str]:
    failures: list[str] = []
    route = spec["thresholds"]["routeChange"]
    if case_id in {"route_change", "device_loss", "missing_reference"}:
        require(events.get("committedSamplesDuringReconfigure") == 0, failures, "samples committed during reconfigure")
        require(events.get("generationAfter", 0) - events.get("generationBefore", 0) >= route["requiredGenerationAdvance"], failures, "capture generation did not advance")
        result = events.get("reconfigureResult")
        require(result in {"rearmed", "terminated_unsupported"}, failures, "invalid reconfigure result")
        if result == "rearmed":
            require(events.get("rearmMS", route["maximumRearmMS"] + 1) <= route["maximumRearmMS"], failures, "capture rearm exceeded deadline")
        else:
            require(events.get("terminalEvents") == 1, failures, "unsupported reconfigure must emit one terminal event")
    if case_id in {"processor_overrun", "effect_failure"}:
        require(events.get("committedSamplesAfterFailure") == 0, failures, "samples committed after processor failure")
        require(events.get("terminalEvents") == 1, failures, "processor failure must emit one terminal event")
    if case_id in {"clipping", "too_quiet"}:
        require(events.get("maximumGainChangeDBPerSecond", math.inf) <= spec["thresholds"]["inputAGC"]["maximumGainChangeDBPerSecond"], failures, "AGC gain slew exceeded")
    if case_id == "clock_drift":
        require(abs(events.get("driftPPM", 10**9)) <= spec["thresholds"]["clockDrift"]["fixtureRangePPM"], failures, "clock drift outside fixture range")
        require(events.get("referenceAgeMaxMS", 10**9) <= spec["thresholds"]["clockDrift"]["referenceAgeMaxMS"], failures, "render reference too old")
    if case_id == "live_packet_cancel":
        expected = spec["thresholds"]["livePacketCancel"]
        sequence = events.get("receivedSequences", [])
        sent = expected["sentPacketCount"]
        wanted = [item for item in range(sent) if item not in expected["lostSequences"]]
        measured_loss = 100 * (sent - len(sequence)) / sent if sent else 0
        require(sequence == wanted and measured_loss == expected["lossPercent"], failures, "packet-loss recipe mismatch")
        require(events.get("packetsAfterCancel", 1) <= expected["packetsAfterCancelMax"], failures, "packet emitted after cancel")
        require(events.get("emittedSequencesAfterCancel") == [], failures, "post-cancel packet trace is not empty")
        require(events.get("committedSamplesAfterCancel", 1) <= expected["committedSamplesAfterCancelMax"], failures, "sample committed after cancel")
        require(events.get("terminalEvents") == expected["terminalEventsExactly"], failures, "cancel terminal event count mismatch")
    return failures


def evaluate(candidate_path: pathlib.Path, corpus: pathlib.Path, lock_path: pathlib.Path) -> dict:
    spec, lock, candidate = contract(), load_json(lock_path), load_json(candidate_path)
    if lock != load_json(LOCK_PATH):
        raise HarnessError("fixture lock differs from the checked-in content lock")
    required = set(spec["candidateArtifact"]["requiredTopLevel"])
    if not required.issubset(candidate):
        raise HarnessError("candidate required top-level fields missing")
    if candidate.get("schemaVersion") != 1 or candidate.get("contract") != spec["contract"]:
        raise HarnessError("candidate schema or contract mismatch")
    identity = candidate.get("candidate")
    identity_fields = spec["candidateArtifact"]["requiredCandidateIdentity"]
    if not isinstance(identity, dict) or any(not isinstance(identity.get(key), str) or not identity[key] for key in identity_fields):
        raise HarnessError("candidate identity must bind platform, build, workflow and route")
    matrix = spec["workflowRouteMatrix"]
    if identity["platform"] not in {"windows", "macos", "harness-self-test"}:
        raise HarnessError("candidate platform outside frozen vocabulary")
    if identity["workflow"] not in matrix["workflows"] or identity["route"] not in matrix["routes"]:
        raise HarnessError("candidate workflow or route outside frozen matrix")
    expected_lock_hash = sha256_file(lock_path)
    if candidate.get("fixtureLockSHA256") != expected_lock_hash:
        raise HarnessError("candidate fixture lock hash mismatch")
    verify_corpus(corpus, lock)
    if not isinstance(candidate["cases"], list) or not all(isinstance(item, dict) for item in candidate["cases"]):
        raise HarnessError("candidate cases must be an array of objects")
    case_map = {item.get("id"): item for item in candidate["cases"]}
    expected_ids = [item["id"] for item in spec["fixtures"]]
    if set(case_map) != set(expected_ids) or len(case_map) != len(expected_ids):
        raise HarnessError("candidate case coverage mismatch")
    lock_map = {item["id"]: item for item in lock["fixtures"]}
    root = candidate_path.parent
    results, failures = [], []
    for case_id in expected_ids:
        record, fixture = case_map[case_id], lock_map[case_id]
        if (
            not isinstance(record.get("output"), str)
            or not isinstance(record.get("sha256"), str)
            or not isinstance(record.get("events"), dict)
        ):
            raise HarnessError(f"{case_id}: malformed output record")
        path = safe_child(root, record.get("output", ""))
        if sha256_file(path) != record.get("sha256"):
            raise HarnessError(f"{case_id}: candidate output hash mismatch")
        signals = {role: decode(safe_child(corpus, file["path"]), fixture["samples"])
                   for role, file in fixture["files"].items()}
        output = decode(path, fixture["samples"])
        metrics, case_failures = evaluate_audio(case_id, signals, output, spec)
        case_failures.extend(validate_events(case_id, record.get("events", {}), spec))
        results.append({"id": case_id, "passed": not case_failures, "metrics": metrics, "failures": case_failures})
        failures.extend(f"{case_id}: {message}" for message in case_failures)
    runtime = candidate.get("runtime", {})
    if not isinstance(runtime, dict):
        raise HarnessError("candidate runtime must be an object")
    processor = spec["thresholds"]["processor"]
    latency = numeric_series(runtime.get("addedLatencyMS"), "addedLatencyMS")
    cpu = numeric_series(runtime.get("cpuRealtimeRatios"), "cpuRealtimeRatios")
    allocations = nonnegative_number(runtime.get("callbackAllocations"), "callbackAllocations")
    waits = nonnegative_number(runtime.get("callbackBlockingWaits"), "callbackBlockingWaits")
    working_set = nonnegative_number(runtime.get("peakWorkingSetMiB"), "peakWorkingSetMiB")
    runtime_checks = [
        (percentile(latency, 0.95) <= processor["addedLatencyP95MaxMS"], "runtime: p95 latency exceeded"),
        (allocations == processor["callbackAllocationMax"], "runtime: callback allocation detected"),
        (waits == processor["callbackBlockingWaitMax"], "runtime: callback blocking wait detected"),
        (percentile(cpu, 0.95) <= processor["cpuRealtimeRatioP95Max"], "runtime: p95 realtime CPU ratio exceeded"),
        (working_set <= processor["peakWorkingSetMiBMax"], "runtime: working-set budget exceeded"),
        (runtime.get("measurementSource") in {"synthetic-self-test", "exact-build-adapter"}, "runtime: invalid measurement source"),
    ]
    failures.extend(message for passed, message in runtime_checks if not passed)
    report = {
        "schemaVersion": 1, "contract": spec["contract"], "task": spec["task"],
        "candidate": candidate.get("candidate"), "fixtureLockSHA256": expected_lock_hash,
        "passed": not failures, "results": results, "failures": failures,
        "runtime": {
            "addedLatencyP95MS": percentile(latency, 0.95),
            "callbackAllocations": allocations,
            "callbackBlockingWaits": waits,
            "cpuRealtimeRatioP95": percentile(cpu, 0.95),
            "peakWorkingSetMiB": working_set,
            "measurementSource": runtime.get("measurementSource"),
        },
        "claimBoundary": spec["claimBoundary"], "manualEvidence": "not-run",
    }
    return report


def conforming_output(case_id: str, signals: dict[str, list[float]]) -> list[float]:
    if case_id in {"far_end_only", "echo_path_change", "clock_drift"}:
        return gain(signals["capture"], 0.02)
    if case_id in {"near_end_only", "double_talk"}:
        return signals["near"]
    if case_id in {"clipping", "too_quiet"}:
        source = signals["capture"]
        current = rms(source)
        factor = min(10 ** (12 / 20), (10 ** (-20 / 20)) / current) if current else 1.0
        peak_factor = (10 ** (-3 / 20)) / max((abs(item) for item in source), default=1.0)
        return gain(source, min(factor, peak_factor))
    return [0.0] * len(signals["capture"])


def demo_candidate(output: pathlib.Path, corpus: pathlib.Path, lock_path: pathlib.Path, mutation: str) -> pathlib.Path:
    if output.exists():
        raise HarnessError(f"output already exists: {output}")
    output.mkdir(parents=True)
    spec, lock = contract(), load_json(lock_path)
    verify_corpus(corpus, lock)
    cases = []
    for fixture in lock["fixtures"]:
        signals = {role: decode(safe_child(corpus, file["path"]), fixture["samples"])
                   for role, file in fixture["files"].items()}
        case_id = fixture["id"]
        values = conforming_output(case_id, signals)
        events = {
            "committedSamplesDuringReconfigure": 0, "generationBefore": 4,
            "generationAfter": 5, "reconfigureResult": "rearmed", "rearmMS": 120,
            "committedSamplesAfterFailure": 0,
            "terminalEvents": 1, "driftPPM": 200, "referenceAgeMaxMS": 80,
            "maximumGainChangeDBPerSecond": 2.5,
            "receivedSequences": [item for item in range(100) if item not in {17, 73}],
            "packetsAfterCancel": 0, "emittedSequencesAfterCancel": [],
            "committedSamplesAfterCancel": 0,
        }
        if mutation == "bypass" and case_id == "far_end_only": values = signals["capture"]
        if mutation == "destruction" and case_id == "near_end_only": values = [0.0] * len(values)
        if mutation == "ceiling" and case_id == "clipping": values = signals["capture"]
        if mutation == "lifecycle" and case_id == "route_change": events["committedSamplesDuringReconfigure"] = 480
        if mutation == "packet_cancel" and case_id == "live_packet_cancel": events["packetsAfterCancel"] = 1
        path = output / f"{case_id}.output.f32le"
        path.write_bytes(encode(values))
        cases.append({"id": case_id, "output": path.name, "sha256": sha256_file(path), "events": events})
    runtime = {
        "addedLatencyMS": [4.0, 4.5, 5.0, 5.5, 6.0], "callbackAllocations": 0,
        "callbackBlockingWaits": 0, "cpuRealtimeRatios": [0.08, 0.10, 0.12],
        "peakWorkingSetMiB": 24.0, "measurementSource": "synthetic-self-test",
    }
    if mutation == "realtime": runtime["callbackBlockingWaits"] = 1
    candidate = {
        "schemaVersion": 1, "contract": spec["contract"], "fixtureLockSHA256": sha256_file(lock_path),
        "candidate": {
            "platform": "harness-self-test", "build": mutation,
            "workflow": "recorded_clip", "route": "speaker",
        },
        "cases": cases, "runtime": runtime,
    }
    path = output / "candidate.json"
    path.write_bytes(json.dumps(candidate, indent=2, sort_keys=True).encode() + b"\n")
    return path


def main() -> int:
    parser = argparse.ArgumentParser()
    subparsers = parser.add_subparsers(dest="command", required=True)
    generate_parser = subparsers.add_parser("generate")
    generate_parser.add_argument("--output", type=pathlib.Path, required=True)
    generate_parser.add_argument("--lock", type=pathlib.Path)
    demo_parser = subparsers.add_parser("demo-candidate")
    demo_parser.add_argument("--output", type=pathlib.Path, required=True)
    demo_parser.add_argument("--corpus", type=pathlib.Path, required=True)
    demo_parser.add_argument("--lock", type=pathlib.Path, default=LOCK_PATH)
    demo_parser.add_argument("--mutation", choices=("conforming", "bypass", "destruction", "ceiling", "realtime", "lifecycle", "packet_cancel"), default="conforming")
    evaluate_parser = subparsers.add_parser("evaluate")
    evaluate_parser.add_argument("--candidate", type=pathlib.Path, required=True)
    evaluate_parser.add_argument("--corpus", type=pathlib.Path, required=True)
    evaluate_parser.add_argument("--lock", type=pathlib.Path, default=LOCK_PATH)
    evaluate_parser.add_argument("--report", type=pathlib.Path)
    args = parser.parse_args()
    if args.command == "generate":
        result = generate_corpus(args.output, args.lock)
        print(json.dumps({"fixtures": len(result["fixtures"]), "output": str(args.output)}, sort_keys=True))
        return 0
    if args.command == "demo-candidate":
        print(demo_candidate(args.output, args.corpus, args.lock, args.mutation))
        return 0
    report = evaluate(args.candidate, args.corpus, args.lock)
    rendered = json.dumps(report, indent=2, sort_keys=True) + "\n"
    if args.report:
        args.report.write_text(rendered, encoding="utf-8")
    print(rendered, end="")
    return 0 if report["passed"] else 1


if __name__ == "__main__":
    try:
        raise SystemExit(main())
    except (HarnessError, OSError, ValueError, KeyError, json.JSONDecodeError) as error:
        print(f"capture-quality harness: {error}", file=sys.stderr)
        raise SystemExit(2)

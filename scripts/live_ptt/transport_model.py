#!/usr/bin/env python3
"""Deterministic live-audio transport budget and bounded-queue model.

This is an engineering model, not physical-network or listening evidence. It
keeps assumptions explicit so later real-hardware evidence can replace them.
"""

from __future__ import annotations

import argparse
import json
import math
import pathlib
import random
from dataclasses import dataclass


SCHEMA_VERSION = 1
DEFAULT_SEED = 0xB4A7CE


def nearest_rank(values: list[float], probability: float) -> float:
    ordered = sorted(values)
    index = max(0, min(len(ordered) - 1, math.ceil(probability * len(ordered)) - 1))
    return round(ordered[index], 3)


@dataclass
class PathState:
    hol_until_ms: float = 0.0
    last_arrival_ms: float = 0.0
    network_losses: int = 0
    hol_events: int = 0


def path_arrival(
    sent_ms: float,
    rng: random.Random,
    transport: str,
    state: PathState,
    loss_percent: float,
) -> float | None:
    # Conservative synthetic two-home path: 45-90 ms base one-way latency,
    # bounded jitter, and an occasional 90-220 ms scheduling/radio spike.
    one_way_ms = rng.uniform(45.0, 90.0) + rng.triangular(0.0, 35.0, 7.0)
    if rng.random() < 0.012:
        one_way_ms += rng.uniform(90.0, 220.0)
    lost = rng.random() < loss_percent / 100.0
    if lost:
        state.network_losses += 1
        if transport == "quic-datagram":
            return None
        # TCP recovers but blocks all following bytes on this connection.
        recovery_ms = sent_ms + one_way_ms + rng.uniform(75.0, 260.0)
        state.hol_until_ms = max(state.hol_until_ms, recovery_ms)
        state.hol_events += 1
    arrival_ms = sent_ms + one_way_ms
    if transport == "wss-tcp":
        arrival_ms = max(arrival_ms, state.hol_until_ms, state.last_arrival_ms)
        state.last_arrival_ms = arrival_ms
    return arrival_ms


def simulate_transport(
    *,
    frame_ms: int,
    transport: str,
    frames: int = 20000,
    seed: int = DEFAULT_SEED,
    loss_percent: float = 2.0,
) -> dict:
    if frame_ms not in (10, 20):
        raise ValueError("frame_ms must be 10 or 20")
    if transport not in ("wss-tcp", "quic-datagram"):
        raise ValueError("unsupported transport")
    if frames < 1000:
        raise ValueError("at least 1000 frames required")

    rng = random.Random(seed + frame_ms + (0 if transport == "wss-tcp" else 1000))
    uplink = PathState()
    downlink = PathState()
    arrivals: list[float | None] = []
    relay_ms = 4.0
    encode_ms = 0.20 if frame_ms == 10 else 0.26
    decode_ms = 0.12 if frame_ms == 10 else 0.17
    for sequence in range(frames):
        capture_started_ms = sequence * frame_ms
        sent_ms = capture_started_ms + frame_ms + encode_ms
        at_relay = path_arrival(sent_ms, rng, transport, uplink, loss_percent)
        if at_relay is None:
            arrivals.append(None)
            continue
        at_client = path_arrival(at_relay + relay_ms, rng, transport, downlink, loss_percent)
        arrivals.append(at_client)

    latencies: list[float] = []
    fec_recovered = 0
    plc_concealed = 0
    delivered = 0
    reordered = 0
    prior_arrival: float | None = None
    jitter_buffer_ms = 60.0
    for sequence, arrival in enumerate(arrivals):
        capture_started_ms = sequence * frame_ms
        if arrival is not None:
            delivered += 1
            if prior_arrival is not None and arrival < prior_arrival:
                reordered += 1
            prior_arrival = arrival
            render_ms = arrival + jitter_buffer_ms + decode_ms
        elif sequence + 1 < len(arrivals) and arrivals[sequence + 1] is not None:
            # Opus in-band FEC in packet N can reconstruct packet N-1. It is
            # useful only when the unreliable transport exposes that loss.
            fec_recovered += 1
            render_ms = arrivals[sequence + 1] + jitter_buffer_ms + decode_ms
        else:
            # A bounded jitter buffer invokes Opus PLC at the scheduled playout
            # point; this model records concealment, not an intelligibility pass.
            plc_concealed += 1
            render_ms = capture_started_ms + frame_ms + 2 * 90.0 + relay_ms + jitter_buffer_ms + decode_ms
        latencies.append(render_ms - capture_started_ms)

    return {
        "frameMs": frame_ms,
        "transport": transport,
        "frames": frames,
        "seed": seed,
        "injectedLossPercentPerLeg": loss_percent,
        "latencyMs": {
            "p50": nearest_rank(latencies, 0.50),
            "p95": nearest_rank(latencies, 0.95),
            "p99": nearest_rank(latencies, 0.99),
            "max": nearest_rank(latencies, 1.0),
        },
        "network": {
            "uplinkLossEvents": uplink.network_losses,
            "downlinkLossEvents": downlink.network_losses,
            "tcpHolEvents": uplink.hol_events + downlink.hol_events,
            "applicationDelivered": delivered,
            "fecRecovered": fec_recovered,
            "plcConcealed": plc_concealed,
            "arrivalReorders": reordered,
        },
        "budgetModelPass": nearest_rank(latencies, 0.50) <= 800.0 and nearest_rank(latencies, 0.95) <= 1500.0,
        "intelligibilityClaim": False,
    }


def queue_isolation(frame_ms: int = 20, frames: int = 2000, capacity: int = 8) -> dict:
    if capacity < 1:
        raise ValueError("capacity must be positive")

    def run(service_ms: float) -> dict:
        queue: list[int] = []
        next_service = 0.0
        dropped = 0
        max_depth = 0
        served = 0
        for sequence in range(frames):
            now = float(sequence * frame_ms)
            while queue and next_service <= now:
                queue.pop(0)
                served += 1
                next_service += service_ms
            if len(queue) == capacity:
                queue.pop(0)
                dropped += 1
            queue.append(sequence)
            max_depth = max(max_depth, len(queue))
        return {"served": served, "droppedOldest": dropped, "maxDepth": max_depth, "remaining": len(queue)}

    fast = run(float(frame_ms))
    slow = run(float(frame_ms * 6))
    return {
        "policy": "per-recipient bounded queue; drop oldest unsent live frame; never block another recipient",
        "capacityFrames": capacity,
        "capacityMs": capacity * frame_ms,
        "fastRecipient": fast,
        "slowRecipient": slow,
        "isolated": fast["droppedOldest"] == 0 and slow["droppedOldest"] > 0 and slow["maxDepth"] <= capacity,
    }


def build_artifact(frames: int = 20000, seed: int = DEFAULT_SEED) -> dict:
    profiles = [
        simulate_transport(frame_ms=frame_ms, transport=transport, frames=frames, seed=seed)
        for frame_ms in (10, 20)
        for transport in ("wss-tcp", "quic-datagram")
    ]
    return {
        "schemaVersion": SCHEMA_VERSION,
        "contract": "p3-live-transport-budget-model.v1",
        "claimBoundary": "deterministic budget/backpressure model only; not real network, hardware, package, audio-quality, or production evidence",
        "assumptions": {
            "sampleRateHz": 48000,
            "channels": 1,
            "codec": "libopus 1.6.1 engineering candidate",
            "bitrateBps": 24000,
            "jitterBufferMs": 60,
            "relayProcessingMs": 4,
            "pathModel": "independent 45-90 ms one-way base plus bounded jitter, 1.2 percent 90-220 ms spikes, and 2 percent loss per leg",
            "wssLossRule": "TCP retransmits and later bytes wait behind the missing segment",
            "datagramLossRule": "no retransmit; one immediately preceding frame may use Opus in-band FEC, otherwise PLC",
        },
        "profiles": profiles,
        "slowRecipient": queue_isolation(),
    }


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--output", type=pathlib.Path, required=True)
    parser.add_argument("--frames", type=int, default=20000)
    parser.add_argument("--seed", type=int, default=DEFAULT_SEED)
    args = parser.parse_args()
    artifact = build_artifact(args.frames, args.seed)
    args.output.parent.mkdir(parents=True, exist_ok=True)
    args.output.write_text(json.dumps(artifact, indent=2) + "\n", encoding="utf-8")
    print(args.output)
    return 0


if __name__ == "__main__":
    raise SystemExit(main())

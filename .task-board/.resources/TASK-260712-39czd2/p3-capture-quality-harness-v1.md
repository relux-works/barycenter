# Capture quality synthetic regression harness v1

`TASK-260712-39czd2` provides a deterministic, content-addressed conformance
harness for capture processors. It is deliberately not a microphone recorder,
an acoustic test, or evidence that either production application passes C3.
The manual status remains `not-run` in `EPIC-260714-th54l3`.

## Trust boundary

The harness generates mono 48 kHz float32 non-speech signals from fixed integer
phase recipes and an integer LCG. No downloaded, user, device, microphone, file
intake, or private audio is accepted. The checked-in lock records every fixture
role's byte length and SHA-256. A candidate bundle binds that exact lock and
every output by SHA-256; absolute paths, traversal, symlinks, wrong lengths,
non-finite samples, and out-of-range samples are rejected before scoring.

The evaluator recomputes sample-domain metrics. It does not accept adapter
claims for ERLE, level, correlation, clipping, or noise improvement. Runtime and
lifecycle values are receipts because they must originate at the exact-build
adapter seam. A `synthetic-self-test` receipt only proves that evaluator failure
paths work; only `exact-build-adapter` is eligible for later platform evidence.

The deterministic correlation metric is a cheap destruction sentinel. The
canonical STOI calculation and blinded listening frozen in the parent contract
remain manual acceptance work and are not replaced or declared passed here.

## Reproducible run

From the repository root, generate the corpus into a new temporary directory:

```sh
python3 scripts/capture_quality/harness.py generate \
  --output .temp/capture-quality/corpus \
  --lock .temp/capture-quality/lock.json
cmp .temp/capture-quality/lock.json \
  acceptance/phase3/capture-quality-harness-lock-v1.json
```

An exact-build adapter consumes that corpus and writes `candidate.json` plus one
exact-length output per case. The schema and thresholds are frozen in
`acceptance/phase3/capture-quality-harness-v1.json`. Evaluate it with:

```sh
python3 scripts/capture_quality/harness.py evaluate \
  --candidate PATH/candidate.json \
  --corpus .temp/capture-quality/corpus \
  --lock .temp/capture-quality/lock.json \
  --report .temp/capture-quality/report.json
```

The report exits zero only when every applicable fixture and every runtime
budget passes independently. Results are not averaged across workflow, route,
platform, or case. Retain the report and hashes; remove generated audio after
the report is bound. Human audio is forbidden in this workflow.

## Harness self-test

Run:

```sh
PYTHONDONTWRITEBYTECODE=1 python3 -m unittest \
  scripts/capture_quality/test_harness.py
```

The suite regenerates and compares the lock, proves a conforming synthetic
bundle, then requires categorical failures for bypass, near-end destruction,
input ceiling/clipping, callback blocking, generation reuse, packet emission
after cancel, hash tampering, traversal, and invented manual evidence.

`demo-candidate` exists only to test the harness. Its platform is always
`harness-self-test`; its CPU, memory, latency, DSP and lifecycle receipts must
never be reused as application evidence.

## Downstream adapter handoff

The macOS and Windows effect tasks must add exact-build adapters without
changing this common evaluator or weakening thresholds. Each adapter must emit
the requested/resolved route and workflow cell independently, use monotonic
generation receipts around route/device/reference changes, and prove zero
post-cancel samples or packets. Signed-app acoustic, hardware-route, canonical
STOI, blinded-listening, physical resource, indicator, and accessibility
results stay in the manual-test epic.

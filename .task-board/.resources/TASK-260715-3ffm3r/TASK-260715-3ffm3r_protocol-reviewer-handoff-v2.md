# Phase 1 independent protocol reviewer handoff v2

- Preparation task: `TASK-260715-3ffm3r` — Approve Phase 1 independent protocol review
- Original task: `TASK-260712-176b74` — Independent Phase 1 protocol and compatibility review
- Prepared: 2026-07-19
- Accepted Phase 1 merge: `524eb78e2d768ade1628d9170654f5f9c9d06e4b`
- Exact later `main` candidate: `191ae26325ba34d32c94358044635fb7a73651e2`
- Machine authority: `acceptance/phase1/protocol-independent-review-handoff-v2.json`
- Decision: packet ready; independent verdict not recorded

## Why the original packet needs this delta

The 2026-07-15 technical audit correctly froze the Phase 1 implementation at
PR #68 and enumerated 39 message goldens. The repository subsequently added
P2 streamed tracks, P3 live PTT and an optional capture-quality object on the
shared `state` payload. Current `main` therefore has 59 goldens. Sending only
the old packet would leave a reviewer unable to tell whether later additive
work changed the original Phase 1 compatibility surface.

This handoff does not perform or claim the required independent review. It
pins the complete authority delta so a non-implementing reviewer can issue an
exact, reproducible verdict without trusting the implementation session's
summary.

## Frozen delta

The review range is `524eb78..191ae263` over these authority paths:

- `coordinator/internal/protocol`
- `pulsar-win/wire`
- `node-app/Sources/NodeCore/Protocol.swift`
- `node-app/Tests/NodeCoreTests/ProtocolContractTests.swift`
- `protocol`
- `docs/protocol.md`
- `docs/analysis/p1-clip-transmission-wire-contract.md`

The range contains 51 changed paths, 4,610 additions and 25 deletions. The
machine packet pins SHA-256 digests for both `git diff --name-status` and
`git diff --numstat`, plus Git object IDs for every current codec, golden and
contract-test authority.

## Golden classification

All 39 original golden filenames remain present. Thirty-eight are byte
unchanged. The only modified original file is `protocol/golden/state.json`; it
adds the optional `capture_quality` object and removes or renames no Phase 1
field. The 20 new files are closed additive families:

- 12 `stream_*` messages for P2 streamed tracks;
- 8 `live_ptt_*` messages for P3 near-live push-to-talk.

This classification is input to review, not the verdict. The reviewer must
independently confirm that older clients ignore unknown v1 messages, reject
incompatible envelope majors, and do not accidentally advertise or enter the
new capabilities.

## Required reviewer work

1. State name and confirm no implementation of the reviewed protocol or
   scheduler paths.
2. Check out exact candidate `191ae26325ba34d32c94358044635fb7a73651e2`.
3. Verify the machine packet, then inspect the complete 51-path delta.
4. Re-sample all 39 original message mappings and the closed enums, including
   the additive `state.capture_quality` object.
5. Review the 20 additive messages and capability/version isolation.
6. Confirm P1-PROTO-001 remains closed without breaking unknown-type forward
   compatibility.
7. Record every finding, rerun after fixes, and write an approve or reject
   verdict naming the exact revision.

Recommended reproducible commands:

```bash
python3 scripts/acceptance/validate_p1_protocol_review_handoff.py
(cd coordinator && go test ./internal/protocol)
(cd pulsar-win && go test ./wire)
(cd node-app && swift test --filter ProtocolContractTests)
```

The original task remains `to-review` until this work is performed by a
genuinely non-implementing reviewer. Repository checks do not substitute for
that identity and judgment. Physical playback, timing and packaged-app
observations remain in `EPIC-260714-th54l3` — Manual real-app hardware
testing. Nothing in this packet authorizes production activation, Store
submission or a release claim.

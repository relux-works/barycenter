# P3 macOS live PTT node integration

Task: `TASK-260712-2kj9kj`

Status: engineering integration; `live_ptt_v1` remains unadvertised and production-disabled.

## Integrated boundary

`MacLivePTTNode` serializes the macOS sender, jitter receiver and frozen control
protocol behind injected target-snapshot and incoming-policy decisions. A local
hold requests a sealed `live_ptt_start`; capture opens only after a validated
accept for the same session and generation. Incoming starts are rejected while
the node is sending or receiving, and DND/block/policy decisions are supplied by
the existing authority rather than inferred locally.

Validated binary `BP` frames enter the receiver seam without passing through
JSON. `CoordinatorClient` now has a capability-gated, authenticated and bounded
binary send/receive seam with at most eight sends in flight. It remains fail
closed because the shipping registration does not advertise `live_ptt_v1` and
the app does not construct the node.

The status projection exposes direction, phase, session/generation, accepted
and rejected receiver counts, terminal error and clip fallback for a future
main-window/menu binding. Status callbacks run off the state queue so UI code
cannot re-enter and deadlock the node.

## Input and lifecycle safety

The node accepts button/menu/shortcut hold begin, heartbeat and release hooks.
It does not add an event tap, poll input, request Accessibility access or convert
the existing Carbon toggle shortcut into an unsafe simulated hold. When the
shell cannot prove a release-capable input, the sender returns the existing clip
recording fallback without opening the microphone.

Local Stop, release, coordinator cancel/failure, sleep, lock, disconnect,
permission recheck, feature rollback and quit converge on sender/receiver
teardown. Session and generation checks ignore stale accepts and frames; a
second concurrent direction is rejected. Terminal controls use only the exact
frozen end/cancel/failure vocabulary and are validated before transmission.

## Deterministic evidence

Swift tests cover disabled fallback, matching and stale accepts, concurrent
direction rejection, injected DND policy, binary routing through buffering and
playback, generation-bound terminal cleanup, all system teardown hooks, bounded
transport classification, and the absence of production wiring/capability
advertisement. Sender tests additionally validate every emitted terminal
control against the frozen wire contract.

These checks prove the integration state machine and bounded seams. They do not
prove a global key-down/key-up path, real microphone/output behavior, audible
ducking, two-home interoperability, sleep/lock delivery or signing on physical
hardware. That matrix remains in `TASK-260712-1rzqh9` under
`EPIC-260714-th54l3`.

## Activation and rollback

Activation still requires a reviewed signed libopus supply path, exact codec
profile evidence, shell composition with authoritative target/DND state and the
manual two-home matrix. Until then, capability advertisement and construction
must remain absent. Rollback is removal of the future composition plus
capability withdrawal; the existing Phase 1/2 clip path remains unchanged.

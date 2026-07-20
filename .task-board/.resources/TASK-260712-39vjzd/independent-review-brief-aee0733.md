# Independent review brief — TASK-260712-39vjzd

Review exact producer commit
`aee07339bcfe014b39edac10734f713d11333792` for
`TASK-260712-39vjzd — windows-e2ee-live-ptt` against accepted main merge
`e47eb6b583fa0319beee460b87397bdb75dbcf39`.

Act as the independent security, protocol, lifecycle, concurrency and realtime
reviewer. Do not modify production code. Temporary adversarial tests are
welcome but must be removed before the verdict. Audit the exact producer diff,
the full current files, accepted macOS live-E2EE model, Windows key-state
authority, design review, vectors, ADR, packet, validators and every hash pin.

Acceptance requires zero open Critical/High/Medium. A deterministic audit
fixture, blind cross-build or local test is not manual real-app evidence.

## Required review focus

1. Confirm the Windows `BE` encoder/decoder is byte-exact with
   `coordinator/internal/e2eecontract/opaque_live.go` and the accepted macOS
   vector. `BP` must never be accepted as a protected frame; no wire delta may
   be hidden in this task.
2. Confirm cross-device AAD is byte-exact with the accepted macOS ordering and
   binds shared epoch + `commitDigest`, never the device-local repository
   revision. Reproduce the two-installation fixture with deliberately skewed
   local revisions and exact protect→open success.
3. Audit the outgoing factory order: witnessed identity/group load, exact local
   CAS and target check, cross-process share-none `live_ptt` generation
   reservation, advanced-state reload, unchanged epoch/commit/target, then
   derivation. Ambiguous reservation success must remain consumed, not retried.
4. Audit provider ownership adversarially. Inputs, nonce and ciphertext outputs,
   cached retry frames, returned opaque frames and opened plaintext must not be
   aliasable by provider, transport or caller mutation. Service-owned plaintext
   must be copied before zeroing; the provider/caller alias fixture must fail on
   an unsafe copy order.
5. Verify retry returns exact ciphertext/nonce without reseal; sequence 15001 is
   rejected before seal; malformed provider output is distinct from nonce
   reuse; outgoing and incoming nonce reuse, tamper, replay and out-of-window
   input terminate fail closed.
6. Verify current authorization is checked before every seal/open. Changed
   epoch, commit, target or sender membership must destroy the provider session
   exactly once, revoke the receiver and never admit bytes to Opus/FEC/PLC.
   Inspect explicit close, error teardown and finalizer behavior for leaks or
   double-destroy races.
7. Confirm sealing is injectable only at the existing transport worker after
   the bounded capture queue, while opening is before
   `WindowsLiveJitterReceiver.Receive`. Existing DND/policy admission,
   backpressure, 8-frame reorder, 60 ms prebuffer, FEC/PLC, PCM bounds and
   teardown semantics must remain unchanged.
8. Confirm production darkness: public factory/channel require an independently
   approved provider; the audit constructor is unexported; no provider, KDF,
   AEAD, nonce algorithm, suite, library, runtime, UI or capability is selected
   or wired. Search every non-test Windows source for unexpected composition.
9. Recompute all 11 acceptance-packet artifact hashes. Confirm the Windows
   key-state validator exception is narrowly limited to this exact dormant
   witnessed boundary, and that existing macOS/key-state/opaque-router packets
   remain frozen.
10. Keep checklist item 5 open. Real coordinator traffic capture, signed MSIX,
    native DPAPI/NTFS, provider/crypto/codec, microphone/speaker, latency,
    memory/crash/swap/backup and macOS-Windows hardware interop stay `not-run`
    in `EPIC-260714-th54l3`.

## Producer evidence to reproduce

```sh
cd pulsar-win
go test -count=1 -run 'TestWindowsE2EELive' ./...
go test -race -count=1 -run 'TestWindowsE2EELive' ./...
go test -count=1 -run 'TestWindowsLive(Capture|Receiver|PTTNode)' ./...
go test -race -count=1 -run 'TestWindowsLive(Capture|Receiver|PTTNode)' ./...
go test -count=1 ./...
go test -race -count=1 ./...
go vet ./...
CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go test -exec /usr/bin/true ./...
CGO_ENABLED=0 GOOS=windows GOARCH=arm64 go test -exec /usr/bin/true ./...
cd ..
PYTHONDONTWRITEBYTECODE=1 python3 -m unittest discover -s scripts/acceptance -p 'test_*.py'
PYTHONDONTWRITEBYTECODE=1 python3 scripts/acceptance/run_automated.py --suite all
```

Producer results: focused 11 scenarios plus race; live regressions plus race;
full Go plus race; vet; Windows amd64/arm64 blind compile; acceptance 215/215;
automated harness 16/16 at
`.temp/acceptance/20260720T065018Z/manifest.json`.

Before verdict, prove `git diff aee0733..HEAD` is tracking-only. Save the full
verdict as a new task outcome resource. Terminal verdict must be exactly
`ACCEPTED` or `REJECTED`. Set the task `done` only for `ACCEPTED` with zero open
Critical/High/Medium; otherwise return it to `to-dev` and include every
reproduction. Do not claim production E2EE or manual evidence.

# Flight Logbook

> Institutional memory. Concise, factual, high-signal.
> Newest entries first. One block per insight.

## 2026-07-06

### 1641 — EPIC B: pulsar-win Windows shell skeleton shipped (blind build)
- MILESTONE: commit ea5321b pushed; CI run 28792158823 fully green (node-core, pulsar-win, coordinator).
- FINDING: Go internal-package rules block importing `coordinator/internal/protocol` from a sibling module even with require+replace ("use of internal package … not allowed", empirically probed).
- DECISION: `pulsar-win/wire/` = verbatim mirror of the protocol package, pinned by two tests: golden round-trip of every `protocol/golden/*.json` + `TestMirrorMatchesCoordinatorSource` (pulsar-win/wire/golden_test.go).
- NOTE: mirror compare is gofmt-normalized because `coordinator/internal/protocol/codec.go` is itself not gofmt-clean and coordinator/ belongs to the EPIC A agent — do not "fix" it from pulsar-win.
- NOTE: CROSS-AGENT TRIPWIRE — any coordinator protocol change now fails the `pulsar-win` CI job on main until wire/ is re-copied (cat mirror header + source, `gofmt -w`). Intended keep-in-sync mechanism, not a flake.
- FINDING: reproducible Windows build verified bit-identical locally — sha256 `c75fa3c0…` twice with `CGO_ENABLED=0 -trimpath -ldflags "-s -w -buildid="`.
- FINDING: go-wca v0.3.0 exports `AUDCLNT_STREAMFLAGS_AUTOCONVERTPCM`, so the WASAPI render loop runs at pipeline-native 44100/2/f32 with no local resampler (pulsar-win/audio_windows.go).
- STATUS: portable parts unit-tested (+race); WASAPI/named-pipe/AppContainer legs untestable until a Windows machine (U-W1..U-W5); fork's Windows pipe output driver = P1, not built yet.

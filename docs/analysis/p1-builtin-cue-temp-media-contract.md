# P1 builtin cue and temporary media contract

`pulsar.capture-media-lifecycle.v1` freezes one recording cue and one cross-platform local-media lifecycle for the later macOS and Windows capture tasks. The machine-readable source of truth is `protocol/capture-media-lifecycle-v1.json`; Swift and Go conformance tests execute every listed transition.

## Reviewed cue

The cue is generated deterministically by `scripts/generate-recording-cue.go`. It contains no samples, voice, model output, or third-party audio. Its provenance is recorded in `assets/audio/pulsar-recording-cue.json` under the repository MIT license.

- PCM WAV, signed 16-bit little-endian, mono, 48 kHz
- 7,680 frames / 160 ms / 15,404 bytes
- SHA-256 `479b1a9d605ac12454e3449e129991b7ce8599251506ca54a93be0b6144730fd`
- macOS package path: `Contents/Resources/Audio/pulsar-recording-cue.wav`
- Windows package path: `Assets/Audio/pulsar-recording-cue.wav`

Both loaders reject missing, corrupted, reformatted, or substituted bytes with a fixed content-free error. Both package builders verify the digest before copying the asset. Audible quality and playback on real output hardware remain manual-test evidence; the coding gate claims only deterministic provenance, exact bytes, packaging, and loader behavior.

## Lifecycle and durability

| Storage class | Final state | Network | Restart | Removal |
| --- | --- | --- | --- | --- |
| self-test | `self_test_local` | forbidden | deleted during recovery | close, explicit delete, or recovery |
| user recording / picker intake | `durable_unsent` | allowed only from this state | retained when structurally valid | confirmed upload or explicit delete |

Every capture starts as an owner-only, opaque `<128-bit-id>.partial.wav` under the app-private root. A partial is neither sendable nor visible as history. Stop moves it to `finalizing`; structural validation and file synchronization must succeed before an atomic rename creates a finalized file. Cancel, failed finalization, and startup recovery delete partials. Recovery also deletes finalized self-tests and invalid/truncated drafts, while retaining structurally valid durable drafts.

Picker intake copies approved bytes into a newly allocated private partial. macOS closes security-scoped access immediately after reading the external source and before finalization; the Windows/core API accepts a reader after the UI has closed its external token. Source paths and filenames are never retained.

## Cue sequencing

Start sequence: open the capture session with microphone commit disabled, play the cue, then enable commit only after cue completion. Stop sequence: disable commit, close and finalize the writer, then play the stop cue. The cue sequencers expose no transition that can commit microphone samples during either cue.

## Privacy and failure copy

Logs may contain storage class, lifecycle state, fixed operation code, and opaque draft ID. They must never contain an absolute path, source filename, external security token, media bytes, or upload token. Filesystem failures are mapped to fixed errors without embedded paths.

Approved user-facing copy:

| Condition | English | Russian |
| --- | --- | --- |
| cue unavailable | The recording cue is unavailable. Recording did not start. | Сигнал записи недоступен. Запись не началась. |
| upload unavailable | Upload is unavailable. Your recording is saved on this device for retry. | Загрузка недоступна. Запись сохранена на этом устройстве для повторной попытки. |
| cleanup failed | The recording could not be removed. Try again before signing out or uninstalling. | Не удалось удалить запись. Повтори попытку до выхода или удаления приложения. |

## Automated evidence boundary

Swift and Go tests cover contract parity, cue validation, cue sequencing, cancel/finalization cleanup, recovery, durable restart behavior, picker copying, and path-free errors. Hosted macOS and Windows builds cover production compilation and package-source checks. Real microphone capture, audible cue assessment, uninstall behavior, and hardware restart/outage exercises belong to the separate manual-testing epic.

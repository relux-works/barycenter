# P1 Main UI, Local Self-Test and Capture Decomposition

## Spec slices reviewed

- `docs/spec-self-contained-audio.md` sections 4.1-5.3, 7.1-7.3, 11.1-11.2,
  14.1-14.4, 15.1-15.4, 16, 19.1-19.6
- `docs/goal-self-contained-audio.md`
- `docs/spec.md` and `docs/protocol.md` for current shipped behavior

## Current implementation snapshot

- Windows (`pulsar-win`) currently has onboarding, tray plumbing, playback
  state and voice-cache logic, but no Phase 1 main window, no microphone
  capture path, no hotkey flow, no file picker/drop path and no self-test.
- macOS (`node-app`) currently has onboarding and a status menu centered on the
  Spotify-era flow, but no Phase 1 main window, no capture/self-test path, no
  file picker/drop path and no configurable recording hotkey.
- Coordinator currently exposes `/pair`, `/ws` and `/media/...`; the Phase 1
  self-service onboarding, upload, transmission, receipt, report and history
  APIs from section 11 are not implemented yet.

## Story tasks created

- `TASK-260712-2lrpc0` Builtin cue asset and temp media contract
- `TASK-260712-9i5se7` Windows main window and tray shell
- `TASK-260712-2w4gyw` Windows microphone capture engine
- `TASK-260712-25at8b` Windows local self-test and short-file intake
- `TASK-260712-c7dmv8` Windows tray hotkey recording controller
- `TASK-260712-1p8ykc` Windows capture/self-test/hotkey integration
- `TASK-260712-2fe5bz` Windows routing, presence, history and failure integration
- `TASK-260712-1c04pk` macOS main window and menu bar shell
- `TASK-260712-30abcm` macOS microphone capture engine
- `TASK-260712-3lg0ht` macOS local self-test and short-file intake
- `TASK-260712-ut6akw` macOS menu-bar hotkey recording controller
- `TASK-260712-1s6h6t` macOS capture/self-test/hotkey integration
- `TASK-260712-3dqc3l` macOS routing, presence, history and failure integration
- `TASK-260712-e5mfqj` Cross-platform UI accessibility, DPI and acceptance evidence

## Within-story dependency chain

- Windows signed hardware spike -> Windows capture engine
- Shared cue/draft contract + platform engine -> each exact local self-test
- Platform engine + shell -> shortcut controller
- Engine + self-test/file intake + shortcut controller -> platform integration
- Platform capture integration -> platform live data integration
- Both live integration tasks -> verification/evidence

## Cross-story dependencies to track

- `STORY-260712-30ju1k` Windows packaged-app spike
  - Needed before Windows capture/hotkey/file picker work can be accepted as
    Store-safe under the current AppContainer posture.
- `STORY-260712-2ve1c8` Identity and self-service onboarding
  - Blocks real `Create` / `Join` actions, control-token storage and unpaired
    to paired transitions on both platforms.
- `STORY-260712-ld674h` Media ingest and storage
  - Blocks upload-session creation, file validation, idempotent retry and the
    draft-to-upload transition after recording or file pick/drop.
- `STORY-260712-25lysg` Transmission protocol and scheduler
  - Blocks routing options, receipt rendering, coordinator-outage semantics,
    `This Pulsar` / `own Barycenter` / `current approach` delivery and honest
    status mapping in history.
- `STORY-260712-34kbkn` Telegram adapter, history and presence
  - Blocks final parity of sender labels, target labels, receipt wording and
    presence summaries between app and bot surfaces.
- `STORY-260712-1i0doc` Store compliance and acceptance
  - Needed for final screenshot/certification evidence, privacy/reporting copy
    and final confirmation that the builtin cue provenance is acceptable for
    Store submission.

## Completeness check

- Covered:
  - Windows and macOS shell surfaces
  - Local self-test and microphone capture
  - Input/output selection, level meter, hotkey, file input and temp cleanup
  - Routing/history/presence integration
  - Accessibility, DPI and acceptance evidence
- Gap explicitly closed:
  - No builtin cue asset or shared temp-media contract exists in the repo; this
    is now its own task instead of an implicit dependency.
  - Disposable self-test recordings, partial capture files, and finalized
    unsent user drafts now have separate lifecycle states. Only finalized user
    drafts survive an outage/restart; self-test and cancelled media are removed.
  - Escape remains foreground-scoped; hidden recording has an explicit tray or
    menu-bar Cancel action rather than a global bare-Escape hook.
- Deferred to sibling stories, not forgotten:
  - Real onboarding APIs
  - Upload sessions and canonical media pipeline
  - Transmission/receipt/presence backends
  - Store compliance evidence and submission materials

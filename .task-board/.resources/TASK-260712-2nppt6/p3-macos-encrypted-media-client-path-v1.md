# P3 macOS encrypted-media client path v1

Task: `TASK-260712-2nppt6`

Status: production-dark repository implementation; no runtime route, capability advertisement, provider, suite, or container is enabled

## Integration boundary

The macOS surface composes the already accepted key-state, protected-send,
protected-playback, live-PTT, recovery/history-grant, and voluntary report-evidence
contracts. It does not implement cryptography or media processing. A dormant
composition constructs `MacE2EEKeyStateRepository`,
`MacProtectedMediaSendService`, `MacProtectedMediaPlaybackService`, and
`MacE2EELiveSessionFactory` around one repository. It retains an abstract
operating-system-backed ownership lease for its full lifetime and refuses
construction unless that lease covers other processes. No concrete lease or
entry-point wiring is supplied by this task.

The presentation model reports an encrypted path only when every gate is true:
the projection is current, the device is verified, membership and epoch are
current, the reviewed suite and capability are present, key/send/playback/live
components are ready, the services share one repository, runtime wiring is
approved, and generation ownership is attested. A failed gate leaves the
selected protected path visibly blocked. It never changes the selection to
plaintext.

## User-controlled transitions

Unsupported recipients block protected send until the user explicitly excludes
the exact unsupported set. Device verification and lost-device revocation are
separate actions; revocation warns that pending transfer/grant state is revoked
and that the group must rotate before protected send resumes. Device transfer
copies only current access. It does not silently include history. A user-held
recovery action exists only when the server projection advertises the reviewed
mode, and the surface explains that the coordinator cannot recover keys.

History access is a separate confirmed command for one selected object, one
verified recipient device, an explicit epoch interval, mode, and expiry. The UI
also states when history is irrecoverable because no authorized peer or reviewed
user-held capability survives.

Metadata-only reporting and decrypted-evidence export are different commands.
The first does not disclose audio. The second requires a separate confirmation
that a user-selected locally decrypted copy will leave the E2EE boundary for
moderation storage. No secret, decrypted bytes, raw device identifier, raw
object identifier, or raw grant identifier is accepted or rendered by the UI.

## Verification and evidence boundary

Swift model tests cover ownership, capability, component, verification,
membership, unsupported-recipient, revoke, transfer, recovery, history-grant,
report-consent, and redaction failures. Source-contract tests ensure the dormant
composition uses one repository and that neither the composition nor the view
is referenced by `main.swift`. The portable contract is
`protocol/macos-encrypted-media-client-path-v1.json`; the repository evidence
record is `acceptance/phase3/macos-encrypted-media-client-path-v1.json`.

These tests do not prove real Keychain behavior, a production cryptographic
provider, a signed/notarized application, device-to-device interoperability,
audio correctness, physical deletion, traffic or memory secrecy, crash-report
redaction, or a VoiceOver/keyboard walkthrough on a packaged build. Those
claims remain in manual epic `EPIC-260714-th54l3`.

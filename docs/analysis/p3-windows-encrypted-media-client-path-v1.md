# P3 Windows encrypted-media client path v1

Task: `TASK-260712-2q4jbu`

Status: production-dark repository implementation; no runtime route, capability advertisement, provider, suite, or container is enabled.

## Integration boundary

The Windows path composes the already accepted key-state, protected-send,
protected-playback and live-PTT services around one
`WindowsE2EEKeyStateRepository`. The accepted repository keeps generation
reservation behind its process mutex and Win32 share-none cross-process lock.
This task does not implement cryptography or media processing and does not add
the composition to `main.go` or the native window/tray runtime.

The pure presentation model reports a protected clip, track or live session as
encrypted only when the server projection is current, the device is verified,
membership and epoch are current, a reviewed suite and capability are present,
all required components point at the same repository, runtime wiring is
approved, and unsupported recipients have been explicitly excluded. Any failed
gate keeps the selected protected path visibly blocked. Selection is never
rewritten to plaintext.

## User-controlled transitions

Device verification and lost-device revocation are separate commands.
Revocation and user-held recovery require explicit confirmation. Device
transfer copies the current epoch only; history requires a distinct grant for
one selected object, verified device, epoch interval, mode and expiry of at
most 30 days. An explicit warning remains present when history is irrecoverable.

Metadata-only reporting and decrypted-evidence export are separate commands.
The latter requires a consent version and explicit confirmation because a
locally decrypted copy leaves the E2EE boundary for moderation storage. The
English and Russian presentation projection carries accessible names and values
but no stable device/object/grant identifiers, secret material or decrypted
bytes.

## Verification and evidence boundary

Go tests cover ownership, capability, component, verification, membership,
unsupported-recipient, revoke, transfer, recovery, history-grant,
report-consent, redaction, accessibility and one-repository composition. A
source gate keeps `WindowsEncryptedMediaClientPathComposition` out of
`main.go`. The portable contract is
`protocol/windows-encrypted-media-client-path-v1.json`; repository evidence is
`acceptance/phase3/windows-encrypted-media-client-path-v1.json`.

Automation does not prove a signed Store MSIX, native DPAPI/NTFS/ACL behavior,
a production cryptographic provider, physical device interoperability, audible
output, Narrator/keyboard/high-DPI behavior, deletion, traffic/pagefile/memory
secrecy, crash redaction or moderation-storage behavior. Those claims remain in
manual epic `EPIC-260714-th54l3`.

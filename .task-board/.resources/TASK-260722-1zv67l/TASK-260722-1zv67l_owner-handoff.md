# Ivan's one-pass Mac Create → Windows Join

This is the only owner pass requested. It uses two ordinary app screens, requires no Terminal, and does not replace the existing unchecked manual/hardware rows. Automated readiness is green; the real Create, invitation, and Join have deliberately not been performed yet.

Exact installed candidates:

- Mac: `/Applications/Pulsar.app`, version `0.3.0` build `946`, x86_64, source `fb807e1caa40ebb7d206d983e234b626f4457945`. `NodeApp` SHA-256 `a862bfd563ef9956527ad5704e290966b8d8922cea3dbdd54cee2097f53fbabd`; `go-librespot` SHA-256 `a6a6808104129b18e2b660526e4d44c8d1731d89f2e62ea6a2cce30e09c7d61f`; `Info.plist` SHA-256 `885b001d33a76ccf95e554e568594d9ae6037459592c45692dbf5d48ca429308`; review archive SHA-256 `87313d3a64821aebf76b4e8d993041819cd7f9f3df20082d7f95c6383cad6c67`. This is the accepted local `duet-nodeapp`-signed candidate, not a notarized distribution build.
- Windows: installed package `ReluxWorksLLC.PulsarBarycenter_0.1.20.0_x64__q036g2bzd7ngc`, source `76f09a4d8be693d57cd5d47b9b9e5ac06196519c`. Installed `pulsar-win-amd64.exe` SHA-256 `0a77f53f026b77dd6abc3b265f18a8d32744847ca23571e97ddd999cc17a0042`; `go-librespot.exe` SHA-256 `1967b76fc6e8e91763cea10c1cac1bb5f97cdb08a6100bdb27c9a01470cf84ca`; `pulsar-capture.dll` SHA-256 `8c1657d035ab738559c91c4c8468d6a4ba663a80dc96aab8951cc4c2d3b52c2f`. Accepted package archive SHA-256 `f74b5c8d6f8c86443f8c1b64715977be1b0183c39e7fc4dde7567c957b958348` (from the accepted install receipt; the unavailable archive was not rehashed during this handoff). This is a local Developer-signed candidate.

## Screen 1 — Mac: create once and copy one invitation

1. In Pulsar, choose **Create**, enter the Air name you want, and select **Create securely** once.
2. When Pulsar says **Save the one-time recovery file before continuing.**, choose **Save recovery file** and keep that file somewhere safe. Expected result: **Credentials saved** and the new Air name remains visible.
3. Open **Devices**. Expected authorization state: **Primary authorization confirmed**.
4. Choose **Generate invitation** once, then **Copy code** once. Expected state: **One-time invitation is ready**, followed by **Copied. Pulsar will clear this exact clipboard value automatically.**
5. Keep Pulsar open while you move immediately to Windows. Do not generate a replacement invitation unless the first attempt returns an explicit error.

## Screen 2 — Windows: join once and report what is visible

1. Pulsar is prepared on **Join**. If it is not open, launch Pulsar from the Desktop or Start menu and choose **Join**.
2. Paste the single invitation into the invitation field and select **Join securely** once.
3. Expected result: **Identity saved securely**, then the connection state changes from **Not paired** to **Connected**. If it does not, stop and preserve the exact visible error instead of creating another invitation.

Please report only the visible result: the Mac status after copying the invitation, and the exact Windows status or error after **Join securely**. A screenshot is optional; no logs, commands, audio test, hardware test, or broader checklist are requested.

Afterward, keep the recovery file. Let Pulsar auto-clear the invitation from the clipboard; close the invitation display when convenient. Leave both installed candidates in place for focused follow-up. On a failure, route the exact visible symptom without repeating the flow: coordinator/route errors to `TASK-260722-3fsxj5`, Mac launch/signing to `TASK-260722-ckyqnw`, Mac Create/invitation behavior to `TASK-260722-26cbwk`, Windows native navigation/crash behavior to `BUG-260721-27irt6`, and Windows assistive-automation semantics to `BUG-260722-224lo9`.

Manual status at handoff: **not run**. No manual or hardware PASS is claimed.

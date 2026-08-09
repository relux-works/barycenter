# Windows Air onboarding binary installation

Date: 2026-07-27
Host alias: `win`
OS: Windows 10 Pro x64

## Installed artifact

- Build label: `v0.3.0-beta.32-airfix.20260727`
- Path: `C:\Users\admin\AppData\Local\Programs\Pulsar\pulsar-win-amd64.exe`
- Size: `9530368` bytes
- SHA-256: `9D544F59997C6B5EDC6FAFB29A0E92DA58F6644D2066512C5C9575264111FC25`

The local build hash, uploaded artifact hash, and installed-file hash match.

## Recovery and launch

- Previous binary retained as `pulsar-win-amd64.exe.bak-20260727-214532`.
- Previous desktop shortcut retained as `Pulsar.lnk.bak-20260727-214903`.
- `Desktop\Pulsar.lnk` now targets the installed standalone binary directly.
- The existing MSIX package `0.3.32.0` was not removed or modified.
- The console session was disconnected during installation, so the GUI was not left running. The direct desktop shortcut is ready for the next interactive session.

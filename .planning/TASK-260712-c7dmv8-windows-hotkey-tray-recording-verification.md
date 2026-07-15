# TASK-260712-c7dmv8 verification boundary

## Automated engineering evidence

- Portable shortcut tests cover default registration, conflict, reconfiguration,
  stale message rejection, overlapping lock/suspend ownership, retryable
  unregister failure, bounded persistence and selected-key labels.
- Portable recording-controller tests cover asynchronous start, second-toggle
  stop, explicit cancel, lock, suspend, failure projection, quit and bounded
  shutdown drain.
- Source-contract tests pin `RegisterHotKey`/`UnregisterHotKey` to the tray HWND,
  `WM_HOTKEY`, WTS and power routing, explicit tray Cancel, foreground-only Esc,
  production capture wiring and the absence of low-level keyboard hooks.
- The portable Go suite, race detector, vet and Windows amd64 cross-build are
  required before acceptance. Hosted CI must additionally pass the signed MSIX
  build/install/cleanup job on the exact accepted code head.

## Deliberately unclaimed manual evidence

No automated result claims a physical keypress, an actual Windows hotkey
conflict, Narrator/tray behavior, real microphone capture, audible cues,
lock/suspend delivery, signed AppContainer behavior or repeated installed-app
cycles. Those observations remain in `EPIC-260714-th54l3` and do not block the
best-effort engineering acceptance of this task.

# Root review round 1 — Windows AppContainer bridge

Date: 2026-07-12  
Task: `TASK-260712-6kba80`  
Verdict: changes required; the direction is plausible, but the current note is
not implementation-safe yet.

## Blocking findings

1. **The COM ownership story is internally incomplete.**
   `ActivateAudioInterfaceAsync` is invoked on the UI thread and completes on a
   system MTA worker, but the note then says a helper-owned dedicated capture
   thread owns and releases `IAudioClient`/`IAudioCaptureClient`. It never
   specifies how the returned `IAudioClient` is legally transferred to that
   thread. Freeze an exact, documented handoff (for example COM marshaling to a
   helper-owned MTA worker), which thread calls `Initialize` and `GetService`,
   and the release order. The thread that releases the service must also
   release the `IAudioClient`. Cover cancellation before activation completes,
   callback lifetime, revoke, and shutdown without a UI-thread deadlock.
   Relevant Microsoft docs:
   - https://learn.microsoft.com/en-us/windows/win32/api/mmdeviceapi/nf-mmdeviceapi-activateaudiointerfaceasync
   - https://learn.microsoft.com/en-us/windows/win32/api/audioclient/nf-audioclient-iaudioclient-getservice

2. **The helper ABI, build, and loading contract is not frozen.**
   Define the narrow exported ABI sufficiently for the probe developer to avoid
   inventing ownership: calling convention, fixed-width types, opaque-handle
   ownership, UTF-16 buffer rules, PCM buffer ownership, async notification
   delivery, HRESULT/error mapping, and idempotent stop/destroy. Pin x64 and the
   native toolchain. State whether the CRT is statically linked or which
   redistributable files are packaged, and record the applicable Microsoft
   redistribution posture for C++/WinRT/Windows SDK/runtime pieces. Load only
   from an absolute package-relative path or a packaged-library API; do not
   authorize ambient DLL search. Clarify that MSIX signing covers package
   payload integrity and whether a separate DLL Authenticode signature is or is
   not required.

3. **A hidden lifecycle HWND is not an accepted picker owner.**
   `IInitializeWithWindow` requires an owner HWND, but the cited documentation
   does not establish that an invisible window gives correct modality,
   foreground placement, or accessibility. Freeze the normal path as a visible
   Pulsar top-level window on the UI thread. If the picker is invoked while the
   UI is hidden, restore/activate that window before opening the picker. A
   hidden owner may remain only as an explicitly failed-or-proved probe branch,
   not the selected production contract.
   - https://learn.microsoft.com/en-us/windows/apps/develop/ui/display-ui-objects

4. **Several AppContainer statements are stronger than the evidence.**
   The cited `RegisterHotKey`, WTS, and power-message pages document Win32
   behavior, but do not by themselves prove runtime behavior under this exact
   signed `packagedClassicApp` + `appContainer` package. `AppCapability.Create`
   is documented as SUA-only, which the note omits. Separate documented facts
   from probe hypotheses. Require MakeAppx validation, package/WACK validation,
   and real signed Windows 10 and 11 evidence for every import and runtime path,
   with HRESULT/GetLastError captured. Keep failures as fail/blocked and name a
   fallback for permission status if `AppCapability` is unavailable while
   `ActivateAudioInterfaceAsync` consent still works.
   - https://learn.microsoft.com/en-us/uwp/api/windows.security.authorization.appcapabilityaccess.appcapability
   - https://learn.microsoft.com/en-us/windows/win32/api/winuser/nf-winuser-registerhotkey
   - https://learn.microsoft.com/en-us/windows/win32/api/wtsapi32/nf-wtsapi32-wtsregistersessionnotification

5. **Lifecycle stop semantics are not precise enough for implementation.**
   Freeze the nonblocking stop state machine and ordering for quit,
   `WM_QUERYENDSESSION`/`WM_ENDSESSION`, suspend, lock, permission revoke,
   device invalidation, and late callbacks. State which paths finalize a valid
   draft versus discard a partial buffer, that no network operation blocks the
   window procedure, and how resume/unlock rechecks capability/device state.
   The next lifecycle task may implement it, but it must not invent the policy.

6. **The selected capture baseline needs exact probe defaults.**
   Pin shared versus exclusive mode, event versus polling operation, the default
   `AudioDeviceRole`, format negotiation/conversion boundary, and the selected
   device-ID type passed to `ActivateAudioInterfaceAsync`. These can be small,
   but they must be explicit enough that default-input and selected-input proof
   exercise the same intended path.

## Required resubmission shape

- Amend the existing research note rather than creating a competing decision.
- Preserve the useful option matrix, manifest delta, and real-hardware evidence
  list.
- Add the exact thread/ABI/lifecycle contracts above and downgrade unproved
  AppContainer claims to mandatory probe hypotheses.
- Keep source code untouched.
- Reattach byte-identical outcome content, check every completed board item,
  and return the task to `to-review`, never `done`.

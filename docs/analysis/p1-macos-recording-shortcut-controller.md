# P1 macOS recording shortcut and menu lifecycle

`TASK-260712-ut6akw` owns the configurable global-toggle controller, honest
shortcut state, foreground Escape and hidden-window cancel surface. It does not
duplicate microphone capture or upload behavior. `TASK-260712-1s6h6t` binds
these accepted seams to the capture engine and persists the selected preset in
the running application.

## System API decision

Pulsar uses the macOS `RegisterEventHotKey` API from Carbon/HIToolbox with
`kEventHotKeyExclusive`. The shipped SDK documents this as a global virtual-key
registration, provides an explicit `eventHotKeyExistsErr` conflict result and
requires callers to unregister when reconfiguring. The controller maps that
result to a visible conflict state and leaves window/menu actions independent.

The implementation deliberately does not use `NSEvent` global key monitoring
or a Core Graphics event tap. Apple documents that global key events through
`NSEvent` require Accessibility trust, while App Sandbox restricts assistive
accessibility API use. Pulsar neither requests nor silently assumes that
permission:

- [NSEvent global monitor](https://developer.apple.com/documentation/appkit/nsevent/addglobalmonitorforevents%28matching%3Ahandler%3A%29)
- [Protecting user data with App Sandbox](https://developer.apple.com/documentation/security/protecting-user-data-with-app-sandbox)

The allowed presets are modifier-bearing `Space` or `R` chords. Bare Escape is
not representable. The default is `Control-Shift-Space`; alternative presets
remain bounded and persist through a validated versioned store.

## Ownership and lifecycle

The main-actor controller owns at most one opaque registration. Every start,
reconfigure, suspend, resume or stop advances a generation, so callbacks from
an unregistered hook are inert even if delivered late. A Carbon token also
unregisters on destruction as a final cleanup backstop.

Sleep and an inactive login session cancel recording through the shared action
seam and unregister the shortcut. Wake/session activation restores the current
configuration once. Quit cancels and stops. Repeated notifications and repeated
teardown are idempotent.

The SwiftUI shell projects registered, conflict, unavailable, suspended and
inactive states with text and symbol semantics. `Esc` is implemented only with
the focused window's `onExitCommand`; there is no global Escape hook. When a
recording is active while the main window is hidden, the status menu adds an
explicit **Cancel recording** action beside the normal toggle/Stop action.

## Automated evidence and manual boundary

Tests cover successful toggle forwarding, conflicts, unavailable registration,
reconfiguration, late stale callbacks, repeated suspend/resume/stop, sleep,
session lock/unlock, quit, validated persistence, EN/RU projection, foreground
Escape and status-menu Cancel. Source guards reject global `NSEvent` monitoring,
Core Graphics event taps and a representable bare-Escape shortcut.

No physical keyboard, real cross-app conflict, lock screen, sleep/wake,
packaged App Sandbox or hidden-window observation is claimed here. Those remain
manual evidence in `EPIC-260714-th54l3`.

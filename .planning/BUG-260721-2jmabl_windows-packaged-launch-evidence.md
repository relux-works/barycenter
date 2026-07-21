# BUG-260721-2jmabl Windows packaged launch evidence

- Date: 2026-07-21
- Host: `DESKTOP-3PBO632` (`mbpro-win`), Windows 10 Pro `10.0.19045`
- Boundary: autonomous engineering evidence only; no microphone, audible,
  accessibility-reader, Store/EV, or other manual hardware PASS is claimed.

## Reproduction

The installed `0.1.1.0` production-schema package accepted AUMID activations,
created a hidden top-level `Pulsar` HWND, and then removed its AppContainer in
about one second. Repeated user shortcut clicks and an interactive
`IApplicationActivationManager` harness produced the same result. The package
status was `Ok`; TWinUI/AppModel activation completed successfully; no WER or
Application Error event existed. `pulsar.log` stayed at zero bytes because the
GUI build's invalid stderr handle caused `io.MultiWriter` to stop before its
file sink.

After repairing durable logging, staged startup records proved that all 129
native child controls, theme setup and accelerator setup completed. The
monolithic initial `render()` then configured every off-screen product section
before the owner thread entered its Win32 message pump. Win10 first removed the
hidden activation after about one second and, when the top-level HWND was made
visible for diagnosis, removed the still-unresponsive activation later. The
final implementation makes the top-level surface visible immediately, creates
child controls hidden, and uses a bounded Home-only initial render. It also
guards failed tray-window destruction, persists GUI logs despite an invalid
stderr handle, records runtime crashes, and surfaces guarded startup panics.

## Exact installed candidate

- Package: `ReluxWorksLLC.PulsarBarycenter_0.1.11.0_x64__q036g2bzd7ngc`
- Family: `ReluxWorksLLC.PulsarBarycenter_q036g2bzd7ngc`
- AUMID: `ReluxWorksLLC.PulsarBarycenter_q036g2bzd7ngc!Pulsar`
- Package SHA-256:
  `b8374791fa95c4b17eb1cae9195c19e344293263678946a02c958599800aafa2`
- GUI executable SHA-256:
  `839b00a84dd271121b8c4987a33b97b238e3ea9d458e19f39c09b0540265f0bb`
- Package status: `Ok`
- Signature: valid local `Developer` signature
- Desktop shortcut target: `C:\Windows\explorer.exe`
- Desktop shortcut arguments:
  `shell:AppsFolder\ReluxWorksLLC.PulsarBarycenter_q036g2bzd7ngc!Pulsar`

## Autonomous launch evidence

Two interactive, hidden-console `IApplicationActivationManager` runs observed
the exact installed package. The final extended soak ran for 188.523 seconds:

- process ID `10268` remained alive in all 720 samples;
- the top-level `Pulsar` HWND was visible in 719/720 samples (the first sample
  preceded HWND creation) and was still visible in the final sample;
- `aliveAfterObservation=true` and `exitCode=null` at observer completion;
- an independent UI Automation capture during the soak reported
  `processRunning=true`,
  `responding=true`, `mainWindowVisible=true`, `mainWindowHung=false`, DPI
  `192`, bounds `1093x815`, and 22 current-section controls;
- soak screenshot SHA-256:
  `49e2836428c0b136cece5f03a54e7da8fc3c608ec0c2fcbcb491c2936b866887`;
- no terminal window was used by the packaged GUI or either hidden observer.

The scheduled observer owns a diagnostic job and Windows tears down its
activation when that job finishes. That harness-only teardown produced no
`unpaired shell stopped` record and no crash output; it is not part of the
ordinary Desktop/Start shortcut path. The shorter run independently recorded
120/120 live samples over 31.199 seconds and the same responsive visible
window before the extended soak was added.

The ordinary shortcut path was then exercised separately through the Windows
Desktop Shell default verb, not `IApplicationActivationManager`. Its launcher
task completed successfully while process ID `6152` remained alive outside
the task. A subsequent independent UI Automation task also completed without
terminating that process and reported `Pulsar` visible, responding and not
hung at 192 DPI with 22 current-section controls. Shortcut-launch screenshot
SHA-256 is
`e398c8c2ea9d38137effc8d298fb47fb5a08be888850dee0f4e59a902a6fe9c1`.

Local verification passed `go test ./...`, Windows-targeted `go vet ./...`, and
the amd64 GUI-subsystem cross-build. Claude Opus 4.8 reviewer run
`RUN-260721-72524f` independently repeated those gates and accepted exact
commit `62302e0`. The consolidated human task remains the only place for later
microphone/audio and subjective real-app acceptance.

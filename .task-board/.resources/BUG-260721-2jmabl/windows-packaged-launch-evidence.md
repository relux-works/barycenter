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

An interactive, hidden-console `IApplicationActivationManager` run observed
the exact installed package for 31.199 seconds:

- process ID `4188` remained alive in all 120 samples;
- final top-level HWND `0x1A028C` was `visible=True` with title `Pulsar`;
- `aliveAfterObservation=true` and the process remained alive and responding
  after the observer task returned;
- an independent UI Automation capture reported `processRunning=true`,
  `responding=true`, `mainWindowVisible=true`, `mainWindowHung=false`, DPI
  `192`, bounds `1093x815`, and 22 current-section controls;
- final screenshot SHA-256:
  `1d5b9ef259411d9264f7dd35060a8d1044db878f269fb397e3f8729d09cacc84`;
- no terminal window was used by the packaged GUI or either hidden observer.

Local verification passed `go test ./...`, Windows-targeted `go vet ./...`, and
the amd64 GUI-subsystem cross-build. The consolidated human task remains the
only place for later microphone/audio and subjective real-app acceptance.

# TASK-260722-ckyqnw local macOS candidate evidence

## Outcome

- Installed candidate: `/Applications/Pulsar.app`
- Accepted Pulsar source: `fb807e1caa40ebb7d206d983e234b626f4457945`
- relux-works go-librespot source: `8bab3269485e8512021261f5efa69890d762e79f`
- Host/target: macOS 15.7.4 (24G517), x86_64
- Bundle: `works.relux.pulsar`, `CFBundleShortVersionString=0.3.0`, `CFBundleVersion=946`
- Swift dependencies: Sparkle 2.9.4 (`b6496a74a087257ef5e6da1c5b29a447a60f5bd7`), Yams 5.4.0 (`3d6871d5b4a5cd519adf233fbb576e0a2af71c17`)
- Status boundary: stable locally signed, **not notarized**, not a Developer ID/App Store distribution claim.

The candidate was built in a detached worktree at the accepted commit. The current branch's later live-rollout commit changes no app, asset, packaging, or test source relative to that accepted commit.

## Self-contained bundle

The installed app contains and validates:

- `Contents/MacOS/NodeApp` (x86_64 release executable)
- `Contents/MacOS/go-librespot` (x86_64, CGO disabled, relux-works fork)
- `Contents/Frameworks/Sparkle.framework` (Sparkle 2.9.4, x86_64 + arm64 framework)
- `Contents/Resources/Pulsar.icns`
- `Contents/Resources/Audio/pulsar-recording-cue.wav`
- `Contents/Resources/en.lproj/InfoPlist.strings`
- `Contents/Resources/ru.lproj/InfoPlist.strings`
- required `Info.plist` values: microphone, Apple Events, local-network usage strings; `_spotify-connect._tcp`; Sparkle feed/public key/automatic-check settings.

The fork binary's Go build metadata records `vcs.revision=8bab3269485e8512021261f5efa69890d762e79f`, `vcs.modified=false`, `CGO_ENABLED=0`, `GOOS=darwin`, and `GOARCH=amd64`. Its binary contains `PULSAR_ZEROCONF_HOST`, while the exact source uses `zeroconf.RegisterProxy` for that environment contract and retains the ordinary registrar when unset.

## Signing

- Identity common name: `duet-nodeapp`
- Certificate SHA-1: `40DF8747F4232A938A57313718A71748A553388D`
- Certificate SHA-256: `99115BC71F2FB9D02824D608F5ED7BE2332BC7F239143293D0320B008C875125`
- App CDHash: `020f0a58bdfebb8371fb07bc070787b7615a9450`
- go-librespot CDHash: `0a552cb95e31924486b317f02c4e6ce25013a861`
- Sparkle CDHash: `df1c6828721727ce2420c7f9e9533e293c5d7928`
- Designated requirement: `identifier "works.relux.pulsar" and certificate leaf = H"40df8747f4232a938a57313718a71748a553388d"`

Both staged and installed bundles passed `codesign --verify --deep --strict --verbose=4` with exit 0. The installed bundle also passed an explicit `codesign --verify --strict -R=...` evaluation of the requirement above with exit 0. Staged and installed trees compare byte-identically (`diff -qr`, exit 0).

`spctl --assess --type execute --verbose=4` returned exit **3** (`rejected`). This is expected and is recorded as a failing gate: the identity is local/self-signed and the candidate was not submitted to Apple notarization.

## Exact SHA-256 values

| Component | SHA-256 |
|---|---|
| Raw Swift release `NodeApp` before bundle signing | `661dd14dabccaadcd2f50ce35214860507a9e431e6ce113d7ba1e477f357c6db` |
| Raw reproducible go-librespot before signing | `b658820ee3ef4622ca655fef56a6aaf60d6709a5965fe6feb7341f880ab4f46d` |
| Installed signed `NodeApp` | `a862bfd563ef9956527ad5704e290966b8d8922cea3dbdd54cee2097f53fbabd` |
| Installed signed `go-librespot` | `a6a6808104129b18e2b660526e4d44c8d1731d89f2e62ea6a2cce30e09c7d61f` |
| Installed Sparkle main binary | `4c9e7751d1e0ff807bcee71e660c13a1fe2b8e46bbf6c51ecb38c1cfad129b70` |
| Installed `Info.plist` | `885b001d33a76ccf95e554e568594d9ae6037459592c45692dbf5d48ca429308` |
| Installed `Pulsar.icns` | `6cf0c1e01a9a93cceb89638b4c186b361d77abbd716589e311f17e0cc30f64cf` |
| Installed English localization | `444e6e7b8d35fff2113e49b26bffde93b5dcb53a9734905bf90618af79548907` |
| Installed Russian localization | `8daa08daa84f0da2ccab176876d433109c5abcf79c0a656de2c105f2e50a3e4a` |
| Installed recording cue | `479b1a9d605ac12454e3449e129991b7ce8599251506ca54a93be0b6144730fd` |
| Review archive `Pulsar-0.3.0-local-fb807e1-x86_64.app.zip` | `87313d3a64821aebf76b4e8d993041819cd7f9f3df20082d7f95c6383cad6c67` |

The ZIP passed `unzip -t` with exit 0.

## Launch / idle / relaunch smoke

No `works.relux.pulsar` credential item and no default `~/duet/node.yml` existed before launch. No Create, Join, account, recovery, or invite action was invoked.

1. `open /Applications/Pulsar.app` exited 0.
2. First process PID 57985 had PPID 1, state `S`, and exact command `/Applications/Pulsar.app/Contents/MacOS/NodeApp`.
3. `NSRunningApplication` reported bundle `works.relux.pulsar`, name `Pulsar`, `isFinishedLaunching=true`, `isHidden=false`, `isTerminated=false`, and regular activation policy.
4. WindowServer reported one on-screen layer-0 window at 1120x812. A one-second `sample` exited 0 after more than two minutes of process life; the main thread was normally blocked in the AppKit event loop rather than stuck in application work.
5. `NSRunningApplication.terminate()` returned true. The bounded exit wait exited 0.
6. A second ordinary `open /Applications/Pulsar.app` exited 0 and produced new PID 58876 with PPID 1.
7. After a 30-second relaunch idle, PID 58876 remained state `S`; WindowServer again reported one visible 1120x812 regular window and `isFinishedLaunching=true`.
8. The accepted-source focused test `MacFirstRunLifecycleSourceTests.freshLaunchUsesMainShell` passed (1 test, exit 0). It pins the no-token GUI path to `mainWindow.show(section: .home)` and requires the Home-shell Create and Join routes.

No `NodeApp` or `Pulsar` diagnostic crash report was created from task start through handoff (standalone absence assertion, exit 0).

Direct `screencapture` returned exit 1 because this host has no Screen Recording grant; direct AX inspection reported `AXIsProcessTrusted=false`. A native XCUITest runner was then built with Xcode's supported UI-testing entitlements, but the host test service timed out enabling automation mode before executing any test action; `xcodebuild test` returned exit **65**. This expected host-permission failure is not reported as a passing UI gate. The live visible-window evidence plus the deterministic focused first-run test are the autonomous Create/Join surface evidence available on this host.

The temporary XCUITest scaffold, derived data, result bundle, ZIP, and board attachment were removed after independent confirmation that this was an infrastructure-only failure. The red exit-65 boundary remains recorded here and in `LOGBOOK.md`; no XCUITest action or assertion is claimed.

The headless host has no default audio input, so unified logs contain CoreAudio no-device diagnostics. The process stayed responsive and relaunched normally; subjective audio, microphone, and hardware acceptance are outside this story.

## Validation command results

| Gate | Exit | Result |
|---|---:|---|
| `go test ./...` (fork) | 0 | pass |
| `go vet ./...` (fork) | 0 | pass |
| repository-wide fork gofmt cleanliness | 1 | fail: pre-existing one-space alignment in `internal/puregotest/decoders_test.go` |
| gofmt cleanliness of `zeroconf/backend_builtin.go` | 0 | pass |
| reproducible fork build (`CGO_ENABLED=0`, darwin/amd64) | 0 | pass |
| fork binary architecture/VCS/contract validation | 0 | pass |
| first `swift test` using selected CommandLineTools | 1 | fail: CommandLineTools lacks module `Testing` |
| documented full-Xcode `xcrun swift test` | 0 | pass: 381 tests in 63 suites |
| focused first-run Create/Join source test | 0 | pass: 1 test |
| full-Xcode release build | 0 | pass |
| `scripts/build-app.sh` with explicit components and identity | 0 | pass |
| staged bundle structural/privacy/runtime-link assertions | 0 | pass |
| staged strict nested codesign | 0 | pass |
| staged explicit designated requirement | 0 | pass |
| installed strict nested codesign | 0 | pass |
| installed explicit designated requirement | 0 | pass |
| staged vs installed tree comparison | 0 | pass |
| ZIP integrity | 0 | pass |
| Gatekeeper/notarization assessment | 3 | expected fail: local build is not notarized |
| ordinary launch, idle, normal termination, relaunch gates | 0 | pass |
| no task-window `NodeApp`/`Pulsar` diagnostic crash report | 0 | pass |
| native live XCUITest | 65 | fail before test actions: host could not enable automation mode |
| final installed NodeApp/go-librespot/Sparkle SHA-256 assertions | 0 | pass |
| final installed go-librespot contract and Go VCS metadata probes | 0 | pass |
| `git diff --check` | 0 | pass |
| `task-board validate` | 0 | command succeeded while reporting 79 pre-existing board-wide issues unrelated to this task |

One source-integrity probe initially exited 1 because the requested fork output path `daemon-fork` is itself a tracked historical artifact and the fresh build replaces those bytes. The corrected integrity gate verified that `daemon-fork` was the sole diff and all Go sources plus `go.mod`/`go.sum` remained byte-identical to commit `8bab326...`; it exited 0. The embedded Go VCS metadata independently records `vcs.modified=false` at build time.

Two late installed-binary probes initially exited 1 because the verification command mistakenly targeted `Contents/Resources/go-librespot`; the self-contained bundle layout documented above places that executable at `Contents/MacOS/go-librespot`. The hash and `PULSAR_ZEROCONF_HOST` probes were rerun against the packaged path and both exited 0.

## Reviewer notes

- The installed candidate and attached review ZIP are local-test artifacts only.
- No notarization, stapling, Developer ID distribution, or Store acceptance is claimed.
- Pulsar remains running from the successful relaunch at handoff unless the reviewer terminates it.

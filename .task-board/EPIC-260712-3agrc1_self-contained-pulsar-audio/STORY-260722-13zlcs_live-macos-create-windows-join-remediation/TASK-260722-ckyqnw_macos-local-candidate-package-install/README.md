# TASK-260722-ckyqnw: macos-local-candidate-package-install

## Description
Produce and install a self-contained local macOS Pulsar candidate from the accepted onboarding source.

## Scope
Build the required relux-works go-librespot fork with the expected PULSAR_ZEROCONF_HOST contract, build the Swift release executable and Sparkle framework, create or reuse a stable local code-signing identity, assemble Pulsar.app with privacy declarations and bundled assets, verify codesign/designated requirement, install it in /Applications, launch through Finder/open as an ordinary GUI application and run an autonomous launch/idle/relaunch smoke. Do not claim notarization or Store distribution.

## Acceptance Criteria
The exact accepted source produces /Applications/Pulsar.app with NodeApp, Sparkle, bundled compatible go-librespot, icon/localizations and required privacy strings. codesign verification passes with a stable local identity, the app launches with no terminal, presents the first-run Create/Join UI, remains responsive through the bounded smoke, and exact component hashes plus non-notarized local-test status are recorded.

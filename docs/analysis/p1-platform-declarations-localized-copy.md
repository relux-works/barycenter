# P1 platform declarations and localized copy

Engineering owner and approver: Ivan Oparin. This handoff freezes the minimal
packaged declarations and the EN/RU vocabulary used by the P1 desktop clients.

## Reviewed declarations

- Windows declares only `internetClient`, `internetClientServer`,
  `privateNetworkClientServer`, and the `microphone` device capability. The LAN
  capabilities support the optional Spotify discovery path. There is no
  `runFullTrust`, library capability, or broad filesystem access.
- macOS declares microphone usage, optional Spotify local-network discovery,
  and optional Airfoil Apple Events usage. No sandbox or Accessibility
  entitlement is added because the current platform spikes did not establish
  one as necessary for shipped P1 behavior.
- Microphone permission is requested only after Record. Automated capture and
  local-self-test suites prove that denial creates no draft and leaves the
  reviewed builtin cue and brokered file-review path usable.

The canonical vocabulary is
[`assets/localization/platform-copy.json`](../../assets/localization/platform-copy.json).
Windows shell tests and macOS shell tests bind Create, Join, Try locally,
routing, history, report, and optional integration copy to that contract.
MSIX package descriptions and macOS privacy prompts use the same source text.

## Packaging and downstream consumers

The release workflow stages EN/RU `.resw` files and compiles `resources.pri`
before `makeappx`. The macOS bundle build stages localized
`InfoPlist.strings`. Product settings expose Spotify and Telegram as optional;
legacy companion onboarding no longer claims either service is required and
does not show Spotify help automatically after pairing.

The package listing, certification packet, support pages, and P1 acceptance
review must use this vocabulary. Any new capability or privacy-sensitive prompt
requires a source change, automated contract update, and delta review.

## Manual evidence boundary

Static XML/plist parsing, portable unit tests, packaging commands, and CI builds
are engineering evidence. Actual WACK UI results, installed signed-package
permission prompts, denial screenshots, and real Windows/macOS hardware checks
remain in the manual-test epic `EPIC-260714-th54l3`; this task does not claim
those observations.

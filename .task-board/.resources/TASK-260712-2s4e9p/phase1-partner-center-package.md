# Phase 1 Partner Center package

This directory is the versioned engineering input for the corrective Pulsar
Store submission. It directly answers the recorded July 2026 `10.3.1 Product
is Testable` and `10.1.1.3 Inaccurate Representation` findings without
pretending that Partner Center, WACK or a real Windows app was operated here.

## Frozen engineering inputs

`partner-center-package.json` binds Product ID `9P26FDCWV1GC`, package identity
`ReluxWorksLLC.PulsarBarycenter`, category, price, coordinator origin, approved
public links and the exact component files. The EN/RU descriptions and feature
lists lead with the self-contained clip flow. They disclose coordinator
processing, 13+ positioning and the fact that Spotify and Telegram are
optional.

`certification-notes.json` is below the 2,000-character project gate and gives
the reviewer the six-step A1 path: launch, Try locally, Create / Create a Barycenter,
record or use the denial-safe file fallback, target This Pulsar, then inspect
History and moderation actions. It explicitly says that Spotify, Telegram and
demo credentials are not required. Exact version, Git commit and MSIX SHA-256
tokens remain unfilled until a candidate is frozen.

`iarc-answer-profile.json` is a source-linked truth ledger, not a fabricated
copy of a storefront-only questionnaire. It requires accurate disclosure of
internet access, private user audio, user-to-user communication, reactive
report/block/remove controls, no pre-moderation or parental controls, and the
absence of product-authored mature content, purchases, gambling, ads and
location sharing. The service minimum age of 13 is not treated as an IARC
rating result.

`screenshots.json` reserves six distinct PNG slots for each locale: main
window, Try locally, active recording, routing, played history, and
settings/moderation. Each image must be at least 1366 x 768, at most 50 MB,
have a locale-specific caption of at most 200 characters and match its recorded
SHA-256. A login/onboarding-only image cannot satisfy any slot.

## Manual/external completion

On the exact candidate, the authorized operators must:

1. Fill version, 40-character Git commit and MSIX SHA-256 in
   `certification-notes.json`, replace every `{{...}}` token and set its state
   to `frozen`.
2. Capture the twelve sanitized real Windows screenshots under the paths in
   `screenshots.json`, fill hashes, mark each `captured`, and mark the manifest
   `captured-reviewed`. This evidence belongs to manual task
   `TASK-260712-e5mfqj` in `EPIC-260714-th54l3`.
3. Run `scripts/acceptance/run_wack.ps1` from an elevated interactive Windows
   session against that same MSIX, review every result, version the sanitized
   manifest/report and set the WACK gate to `completed-reviewed`.
4. In Partner Center, map the IARC truth ledger to the exact questions shown,
   export or capture the answers, fill the generated rating ID, hash, time and
   Ivan Oparin review, and set both IARC states to `generated-reviewed`.
5. Run the live policy/source delta gate, attach the raw sanitized July 2026
   finding artifact, then let Ivan Oparin change the package/submission states
   to `submission-ready`/`proceed` only if every gate is current.

The ready command must then pass:

```sh
cd coordinator
GOTOOLCHAIN=go1.25.12 go run ./cmd/store-listing-check --require-ready
```

Partner Center mutation is outside this engineering task and requires the
approved final-submit authority.

## Current official limits

The source registry was rechecked on 2026-07-15. Microsoft currently requires
a description and at least one screenshot; desktop screenshots are PNG, at
least 1366 x 768 and at most 50 MB, with up to ten per locale and captions up
to 200 characters. Descriptions allow 10,000 characters, up to 20 feature
entries of 200 characters, and up to seven keywords of 40 characters/21 words
total. Microsoft describes IARC ratings as content suitability rather than
target-audience age and requires accurate questionnaire answers. See the exact
official URLs in `partner-center-package.json`.

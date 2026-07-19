# Engineering-only Store package delta review — VERDICT: APPROVE (engineering scope only)

- Task: TASK-260712-2s4e9p — Prepare the corrective Store, IARC and screenshot package
- Reviewer: independent engineering-only delta review per owner-approved boundary
- Reviewed revision: exact `origin/main` `e3bf9851430bdb1bcccdfd01b4a352e0ad4463d8` (HEAD == origin/main verified; working tree contained only board tracking files)
- Review date: 2026-07-19
- Boundary honored: no Partner Center login or mutation, no submission, no screenshot/WACK/rating/build-identity inference. Historical checklist items 3, 4, 5, 7 remain intentionally unchecked and routed (manual: TASK-260712-e5mfqj; owner/external: TASK-260715-24ube9).

## Verdict

**APPROVE** for engineering scope only. The repository-verifiable portion of the corrective Store/IARC/screenshot package is complete, internally consistent, source-linked, matches current official Microsoft guidance as of 2026-07-19, matches the implemented build's behavior and strings, and fails closed on every piece of absent manual/external evidence. No Store-readiness, hardware, WACK, rating, submission or release claim is made or implied. Both routed tasks (TASK-260712-e5mfqj, TASK-260715-24ube9) remain open.

## 1. Package inspection (docs/store/phase1, checker, tests)

- `docs/store/phase1/partner-center-package.json` binds Product ID `9P26FDCWV1GC`, package identity `ReluxWorksLLC.PulsarBarycenter`, category Music, Free, minimum age 13, coordinator origin `https://barycenter.relux.works`, the approved-links file, both listings, screenshot manifest, IARC profile, certification notes, WACK gate (`manual-required`) and submission gate (`hold`, authority Ivan Oparin), plus a 7-entry official source registry retrieved 2026-07-15.
- `coordinator/internal/storelisting/storelisting.go` enforces frozen product identity, canonical-HTTPS links, EN+RU-only listings, description ≤10,000 runes, short description ≤270, what's-new ≤1,500, features 1..20 × ≤200 with no self-bullets, keywords 1..7 × ≤40 and ≤21 words, approved-link equality per locale, mandatory optional-Spotify/Telegram wording, mandatory not-E2EE wording (EN and RU), rejection of retired "Spotify Premium" prerequisite copy, ≥3 limitations, existing claim-evidence files, path-traversal-safe repo paths, six distinct scene slots per locale with caption ≤200 and PNG/dimension/digest validation for any present file, IARC truth-ledger completeness with no invented questionnaire results, certification-notes identity/content gates with `{{...}}` freeze tokens, and WACK/submission state machines that refuse partial evidence.
- `coordinator/internal/storelisting/storelisting_test.go` covers: checked-in package passes engineering shape and fails ready-gate specifically on absent screenshots; mutation cases for description/feature/keyword limits, non-approved link, missing optional integration, evidence path traversal; duplicate scene and screenshot path traversal; real-PNG dimension/digest validation including undersized rejection; certification notes corrective-statement and exact-build-freeze rejection.

## 2. Current official Microsoft guidance (primary sources, checked 2026-07-19)

- Store Policies v7.19 (learn.microsoft.com/en-us/windows/apps/publish/store-policies, effective 2025-10-14, still current): 10.3.1 requires a working demo account via Notes for certification only "if your product requires login credentials" — the package's answer (accountless six-step path, no demo credentials) is the correct direct response; 10.3.2 server functionality is answered with the coordinator origin and availability owner; 10.1.1 requires metadata to accurately describe functions/features/limitations; 10.1.3 search terms ≤7, relevant, no other product titles — keywords comply and deliberately exclude Spotify/Telegram; 11.11 requires accurate IARC answers; 11.12 UGC requires published content guidelines, in-product reporting, and removal capability — all present.
- Add/edit Store listing info (msix): description required ≤10,000 plain-text chars; what's new ≤1,500; features ≤20 × ≤200, displayed bulleted (no own bullets); short description recommended, best ≤270 visible. Matches checker exactly.
- Screenshots and images (msix, page updated 2026-07-14): PNG only, ≤50 MB, desktop ≥1366×768, up to 10 desktop screenshots, captions ≤200 chars, one screenshot minimum per submission. Matches the frozen manifest rules.
- Add additional information (msix): keywords ≤7 × ≤40 chars, ≤21 words total. Matches checker.
- Age ratings (msix): IARC questionnaire is completed inside Partner Center during submission; answers must be accurate; ratings are about content suitability, "rather than the age of the target audience" — exactly the package's `audience_note` treatment of the 13+ service minimum. Deferring generated rating/ID to the Partner Center session (owner task) is the correct non-fabricating design.

## 3. Live approved public surfaces (read-only checks)

All eight approved URLs from `docs/compliance/store-public-links.json` are live: each returns a single same-host trailing-slash 308 redirect to a 200 final (`/legal/privacy/`, `/legal/terms/`, `/legal/content-guidelines/`, `/legal/support/` and their `/ru/` variants). Content sniffs: EN privacy contains coordinator processing, end-to-end (absence) disclosure, microphone, 13, Spotify, Telegram, delete, report; RU privacy states «Медиа Phase 1 в Pulsar не защищены сквозным шифрованием» — semantically equivalent to the listing's «сквозного шифрования нет»; terms contains 13/termination; content-guidelines contains report/prohibit/remove (satisfying Store Policy 11.12); support exposes contact/mail. Nothing was modified.

## 4. Copy truth vs the implemented build

Verified in `pulsar-win` sources at the reviewed revision: "Try locally"/«Попробовать локально», "Create a Barycenter"/«Создать Барицентр» (windows_shell.go:1859,1866,1896,1903), routes "This Pulsar"/"My Barycenter"/"Current air" (windows_shell.go:1129), delivery modes "Overlay"/"Interrupt"/"After current" (windows_shell.go:1138), "Do Not Disturb"/«Не беспокоить» (windows_shell.go:1863,1900), Report/«Пожаловаться» (windows_shell.go:1874,1911), "Block sender" (main_window_windows.go:905), "Delete media permanently" (main_window_windows.go:1919), sender-blocked feedback in both locales (windows_shell.go:1248-1249,1264-1265), and availability-gated actions (`CanReport` etc.) matching "when the action is available". MSIX manifest (`pulsar-win/msix/AppxManifest.xml.in`) declares internetClient, internetClientServer, privateNetworkClientServer and microphone DeviceCapability only — no location capability (IARC fact holds) — and documents that microphone is requested only after the user presses Record, matching the certification notes' expected-prompt step and denial-safe file fallback. Certification notes contain the exact six-step A1 path, Product ID, server + availability owner, the explicit statement that no Spotify account, Telegram account or demo credentials are required, and direct 10.3.1/10.1.1.3 responses within the 2,000-character project gate.

## 5. Checker and deterministic tests (executed at the reviewed revision)

- `GOTOOLCHAIN=go1.25.12 go run ./cmd/store-listing-check` → exit 0: "Partner Center package engineering shape is valid; manual screenshots, WACK, IARC and exact-build owner proceed remain required".
- `GOTOOLCHAIN=go1.25.12 go run ./cmd/store-listing-check --require-ready` → exit 1: "screenshot missing: docs/store/phase1/screenshots/en-US/01-main-window.png" — fails closed specifically because manual/external evidence is absent, as required.
- `GOTOOLCHAIN=go1.25.12 go test -count=1 ./internal/storelisting/...` → ok (fresh run, not cached).
- `scripts/acceptance/run_wack.ps1` exists at the path referenced by the WACK gate.

## 6. Delta review since package merge ee0cf03 (claim drift)

`git log ee0cf03..e3bf985` over `docs/store/phase1`, `coordinator/internal/storelisting`, `coordinator/cmd/store-listing-check`, `docs/compliance/store-public-links.json`, `pulsar-win/msix/AppxManifest.xml.in` and `pulsar-win/msix/Strings` is empty — the reviewed package, checker, capabilities and localized MSIX strings are byte-identical to the merged package. Post-merge feature work (stream player/cache/track model, air client, soundboard client, automation admin, e2ee contract audit) does not drift the Phase 1 claims because: `DUET_LIVE_PTT` is env-only and defaults off (coordinator/internal/config/config.go:73,165-167); soundboard cues require explicit owner enablement (windows_shell.go:1197 "The built-in cue appears after an owner enables soundboard"); e2ee work is audit/contract-only with no shipped crypto; and `docs/compliance/phase3-disclosure-delta.md` (2026-07-17) explicitly keeps the checked-in Phase 1 store text as the only publication source, prohibits availability/E2EE wording before external gates, and holds the conservative IARC baseline (private user-audio UGC, reactive report/block/delete, no answer reductions). `docs/store/phase3/disclosure-draft-v1.json` remains a non-submittable conditional draft.

## Non-blocking observations (for the manual/owner tasks)

1. The approved links resolve through one same-host 308 (trailing slash). Partner Center reviewers following the links land on 200 content; if a future policy check demands redirect-free URLs, adding the trailing slash in the approved-links file would be a cosmetic follow-up — do not change it unilaterally, it is the approved artifact.
2. The manifest's 50 MB cap is implemented as 50 MiB (52,428,800 bytes), the binary reading of Microsoft's "50 MB". Real captures will be orders of magnitude below either interpretation; no action needed.

## Routing

- TASK-260712-2s4e9p → `done` (engineering scope only, owner-approved split of 2026-07-19).
- Real localized screenshots + WACK evidence → open manual task TASK-260712-e5mfqj.
- Partner Center questions/export, generated IARC rating + ID, candidate version/commit/MSIX hash, sanitized raw findings, owner proceed/hold → open owner task TASK-260715-24ube9.

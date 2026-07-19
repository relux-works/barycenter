# Independent engineering-only Store package delta review

Review exact `origin/main` revision
`e3bf9851430bdb1bcccdfd01b4a352e0ad4463d8` for the repository-verifiable
portion of `TASK-260712-2s4e9p`.

## Owner-approved boundary

Ivan Oparin previously moved every real-app/real-hardware observation into the
manual epic and approved best-effort coding, deterministic tests and CI. Apply
that boundary here:

- Real localized screenshots and WACK evidence belong only to manual task
  `TASK-260712-e5mfqj`.
- Exact Partner Center questions/export, generated rating and rating ID,
  candidate version/commit/MSIX hash, sanitized raw findings and owner
  proceed/hold belong only to `TASK-260715-24ube9`.
- Do not log in to or mutate Partner Center. Do not submit or publish anything.
- Do not infer screenshots, WACK, rating, build identity or portal evidence.
- Historical checklist items 3, 4, 5 and 7 on the original task remain
  intentionally unchecked and are not blockers to engineering-only acceptance.

## Required review

1. Inspect `docs/store/phase1`, `coordinator/cmd/store-listing-check`, the
   relevant acceptance tests and current product behavior/limitations.
2. Check current official Microsoft Store listing, screenshot, certification
   and age-rating guidance using primary Microsoft sources as of 2026-07-19.
3. Verify the live approved `barycenter.live` terms, content-guidelines,
   privacy/support surfaces without changing them.
4. Verify EN/RU copy, feature/keyword limits, optional Spotify/Telegram
   wording, private UGC/communication IARC truth profile, Product ID
   `9P26FDCWV1GC` responses for findings 10.3.1 and 10.1.1.3, and six distinct
   screenshot slots per locale.
5. Run the default checker and relevant deterministic tests. Confirm
   `--require-ready` fails closed specifically because manual/external evidence
   is absent.
6. Review all deltas since the package merge `ee0cf03` for claim drift.

Publish a source-linked APPROVE or CHANGES REQUESTED verdict. On APPROVE, the
original task may be accepted for engineering scope only while both routed
manual/external tasks remain open. Make no Store-readiness, hardware, WACK,
rating, submission or release claim.

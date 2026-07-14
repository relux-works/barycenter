# Microsoft Store policy baseline — 2026-07-12

> Historical planning snapshot. The current verified matrix is
> `docs/analysis/store-policy-baseline-2026-07-14.md`; preserve this file as the
> pre-task baseline for delta review.

Planning snapshot for `STORY-260712-1i0doc`. This is not a substitute for the
mandatory re-check immediately before submission.

## Official sources checked

- Microsoft Store Policies, document version 7.19, published 2025-09-10 and
  effective 2025-10-14:
  <https://learn.microsoft.com/en-us/windows/apps/publish/store-policies>
- App Quality, especially "Metadata is key":
  <https://learn.microsoft.com/en-us/windows/apps/publish/store-app-quality>
- MSIX screenshots and images:
  <https://learn.microsoft.com/en-us/windows/apps/publish/publish-your-app/msix/screenshots-and-images>
- Microsoft Store submission FAQ:
  <https://learn.microsoft.com/en-us/windows/apps/publish/faq/submit-your-app>

## Certification-report mapping

Product `9P26FDCWV1GC` failed on 2026-07-10 for:

- `10.3.1 Product is Testable`: the reviewer believed primary functionality
  required an account and no credentials were supplied. Phase 1 must instead
  make the reviewer path genuinely accountless: Try locally, create a new
  Barycenter, record, target This Pulsar, play, and inspect the receipt. Notes
  must state that Spotify, Telegram, and demo credentials are not required.
- `10.1.1.3 Inaccurate Representation`: submitted images showed only splash or
  login. New locale-specific screenshots must be captured from the submitted
  Windows build and show distinct implemented primary features: main window,
  local test, recording, routing, played history, and settings/moderation.

## Current requirements that affect implementation

- `10.3.2`: the coordinator used by certification must remain functional.
- `10.5`: a Win32/Desktop Bridge product handling personal information needs a
  current privacy-policy URL and must describe collection, use, storage,
  security, disclosure, user controls, and access.
- `10.6`: declared capabilities must be legitimately used; microphone access
  therefore stays tied to explicit Record, with no broad capability added for
  convenience.
- `10.7`: each declared language needs a reasonably equivalent localized
  experience and localized listing.
- `11.11`: the Partner Center IARC questionnaire must reflect the shipped UGC
  and communication behavior.
- `11.12`: UGC requires published terms or content guidelines, an in-product
  reporting mechanism or proactive detection, and an operational ability to
  remove or disable content when Microsoft requests it.
- Current screenshot guidance requires PNG, desktop dimensions of at least
  1366 x 768, no more than 50 MB, and a separately populated listing for each
  language. Microsoft suggests several screenshots showing actual, distinct
  features; one splash/login image is not adequate evidence for this product.

## Submission gate

`TASK-260712-g9ycx5` must re-open the official sources on the submission date,
record the then-current versions and dimensions, and turn any delta from this
snapshot into a board task before Partner Center is changed.

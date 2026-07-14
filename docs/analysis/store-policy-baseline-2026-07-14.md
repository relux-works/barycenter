# Microsoft Store policy baseline — 2026-07-14

This is the dated requirements matrix for `TASK-260712-g9ycx5`. It records the
official rules retrieved on 2026-07-14, separates requirements from guidance,
and assigns every relevant item to engineering or evidence work. The
machine-readable companion is
`docs/compliance/store-policy-baseline-2026-07-14.json`.

This snapshot is not a certification verdict and is not valid indefinitely.
The pre-submit delta gate at the end must run immediately before Ivan Oparin
authorizes a Partner Center mutation.

## Version and source registry

The current Microsoft Store Policies page reports document version **7.19**,
published **2025-09-10** and effective **2025-10-14**. The official change
history also lists 7.19 as its newest entry. Both pages were retrieved on
2026-07-14.

All normative claims in this matrix come from these Microsoft or IARC sources:

| ID | Authority | Official source | Retrieved |
|---|---|---|---|
| S1 | Microsoft | [Microsoft Store Policies](https://learn.microsoft.com/en-us/windows/apps/publish/store-policies) | 2026-07-14 |
| S2 | Microsoft | [Store Policies change history](https://learn.microsoft.com/en-us/windows/apps/publish/store-policies-change-history) | 2026-07-14 |
| S3 | Microsoft | [App Quality](https://learn.microsoft.com/en-us/windows/apps/publish/store-app-quality) | 2026-07-14 |
| S4 | Microsoft | [MSIX Store listing fields](https://learn.microsoft.com/en-us/windows/apps/publish/publish-your-app/msix/add-and-edit-store-listing-info) | 2026-07-14 |
| S5 | Microsoft | [MSIX screenshots and images](https://learn.microsoft.com/en-us/windows/apps/publish/publish-your-app/msix/screenshots-and-images) | 2026-07-14 |
| S6 | Microsoft | [MSIX app properties](https://learn.microsoft.com/en-us/windows/apps/publish/publish-your-app/msix/enter-app-properties) | 2026-07-14 |
| S7 | Microsoft | [MSIX additional information](https://learn.microsoft.com/en-us/windows/apps/publish/publish-your-app/msix/add-additional-information) | 2026-07-14 |
| S8 | Microsoft | [Store submission FAQ](https://learn.microsoft.com/en-us/windows/apps/publish/faq/submit-your-app) | 2026-07-14 |
| S9 | Microsoft | [Generate age ratings for MSIX apps](https://learn.microsoft.com/en-us/windows/apps/publish/publish-your-app/msix/age-ratings) | 2026-07-14 |
| S10 | Microsoft | [Windows App Certification Kit](https://learn.microsoft.com/en-us/windows/uwp/debug-test-perf/windows-app-certification-kit) | 2026-07-14 |
| S11 | IARC | [About IARC](https://globalratings.com/about/) | 2026-07-14 |
| S12 | IARC | [IARC FAQs](https://globalratings.com/faq/) | 2026-07-14 |
| S13 | IARC | [How IARC works](https://globalratings.com/how-iarc-works/) | 2026-07-14 |

`App Quality` is Microsoft guidance, not an additional Store-policy section.
Words such as “suggest” and “recommend” below remain recommendations unless a
separate policy or Partner Center validation makes them mandatory.

## Requirements matrix

| Rule | Classification | Current requirement applied to Pulsar | Implementation owner | Required evidence / gate |
|---|---|---|---|---|
| 10.1 and 10.1.1 | Mandatory | Product name, description, category, keywords, screenshots, ratings and claims must match the submitted build, its primary functions and material limitations. Optional Spotify and Telegram integrations cannot be described as prerequisites. | `TASK-260712-e1ie4x`, `TASK-260712-2s4e9p`, `TASK-260712-38lssj` | `TASK-260712-e5mfqj` captures real submitted-build UI evidence; `TASK-260712-1xik11` checks the frozen listing against the build. Sources S1, S4. |
| 10.3 and 10.3.1 | Mandatory, conditional credential clause | The product must be testable. A working demo account is required only when the product actually requires login credentials. Pulsar's primary reviewer flow must therefore be truly accountless rather than accompanied by fabricated credentials. | `TASK-260712-1cdoxh`, `TASK-260712-2s4e9p` | Deterministic reviewer-path fixtures plus real-app execution in `TASK-260712-e5mfqj`; final evidence index in `TASK-260712-1xik11`. Sources S1, S8. |
| 10.3.2 | Mandatory when a server is required | The exact coordinator origin configured in the submitted build must stay reachable and functional for the entire certification window. Health alone is insufficient: Create/Join, upload, target, fetch and receipt operations used by the notes must work. Optional Spotify and Telegram services are outside the primary path and cannot mask a coordinator outage. | `TASK-260712-1cdoxh`, `TASK-260712-2s4e9p` | Record candidate identity, coordinator origin, sanitized health/transaction evidence, certification-window owner and rollback contact; freeze it in `TASK-260712-1xik11`. Source S1. |
| 10.5 and 10.5.1 | Mandatory | Because Pulsar is a Win32 product and handles account, device and user-audio data, Partner Center needs a current privacy URL. The policy must describe accessed data, purpose, storage, security, disclosure, user controls and access, and evolve with features. | `TASK-260712-1epb3a`, `TASK-260712-1x0lot` | `TASK-260712-e1ie4x` wires the approved URL/copy; `TASK-260712-1xik11` verifies reachable reviewed content. Approved target: `https://barycenter.live/legal/privacy`. Sources S1, S6. |
| 10.5.4 | Mandatory | Personal information must be collected, stored and transmitted securely using modern cryptography. Store copy must not repeat the obsolete “collects no personal data” claim. | `TASK-260712-1epb3a`, plus the relevant identity/media security tasks already or later accepted | Security review and policy/build comparison in `TASK-260712-wy05n6` and `TASK-260712-1xik11`. Source S1. |
| 10.6 | Mandatory | Package capabilities must correspond to shipped functions and must not bypass OS checks. Microphone permission belongs behind an explicit Record action; denial must leave builtin cue and file intake useful. | `TASK-260712-e1ie4x` | Manifest/schema and current WACK output in `TASK-260712-1cdoxh`; real prompt/denial observation in `TASK-260712-e5mfqj`. Sources S1, S6, S10. |
| 10.7 | Mandatory | Every declared language needs localized product text and listing text, equivalent behavior, and explicit disclosure of any unavailable localized feature. English controls legally, but RU must remain semantically equivalent. | `TASK-260712-e1ie4x`, `TASK-260712-2s4e9p` | EN/RU build/listing comparison and locale-specific screenshots in `TASK-260712-e5mfqj`; final index in `TASK-260712-1xik11`. Sources S1, S4, S5. |
| 11.11 | Mandatory | Partner Center must generate an age rating from an accurately completed IARC questionnaire. The answer set must describe the submitted build's user audio, online communication, user interaction and internet access. | `TASK-260712-2s4e9p` | Preserve exported answers, generated rating/certificate ID and reviewer sign-off in `TASK-260712-1xik11`. Sources S1, S9, S11-S13. |
| 11.11.3 | Conditional | If accessible UGC can exceed the assigned rating, the product needs the policy's opt-in/filter or pre-existing-account protection. This cannot be declared “not applicable” until the generated rating and content behavior are compared. | `TASK-260712-1epb3a`, `TASK-260712-2s4e9p` | IARC answer review and content-guideline comparison in `TASK-260712-1xik11`. Sources S1, S9. |
| 11.12 | Mandatory because Pulsar carries online user audio | Publish terms or content guidelines; provide in-product reporting or proactive detection; and be able to remove or disable UGC when Microsoft requests it. | `TASK-260712-1epb3a`, `TASK-260712-1x0lot`, `TASK-260712-3t9nr8`, `TASK-260712-pbfz37` | The canonical operator backend is accepted in `TASK-260712-2kec2s`; Windows UI, published pages, operational runbook, listing/IARC pack and final readiness must prove the full path. Sources S1. |

## Why no demo credentials are supplied

Section 10.3.1 does not say every networked app needs a demo account. It says a
working demo account must be provided **if login credentials are required**.
The corrective Pulsar reviewer path is designed around capabilities that do
not require a pre-existing username, password, Spotify account, Telegram
account or secret supplied in certification notes:

1. launch the submitted Windows package;
2. run `Try locally` with the bundled cue, then a local file if desired;
3. choose `Create Barycenter`, which provisions the app without conventional
   account login;
4. record a short clip after the explicit microphone prompt, or use a local
   file when microphone access is denied;
5. target `This Pulsar`, play it and observe the terminal receipt;
6. open history, settings and the report/block surface.

This reasoning is valid only after the listed UI/capture tasks implement the
flow and the exact submitted package is exercised. `TASK-260712-2s4e9p` must
put these steps in certification notes and state plainly that Spotify,
Telegram and demo credentials are not required. If the submitted build later
introduces required login, 10.3.1 changes the answer: a working reviewer
account must then be supplied.

The accountless choice does **not** remove 10.3.2. The submitted build's
coordinator is a required server and must remain functional throughout
certification. `TASK-260712-1cdoxh` owns the reproducible service check and
artifact shape; `TASK-260712-2s4e9p` owns the exact origin, expected behavior
and escalation details in the notes; `TASK-260712-1xik11` refuses readiness if
the candidate origin is temporary, unreachable or not transactionally usable.

## Listing, screenshot and logo rules

### Official minimums

- Each Store listing needs a description and at least one screenshot (S4).
- A desktop screenshot must be PNG, landscape or portrait, at least
  1366 x 768, and no larger than 50 MB. Up to ten desktop screenshots are
  accepted (S5).
- Images and localized captions must be populated separately for each listing
  language, even when the underlying image is reused (S5).
- The required description is plain text up to 10,000 characters. Product
  features are optional, at most 20, and at most 200 characters each (S4).
- Keywords are limited to seven entries and must be relevant. Partner Center's
  current field guidance additionally limits each to 40 characters and the
  whole set to 21 words; policy 10.1.3 forbids pricing terms and other
  publishers' product titles (S1, S7).
- Store logos are optional for this non-Xbox app. A 300 x 300 PNG app tile icon
  is strongly recommended; if omitted, the Store uses the package image. If a
  Store icon is uploaded, it takes precedence over the package image (S4, S5).

### Microsoft guidance and Pulsar's corrective set

Microsoft recommends at least four screenshots per supported device family.
Its quality guidance says screenshots should show actual Windows app features
and distinct parts of the experience, rather than repeating nearly identical
views. Those are recommendations, but Pulsar needs stronger project evidence
because the July 2026 feedback says the prior listing showed only onboarding
or login-like UI.

For **each EN and RU listing**, the corrective set is six real screenshots from
the exact submitted Windows build:

1. main window showing connected/accountless product state;
2. `Try locally` with a completed builtin-cue or file result;
3. explicit active recording state and permission-safe controls;
4. routing with `This Pulsar` selected and a clip ready or sent;
5. played history with a terminal receipt;
6. settings plus the report/block/moderation entry surface.

The first five directly demonstrate primary functionality; the sixth proves
the Store-policy surface shipped with UGC. Onboarding may appear only as an
additional trailing image, never as the sole or leading proof. Captures must
contain no bearer token, invite/recovery code, personal audio name, local path,
private message or unrelated user data.

The UI implementation tasks own the renderable states:
`TASK-260712-9i5se7`, `TASK-260712-25at8b`, `TASK-260712-c7dmv8`,
`TASK-260712-1p8ykc`, `TASK-260712-2fe5bz` and `TASK-260712-pbfz37`.
The user-approved manual boundary assigns real-app capture and observation to
`TASK-260712-e5mfqj` in `EPIC-260714-th54l3`. Engineering task
`TASK-260712-2s4e9p` validates dimensions, locale mapping, redaction, captions
and listing assembly without claiming the manual capture passed.

## WACK and certification-note evidence

The current WACK guidance requires using the most current installed kit.
Command-line execution must occur in an active user session; it can validate an
installed package by package full name or an MSIX path and produces HTML and
XML reports. The Store may run every applicable test even when a local workflow
allows selection. Therefore the evidence record must contain:

- immutable MSIX SHA-256 and package identity;
- Windows and SDK/WACK versions;
- exact command and whether the package was installed or tested by path;
- complete sanitized HTML/XML result, with no deselected applicable tests;
- pass/fail and an owner for every warning or failure.

WACK is an engineering/package gate in `TASK-260712-1cdoxh` and
`TASK-260712-e1ie4x`; unavailable physical observations remain in the manual
epic. A WACK pass does not substitute for Store certification or real-app UI
evidence.

Partner Center describes certification notes as an optional submission field.
For Pulsar they are a project-required corrective artifact. The notes must name
Product ID `9P26FDCWV1GC`, exact package/build hash, the six accountless steps,
the exact coordinator origin and availability window, the expected microphone
prompt and denial fallback, and the explicit no-Spotify/no-Telegram/no-demo-
credentials statement. They must not contain production bearer credentials.

## IARC boundary

The approved product minimum age is 13 and Pulsar is not directed at children
under 13. That business position does not select the IARC result: Microsoft's
guidance says ratings describe content suitability, not the intended audience.
The questionnaire must accurately disclose shipped content and interactive
elements, including online user audio, user interaction and internet access.

IARC states that developers can access the questionnaire only through a
participating storefront's ingestion portal. Consequently this task does not
invent current question text or pre-answer a form that cannot be retrieved
publicly. `TASK-260712-2s4e9p` must export or capture the exact Partner Center
questionnaire presented for the immutable candidate, have Ivan Oparin review
the answers, save the generated rating and certificate ID, and retake it when a
content change can affect the rating.

## July 2026 certification findings

The repository contains planning summaries for two Partner Center findings,
but no raw Partner Center report artifact. The exact date is inconsistent
between the older files (`2026-07-09` and `2026-07-10`). This matrix therefore
records only “July 2026” and does not claim independent verification. Before
the findings are called closed, `TASK-260712-2s4e9p` must attach a sanitized
export or screenshot of the original report.

Also, `10.1.1.3` is the certification finding label recorded by the project;
the public v7.19 page exposes 10.1 and 10.1.1 but does not publish a separately
numbered 10.1.1.3 clause. Its normative anchor here is the accurate-
representation language in 10.1/10.1.1.

| Finding | Exact corrective evidence | Closure owner |
|---|---|---|
| `10.3.1 Product is Testable` | Submitted-build execution of the six-step accountless path; certification notes saying no Spotify, Telegram or demo credentials are required; exact functional coordinator origin and certification availability record; raw finding artifact. | `TASK-260712-1cdoxh`, `TASK-260712-2s4e9p`; manual observation `TASK-260712-e5mfqj`; readiness index `TASK-260712-1xik11`. |
| `10.1.1.3 Inaccurate Representation` | Six EN and six RU exact-build screenshots listed above, validated against current PNG/size/dimension/locale rules; listing text checked against shipped capabilities and limitations; raw finding artifact. | `TASK-260712-2s4e9p`; implementation tasks named in the screenshot section; manual capture `TASK-260712-e5mfqj`; readiness index `TASK-260712-1xik11`. |

## Mandatory pre-submit delta gate

Immediately before any external submission, `TASK-260712-2s4e9p` must reopen
S1-S13 and create a delta record containing:

- verification date and reviewer;
- current policy version, publish date and effective date;
- exact URLs and content hashes for every source relied upon;
- changed requirements, asset dimensions/limits and questionnaire changes;
- task IDs created or reopened for every material delta;
- an explicit `proceed` or `hold` decision.

`TASK-260712-1xik11` checks that record before presenting the payload to Ivan
Oparin. A material delta without an owner is a hold. The external Partner
Center mutation and real-app observations remain outside the engineering
workflow; this baseline only makes their prerequisites explicit and testable.

The checked-in record intentionally starts at `decision: hold`. Validate its
shape from the repository with:

```sh
cd coordinator
go run ./cmd/store-policy-check
```

The external workflow uses the fail-closed form below and will reject a hold,
the wrong release tag, a verification older than 24 hours, an unowned delta or
an incomplete source/hash set:

```sh
go run ./cmd/store-policy-check \
  --require-proceed \
  --max-age 24h \
  --tag vX.Y.Z
```

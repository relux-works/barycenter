# Phase 1 privacy and UGC policy pack

- Date: 2026-07-14
- Task: `TASK-260712-1epb3a`
- Version: 1.0
- Controlling language: English
- Canonical machine record:
  [`policy-pack-2026-07-14.json`](../compliance/policy-pack-2026-07-14.json)
- Publication state: **proceed** — Ivan Oparin approved the exact authored EN/RU
  source hashes from commit `43c0bd992e25c1e85aba6b7a086a94dad378eb35`
  for production publication on 2026-07-14.

This pack turns the approved legal/operations inputs and shipped Phase 1
contracts into publishable-source English and Russian documents. Exact source
approval does not itself prove that `/legal/*` is live. Publishing those routes,
wiring every surface and verifying deployed hashes belongs to
`TASK-260712-1x0lot`; Store submission continues to fail closed until its live
publication check passes.

## Artifacts

| Artifact | English source | Russian source | Canonical URL |
| --- | --- | --- | --- |
| Privacy Policy | [`docs/legal/en/privacy.md`](../legal/en/privacy.md) | [`docs/legal/ru/privacy.md`](../legal/ru/privacy.md) | `https://barycenter.live/legal/privacy` |
| Terms of Service | [`docs/legal/en/terms.md`](../legal/en/terms.md) | [`docs/legal/ru/terms.md`](../legal/ru/terms.md) | `https://barycenter.live/legal/terms` |
| Content Guidelines | [`docs/legal/en/content-guidelines.md`](../legal/en/content-guidelines.md) | [`docs/legal/ru/content-guidelines.md`](../legal/ru/content-guidelines.md) | `https://barycenter.live/legal/content-guidelines` |
| Upload/recording notice | [`docs/legal/en/upload-rights-notice.md`](../legal/en/upload-rights-notice.md) | [`docs/legal/ru/upload-rights-notice.md`](../legal/ru/upload-rights-notice.md) | in-product notice linking Terms and Guidelines |
| Support and safety contacts | [`docs/legal/en/support.md`](../legal/en/support.md) | [`docs/legal/ru/support.md`](../legal/ru/support.md) | `https://barycenter.live/legal/support` |

## Disclosure coverage from specification sections 15.1 and 15.2

| Required disclosure/control | Policy sections | Engineering/evidence owner |
| --- | --- | --- |
| Explicit microphone permission; no hidden/background capture; local fallback | `P-03`, `P-04`, `P-06`, `U-04` | `TASK-260712-30abcm`, `TASK-260712-2w4gyw`, `TASK-260712-1s6h6t`, `TASK-260712-1p8ykc`; real-app observation remains manual `TASK-260712-e5mfqj` |
| What audio and metadata is collected and why | `P-02` through `P-05`, `P-07` | Generic ingest tasks `TASK-260712-z6h6wh` through `TASK-260712-jolzhh` |
| Current media is readable by the coordinator, not E2EE | `P-07`, `T-02`, `U-02` | Current plaintext processing and ACL contracts; future E2EE story `STORY-260712-1frfmi` must trigger policy delta review |
| Accountless identity, credential storage and recovery limitation | `P-02`, `P-03`, `P-07`, `P-11`, `T-03` | `TASK-260712-1bpog0`, `TASK-260712-m5264f`, `TASK-260712-2u1w16`, `TASK-260712-47uve0`, `TASK-260712-3v1k7q` |
| Explicit immutable target scope; blocks and DND; recipient-copy limitation | `P-03`, `P-08`, `P-10`, `P-11`, `T-05`, `T-08`, `U-02`, `U-03` | Transmission story `STORY-260712-25lysg`; `TASK-260712-1c1ska`; manual playback stays outside this task |
| Retention, delete, asynchronous cleanup, evidence hold and backup limit | `P-09`, `P-10`, `T-10`, `U-03` | `TASK-260712-1sae4q`, `TASK-260712-gj0cko`, `TASK-260712-2kec2s`, `docs/backup-restore.md` |
| UGC terms/guidelines, uploader rights and recording consent | `T-04` through `T-07`, `C-01` through `C-08`, `U-01` | Microsoft Store policy 11.12; UI placement tasks listed below |
| In-product Report, block and operator disable/delete | `P-03`, `P-08`, `P-11`, `T-09`, `C-09`, `C-10`, `U-03` | `TASK-260712-2kec2s`, `TASK-260712-pbfz37`, `TASK-260712-34stvx`, `TASK-260712-dlltnr` |
| Moderation mailbox, runbook and Microsoft removal | `P-14`, `T-09`, `C-09`, `C-10` | `TASK-260712-3t9nr8`; approved operator inputs in `legal-ops-inputs.json` |
| UGC/child-safety and accurate IARC questionnaire | `P-12`, `T-01`, `C-02`, `C-03`, `C-10` | `TASK-260712-2s4e9p`, `TASK-260712-1xik11`; the 13+ service minimum is not represented as an IARC result |
| Optional Telegram/Spotify and no public commercial-music rebroadcast | `P-03`, `P-04`, `P-08`, `P-10`, `T-02`, `T-07`, `C-07` | Telegram compatibility and parity tasks; optional Spotify integration and current third-party rules |

## Product-surface cross-reference

| Surface | Required artifact/use | Owning task |
| --- | --- | --- |
| Website `/legal/*` | Render immutable versioned EN/RU sources; expose language switch and current effective date | `TASK-260712-1x0lot` |
| Windows/macOS first run and Settings/About | Link Privacy, Terms, Guidelines and Support; show controlling language and version | `TASK-260712-e1ie4x`, UI integration tasks in `STORY-260712-2e36uz` |
| Record/upload/send confirmation | Show `U-01` through `U-04` before the first external submission and keep links available thereafter | `TASK-260712-2lrpc0`, capture/UI tasks in `STORY-260712-2e36uz` |
| Report/block UI | Link Content Guidelines and describe evidence access/retention without exposing another user | `TASK-260712-pbfz37`, `TASK-260712-34stvx` |
| Telegram bot | Set the bot's own Privacy URL; link Terms/Guidelines around upload/report flows | `TASK-260712-dlltnr`, `TASK-260712-1x0lot` |
| Microsoft Store listing and Partner Center | Privacy URL, accurate UGC/IARC answers, certification notes, EN/RU policy links | `TASK-260712-2s4e9p`, `TASK-260712-1xik11` |
| Support/moderation operations | Use exact public language for normal/urgent targets, evidence window and available actions | `TASK-260712-3t9nr8` |

No row above claims that a real application surface or public URL was manually
observed. Those claims stay in the separate manual-test epic or the later
publication task as appropriate.

## Factual traceability

Every public section has the same stable ID in EN and RU. The machine record
maps each ID to one or more of the following evidence classes:

- `approved-input` — identity, contacts, locations, age, law, reviewer and
  operating targets in `docs/compliance/legal-ops-inputs.json`;
- `shipped-control` — exact coordinator/client code or an accepted contract
  describing the current enforcement path;
- `product-boundary` — an explicit limitation such as non-E2EE, target-limited
  rather than recipient-proof sharing, non-guaranteed delivery or absence of
  magical recovery;
- `official-rule` — the current official Microsoft, Telegram, Spotify, FTC,
  California or European Commission source recorded with retrieval date.

The validator requires every EN section ID to exist exactly once in RU and in
the traceability set, verifies exact file hashes, rejects placeholder language
and unsupported claims such as “end-to-end encrypted”, “anonymous”, “instant
erasure from backups” or “guaranteed delivery”, and compares public entity,
contact, URL and retention constants with the approved machine input.

## External-source review

Retrieved 2026-07-14:

- [Microsoft Store Policies 7.19](https://learn.microsoft.com/en-us/windows/apps/publish/store-policies), especially 10.5 personal information, 10.6 capabilities, 10.7 localization, 11.11 age rating and 11.12 UGC;
- [Microsoft age-rating guidance](https://learn.microsoft.com/en-us/windows/apps/publish/publish-your-app/pwa/age-ratings), which distinguishes content suitability from target audience age;
- [Telegram privacy policy](https://telegram.org/privacy) and [Bot Developer Terms](https://telegram.org/tos/bot-developers), which require the bot operator to disclose received data, purpose and retention and to delete unnecessary/requested bot data;
- [Spotify Developer Policy](https://developer.spotify.com/policy) and [Developer Terms](https://developer.spotify.com/terms), including transparency, disconnect/data control and restrictions on public/commercial or mixed streaming use;
- [FTC COPPA rule page](https://www.ftc.gov/legal-library/browse/rules/childrens-online-privacy-protection-rule-coppa), for the under-13 boundary;
- [European Commission privacy-notice guidance](https://commission.europa.eu/law/law-topic/data-protection/rules-business-and-organisations/principles-gdpr/what-information-must-be-given-individuals-whose-data-collected_en), for controller, purpose, basis, retention, recipients, transfer and rights disclosures; and
- [California Attorney General CCPA overview](https://oag.ca.gov/privacy/ccpa), for applicable know/delete/correct/opt-out/non-discrimination disclosures. Pulsar states that it does not sell or cross-contextually share personal information rather than presenting an inapplicable sale opt-out.

These sources inform the pack; they do not substitute for factual product
controls. Ivan Oparin approved the exact source hashes for publication.
Separate counsel review is not required by the approved input; any later
source-byte change must produce new hashes and a new owner review before
publication.

## Delta triggers

Re-run the policy review and change the version before any of these reaches
production: a new data category or recipient; provider/subprocessor or region
change; retention change; public targeting or broadcast; advertising or sale;
payment; new third-party integration; new minor-facing feature; E2EE; proactive
content scanning; automated decision/profile; materially different moderation
action; or a changed official Store/Telegram/Spotify rule. A code-only change
that does not alter public facts still requires the validator and hash check.

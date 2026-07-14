# Pulsar Privacy Policy

Version 1.0. Effective and last updated: 14 July 2026.
Canonical URL: https://barycenter.live/legal/privacy

English is the controlling version. The Russian translation is provided for
convenience and is intended to have the same meaning.

## P-01. Who controls your data

Relux Works LLC (registration number 999.110.1559507, tax ID 03036828),
registered at Orbeli Brothers St. 19, Tsaghkadzor, Kotayq, Armenia 2310, is the
controller of personal data processed by Pulsar. Questions and privacy
requests may be sent to privacy@relux.works. Product support is available at
support@barycenter.live and https://barycenter.live/legal/support.

## P-02. Scope and product model

This Policy applies to the Pulsar desktop applications, coordinator service
and optional Pulsar Telegram bot. Pulsar lets a person record or upload a
short audio clip and route it to explicitly selected Pulsar people or devices.
The primary product is accountless: it does not require an email address,
username, password, Telegram account or Spotify account. It instead uses
opaque installation, actor and group identifiers and high-entropy device,
control and recovery credentials.

## P-03. Data we process

Depending on the features you use, Pulsar processes:

- audio and upload data: recordings or files you deliberately submit, the
  filename or title you provide, content type, size, duration, codec,
  normalization measurements, creation and expiry times, and processing
  status;
- accountless identity data: opaque actor, Barycenter, installation and device
  identifiers, membership and role, credential digests, non-secret recovery
  handles, and credential issue, rotation, revocation and use events;
- routing and activity data: the exact recipients selected when a transmission
  is accepted, delivery mode, DND and block settings, delivery receipts,
  playback state, timestamps and failure reasons;
- moderation data: a report category, optional report text, the reported media
  and routing metadata, restricted evidence copy, report status, operator
  access and enforcement audit records;
- device and operations data: platform, app version, declared media
  capabilities, connection state, coarse error and health counters, request
  time and security/rate-limit events. The service may temporarily process a
  network address to establish a connection and limit abuse; durable
  rate-limit records use a class-separated digest rather than the raw subject;
- optional Telegram data: the Telegram user, chat, message and file identifiers
  and profile label delivered to the bot, commands and audio you send to it,
  and the bot replies; and
- optional Spotify data: track URI and display metadata, playback state and
  device-control events needed for the optional personal Spotify integration.
  Pulsar does not ask the coordinator to store your Spotify password.

Pulsar is not designed to collect precise location, address-book contacts,
payment information, health information or advertising identifiers. Please do
not put sensitive personal information into a clip or report unless it is
necessary and you have the right to share it.

## P-04. Where data comes from

We receive data directly from actions you take in Pulsar, from another Pulsar
user who targets you, from your Pulsar devices, and from Telegram or Spotify
only when you choose the corresponding integration. Operating systems tell the
app whether the microphone permission is granted; Pulsar does not bypass that
control. We may also receive a lawful request or a moderation request from
Microsoft or another authorized authority.

## P-05. Why we use data

We use the data only to provide the action you request; authenticate an
accountless installation; process, deliver and display status for targeted
audio; operate optional integrations; apply DND, blocking and deletion;
investigate reports and enforce the Content Guidelines; secure, debug and
recover the service; comply with law; and establish, exercise or defend legal
claims. Depending on applicable law, these purposes rely on performance of our
agreement with you, your consent or deliberate request, our legitimate
interests in operating and protecting Pulsar, or a legal obligation. We do not
sell personal information, use it for cross-context behavioural advertising,
or use clips to train generative-AI models.

## P-06. Microphone and local self-test

Pulsar requests microphone permission only after an explicit Record action.
While capture is active, the app must show a visual and audible indication.
Pulsar has no hidden always-on pre-roll or background recording. Denying
permission does not prevent file playback or the built-in local test. A local
self-test stays on the device unless you separately choose to upload or send
its output.

## P-07. Processing, security and the non-E2EE limitation

Remote supported origins use HTTPS or WSS and accountless credentials are
stored on the server as digests. The macOS and Windows clients use the
operating system's protected credential storage. Access to a clip is checked
against its owner or the immutable recipient snapshot, current credential
generation, expiry, block and revocation state. Operator credentials are
separate and access to report evidence is audited.

Phase 1 Pulsar media is **not end-to-end encrypted**. The coordinator receives
the submitted audio in readable form so that it can validate and normalize the
file, deliver it to selected recipients and, when reported, make a restricted
evidence copy available to an authorized moderator. Do not use Pulsar for data
that requires end-to-end encryption.

## P-08. Who receives data

We disclose data only as needed to:

- the Pulsar people and devices explicitly captured as recipients of a
  transmission; they receive the clip and the minimum sender/routing context
  needed to present it;
- authorized Relux Works operators, who may access report metadata and a
  time-limited evidence copy only for support, security and moderation;
- Telegram or Spotify when you use their optional integration. Those services
  operate under their own terms and privacy policies. Telegram already
  receives material you send in Telegram and delivers it to the Pulsar bot;
  Spotify receives activity performed through its service;
- Microsoft as the app distributor and certification authority, to the extent
  data is submitted to or requested by Microsoft; and
- courts, regulators, law enforcement, rights holders or a successor to the
  service when disclosure is required by law or reasonably necessary to
  protect users, rights and the service.

The production coordinator and database backup infrastructure is operated by
Relux Works LLC in the United States. No subprocessors are used for coordinator
hosting or object backups. A future provider or subprocessor must be disclosed
before its production use.

Target-limited sharing is an access-control boundary, not a promise that a
recipient cannot hear, record or copy material after receiving it. Share only
with people you trust and only when you have the necessary rights and consent.

## P-09. Retention

Pulsar uses the following current server-side limits:

| Data | Current handling |
| --- | --- |
| Open upload capability and temporary upload | The capability expires after one hour. Failed or abandoned source bytes are collected by the maintenance worker and in all cases are scheduled for removal within 24 hours. |
| Ready audio clip | Available for at most seven days from creation unless the sender deletes it, it is disabled, or a shorter delivery limit applies. Physical cleanup is asynchronous after access is revoked. |
| Delivery and history metadata | Kept for up to 30 days where the corresponding history feature uses it; security, dispute or legal-hold needs may require a longer limited record. |
| Report text and restricted audio evidence | Available to moderation for up to 30 days after the report. The free-form text is then scrubbed and the evidence hold is released. |
| Content-free media-ingest audit | Kept for up to 90 days. Security, identity and moderation enforcement records that are needed to protect the service or prove an action may be retained for the life of the relevant Barycenter and afterwards only as reasonably required for security, disputes or law. |
| Accountless identity and settings | Kept while the installation or Barycenter remains active. Revocation or dissolution makes credentials and future access unusable; minimal tombstones and audit events may remain for integrity and abuse prevention. |
| SQLite recovery points | Kept for up to seven days in United States object storage. They contain database state, not canonical audio files. A restore must run current deletion and retention reconciliation before service resumes. |

A report may temporarily hold the exact reported audio until its 30-day
evidence deadline even if the ordinary seven-day clip limit or a sender delete
occurs first. A legal obligation or properly scoped legal hold may also delay
deletion; access remains restricted to that purpose.

## P-10. Deletion and its limits

An authorized sender can delete an active clip. Pulsar immediately marks it
unavailable for new downloads, cancels pending playback and schedules its
canonical bytes for durable cleanup; an already playing clip is stopped under
the current playback policy. Deletion does not take back audio already heard
or independently copied by a recipient. It also does not delete a separate
copy you sent through Telegram or data controlled independently by Telegram,
Spotify, Microsoft or a recipient.

Database recovery points may contain pre-deletion metadata for up to seven
days. They are not served as live data; if restored, current reconciliation
must reapply deletion before endpoints are enabled. Minimal content-free
security, moderation, transaction and tombstone records may remain when needed
for integrity, abuse prevention, legal compliance or dispute handling.

## P-11. Your choices and rights

You can decline microphone permission, use local testing without upload,
choose each recipient, apply DND, block a sender, report accessible foreign
media and delete media you own. You can stop an optional Telegram or Spotify
integration through that service and Pulsar's available controls.

Subject to applicable law, you may ask us to provide access to, correct,
delete, restrict or export your personal data, or object to certain processing
and withdraw consent for future processing. Send a request to
privacy@relux.works. Because Pulsar is accountless, we may ask you to prove
control of the relevant installation, recovery credential or Barycenter; we
will not disclose another person's data merely because a requester knows an
identifier. You may complain to the competent data-protection authority and
will not receive discriminatory treatment for exercising a privacy right.

## P-12. Children

Pulsar is for people aged 13 or older and is not directed at children under
13. A person under the age of legal majority may use it only with permission
of a parent or legal guardian where required. If we learn that we processed a
child's personal data contrary to applicable law, we will disable the relevant
access and delete the data subject to the limited retention rules above.
Contact privacy@relux.works with a child-safety privacy concern.

## P-13. International operation

Relux Works LLC is established in Armenia and the primary service and backup
regions are in the United States. Consequently, data may be processed outside
your country. Where applicable law requires a transfer safeguard, we will use
a lawful mechanism or will not offer the affected processing in that market.
Pulsar is unavailable in sanctioned, embargoed or otherwise legally
prohibited jurisdictions.

## P-14. Changes and contact

We will update this Policy before a materially different collection, use,
recipient or provider is put into production. We will change the version and
date and provide notice in the app or on the policy page when required. If a
change needs consent, it will not apply to that processing until consent is
obtained.

Privacy: privacy@relux.works. Support: support@barycenter.live. Moderation:
moderator@barycenter.live. Urgent removal: moderation-urgent@barycenter.live.
Mail: Relux Works LLC, Orbeli Brothers St. 19, Tsaghkadzor, Kotayq, Armenia
2310.

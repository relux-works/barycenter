# P3 E2EE report evidence and moderation export v1

Task: `TASK-260712-2i0w6x`

Status: production-dark engineering foundation; no HTTP route, storage adapter, or product capability is enabled

## Decision

An E2EE recipient can create a metadata-only moderation report about an opaque protected object without disclosing content. The report freezes the original object, sender, recipient, Air, epoch, generation, target snapshot, manifest digest, and ciphertext digest. It creates no evidence consent, evidence metadata, or evidence state row. This preserves reporting after protected-object access is revoked while making no claim that the coordinator can decrypt the object.

Evidence is a separate, voluntary transition. The intended client flow is local recipient decryption followed by an explicit `explicit_report_evidence_export` consent action. The coordinator accepts only an authenticated evidence digest and a bounded reference to a copy encrypted under the moderation storage boundary. It never accepts a content key, group secret, decrypted payload, or plaintext evidence field. The dormant store seam requires the exact report, reporter actor and device, protected object, current report revision, manifest binding, consent version/digest, evidence digest, MIME, size, expiry, and current recipient authorization. Revoked, deleted, stale, forked, removed, rejoined-under-new-lineage, disabled, or unverified access cannot create a new export. An exact replay is idempotent.

No evidence bytes are stored by this task. `encrypted_evidence_ref` and its independent at-rest ciphertext digest describe a future moderation-storage object; runtime upload and download wiring remains deferred. Consequently the implementation proves authorization, retention state, audit, and deletion of coordinator access to the reference, but does not claim physical deletion from a storage provider that is not yet connected.

## Operator boundary and audit

The existing moderation operator token domain is reused. Queue listing requires `List`, evidence-reference authorization and deletion require `Evidence`, and decisions require `Decide`. Revoked operator credentials fail closed. Evidence authorization returns only the scoped opaque reference and public binding metadata. It appends a content-free `evidence.read` audit event. Report creation, consent recording, evidence creation, evidence deletion, evidence expiry, decision request, and decision completion are also append-only audit events; neither evidence bytes nor report statements enter audit rows.

Evidence starts as `active`, has a maximum 30-day retention interval, and becomes terminal `deleted` or `expired`. A terminal reference cannot be authorized again. The report statement has the same bounded retention and is scrubbed by the expiry sweep. Consent and identity bindings remain immutable audit metadata. Because no storage adapter exists, `deleted` and `expired` mean coordinator authorization is revoked and the future adapter must delete the referenced ciphertext before acknowledging the same transition.

## Moderation decisions

The dormant moderation service supports `no_action`, `delete_media`, `disable_actor`, and `disable_orbit`. E2EE `delete_media` reuses the opaque protected-object terminal transition: server-held ciphertext chunks are removed, the object becomes `deleted`, and future manifest/range access fails. The operation is available only for the exact pending decision and authenticated deciding operator. Actor and Orbit actions reuse the canonical identity revocation, transmission cancellation, socket-disconnect, inbox, and saved-cue enforcement paths. Decision begin/complete and protected-object deletion are crash-retryable and idempotent.

These methods are not registered by the coordinator command and are not advertised as a capability. A future runtime must authenticate the caller's verified device rather than trust a supplied device ID, upload the moderation-at-rest copy through a reviewed storage adapter, call these exact seams, and delete that copy on terminal transition.

## Verification and limits

Unit coverage proves metadata-only creation, zero evidence rows before consent, revoked-object export denial, exact consent/report/device binding, capability enforcement, operator revocation, read/delete/expiry audit, append-only audit tamper rejection, checkpoint rollback, canonical ciphertext chunk deletion, restart replay, and the dormant service decision path. Existing coordinator regressions remain in scope.

Fixture ciphertext and digests are not proof of a production cryptographic suite, secure client memory, or transport confidentiality. No physical device, signed package, traffic capture, live mailbox, provider deletion, or real-app moderation flow was tested. Those claims remain in `EPIC-260714-th54l3`; production crypto and external security approval remain open gates.

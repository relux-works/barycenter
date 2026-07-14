# Pulsar moderation operations runbook

- Task: `TASK-260712-3t9nr8`
- Accountable owner, primary operator, backup and escalation: Ivan Oparin
- Coverage: Monday-Friday 10:00-19:00 GMT+4
- Ordinary response target: 2 business days
- Urgent-removal response target: 24 hours
- Machine-readable contract: `docs/compliance/moderation-operations.json`

This runbook operates the least-privilege control plane documented in
`docs/moderation-control-plane.md`. It does not authorize direct SQLite edits,
object-storage browsing, emailing evidence, or copying bearer tokens into a
ticket.

## Mailbox state and accountable rotation

Approved public addresses are `support@barycenter.live` for product support,
`moderator@barycenter.live` for ordinary reports, and
`moderation-urgent@barycenter.live` for urgent removal and verified Microsoft
requests. Ivan Oparin owns all three roles under the approved initial default.
That is accountable but not personnel-redundant; inability to respond is an
incident and escalates to the same owner until a distinct backup is appointed.

DNS inspection at 2026-07-14T20:28:36+04:00 found no MX record for
`barycenter.live`. Delivery is therefore **not claimed ready**. Provider-side
routing and a synthetic delivery check are tracked outside the 205-task
engineering inventory as `TASK-260714-200ib8`. The proposed default is
Cloudflare Email Routing from all three aliases to one private, verified Ivan
Oparin destination. The private destination must not be committed here.

After provider setup, update `observed_mx` and `delivery_state` in the contract,
then run:

```sh
cd coordinator
go run ./cmd/moderation-ops-check --require-mail-ready
```

Send only a synthetic message containing no user identifiers or audio. Record
timestamp, source alias, receipt and owner acknowledgment in the external task.

## Intake and reporter-safe communication

For each message record only:

- received time, source mailbox and whether the sender claims Microsoft or a
  competent authority;
- Pulsar report ID (`rp_...`), media ID, product/package ID and concise policy
  category when supplied;
- verification state, response due time, assigned operator and final action;
- the control-plane audit export, kept in the restricted case record.

Ask the reporter for the minimum missing identifier. Do not request passwords,
operator tokens, recipient identity, an audio attachment, or unrelated account
data. Never reveal another user's actor/Barycenter identifiers, report details,
decision rationale, evidence state or enforcement history. A user-facing
`GET /v1/reports/{report_id}` response is authoritative and deliberately says
only `received` or `reviewed`.

Do not email audio. Do not paste audio, report free text, storage keys, bearer
tokens or local paths into ordinary logs or tickets. If a message itself
contains sensitive media, restrict the original message and do not forward it.

## Operator credential lifecycle

Stop other coordinator writers, provision the narrowest role, capture the token
once into the operator's credential manager, and restart normal service:

```sh
duet-coordinator --config /etc/duet/coordinator.yml \
  --provision-moderation-operator "Ivan Oparin queue" \
  --moderation-operator-scopes list

duet-coordinator --config /etc/duet/coordinator.yml \
  --provision-moderation-operator "Ivan Oparin reviewer" \
  --moderation-operator-scopes list,evidence,decide
```

Use distinct tokens when queue-only work can be separated from evidence and
decision work. Review credentials quarterly and immediately after a device or
role change. Revoke a lost, copied, inactive or replaced token by operator ID:

```sh
duet-coordinator --config /etc/duet/coordinator.yml \
  --revoke-moderation-operator op_XXXXXXXXXXXXXXXXXXXXXXXXXX
```

Revocation is audited and immediate. Never send the plaintext token by email.

## Queue, evidence, decision and audit commands

Set the HTTPS origin and retrieve the bearer from the credential manager into
the shell without command history. The examples use `curl`; the server, not the
shell, resolves operator identity from the bearer.

```sh
export PULSAR_MODERATION_URL=https://coordinator.example.invalid
export PULSAR_MODERATION_TOKEN=mod_REDACTED

curl --fail-with-body --silent --show-error \
  -H "Authorization: Bearer $PULSAR_MODERATION_TOKEN" \
  "$PULSAR_MODERATION_URL/v1/moderation/reports?status=open&limit=50"
```

Triage the reason, accepted-target snapshot, age and urgency before opening
evidence. Metadata-only review is preferred. If audio is necessary, use a
private `0700` directory, never a shared Downloads folder:

```sh
install -d -m 0700 "$HOME/.pulsar-moderation"
umask 077
curl --fail-with-body --silent --show-error \
  -H "Authorization: Bearer $PULSAR_MODERATION_TOKEN" \
  -o "$HOME/.pulsar-moderation/rp_REPLACE.evidence" \
  "$PULSAR_MODERATION_URL/v1/moderation/reports/rp_REPLACE/evidence"
```

Every successful evidence descriptor open appends `evidence.read` before bytes
are served. Delete the local working copy after the decision; the server's
restricted evidence hold remains independently governed by the 30-day window.

Apply exactly one action. Repeating the same request is a crash-safe retry; a
different action returns conflict rather than rewriting history.

```sh
curl --fail-with-body --silent --show-error \
  -H "Authorization: Bearer $PULSAR_MODERATION_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"action":"no_action"}' \
  "$PULSAR_MODERATION_URL/v1/moderation/reports/rp_REPLACE/decision"

curl --fail-with-body --silent --show-error \
  -H "Authorization: Bearer $PULSAR_MODERATION_TOKEN" \
  "$PULSAR_MODERATION_URL/v1/moderation/reports/rp_REPLACE/audit?limit=500"
```

The audit response is report-scoped and content-free. It can contain operator
and actor IDs, event type, action and timestamp; it cannot contain report text,
evidence, storage identity, bearer material or local paths.

## Decision policy and recovery boundary

| Action | Use | Effect and recovery |
| --- | --- | --- |
| `no_action` | Evidence does not substantiate a violation or the request is unsupported. | No content or credential enforcement occurs. The decision record is immutable; keep the audit export and send only the reporter-safe reviewed status. |
| `delete_media` | The exact clip must no longer be distributed. | Access is revoked, pending work is cancelled and active playback receives fade-stop. Physical cleanup is asynchronous; once cleanup occurs, physically deleted audio is not recoverable. Never promise rollback. |
| `disable_actor` | One actor/device identity is compromised or repeatedly abusive. | Credentials are revoked, sockets close and source/target work is cancelled. Do not edit revocation history. If later proven mistaken, an unaffected owner may issue a new device invite; the old credential is never restored. |
| `disable_orbit` | The whole Barycenter is abusive or compromised. | All orbit credentials and work are disabled. If later proven mistaken, owner-approved recovery creates a fresh Barycenter through `POST /v1/onboarding/orbits`; it does not resurrect deleted media, old credentials or forbidden content. |

For a mistaken decision, stop further work, preserve the audit export, notify
Ivan Oparin and document the correction. Only access restoration through fresh
credentials is recoverable. Do not modify append-only audit rows, reopen the
same report with a different action, or restore content that was forbidden or
physically deleted.

## Verified Microsoft removal requests

Treat the word "Microsoft" in an email as unverified. Confirm the product ID
`9P26FDCWV1GC`, package identity `ReluxWorksLLC.PulsarBarycenter`, request or
case identifier, sender channel and requested scope against Partner Center or a
known Microsoft support channel controlled by Ivan Oparin. Do not follow login
links or execute attachments from the inbound message.

A verified request must identify an existing Pulsar report or enough exact
media/report metadata to locate the immutable queue snapshot. If it does not,
reply for the missing identifiers and classify it as unsupported; do not use a
direct database edit or broad storage search. Once located, preserve the
verification reference outside the coordinator without message bodies or
audio, choose the narrowest applicable `delete_media`, `disable_actor` or
`disable_orbit` action, execute through `/decision`, and export `/audit`.
Urgent verified removals use the 24 hours target. Ordinary requests use the
2 business days target.

## Retention, backups and incidents

Report free text and restricted evidence are available to moderation for at
most 30 days. Ordinary ready media is normally bounded to seven days; an
evidence hold may delay physical cleanup until its own deadline. Append-only,
content-free audit and minimal enforcement records survive evidence scrubbing.
Backups are disaster-recovery copies with the same access restrictions, not an
operator undo button and not a way to retrieve deleted content on demand.

Escalate immediately when evidence access fails after authorization, an audit
event is missing, a token may have leaked, an urgent request cannot be verified,
mail delivery fails, the sole operator is unavailable, or enforcement only
partially applies. Revoke suspect credentials, preserve content-free IDs and
timestamps, avoid repeated conflicting decisions, and use the canonical retry
of the same action after the underlying service is healthy.

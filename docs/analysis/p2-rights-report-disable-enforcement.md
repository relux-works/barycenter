# P2 rights, report, and disable enforcement

`TASK-260712-2zoy4u` extends the existing Phase 1 owners instead of creating a
second report, block, delete, or authorization store. The enforcement boundary
is the frozen `p2-targets-inbox-parity.v1` contract.

## Canonical decision matrix

| Event | Immediate scope | Future open / refill | Inbox and replay | Scheduler | Global media state |
|---|---|---|---|---|---|
| Reporter submits a report | Reporting actor and reported media only | Denied by the immutable-target ACL's canonical `moderation_reports` check | That actor's matching items become terminally `unavailable/reported`; replay returns the ordinary non-existence surface | Only that reporting actor's targets are disarmed with `reported`; a shared Telegram evidence target never cancels another actor's node | Unchanged |
| Reporter blocks actor or orbit | Existing Phase 1 block owner/scope | Denied by the existing block predicate | Existing block projection applies | Existing source-to-node cancellation applies | Unchanged |
| Sender or moderator deletes media | All non-moderator consumers | Denied before a new descriptor is opened | All matching items become unavailable; no replay | Existing media lifecycle cancellation applies | Terminal `deleted` |
| Moderator disables actor | Disabled source and its current bindings | Denied by actor revocation | Source and target items become unavailable | Existing source and node cancellation applies | Media rows retained for evidence/retention |
| Moderator disables orbit | Disabled tenant and its current bindings | Denied by orbit status and credential revocation | Source and target items become unavailable | Existing source-orbit and node cancellation applies | Media rows retained for evidence/retention |

A report is therefore not a global censorship primitive. It cannot delete
media, disable an actor or orbit, quarantine content, or cancel another
recipient. There is no count-based escalation. The separately frozen global
quarantine concept remains unavailable until an explicit reversible,
operator-authorized implementation is reviewed; this task does not silently
manufacture that authority.

`reported` is an internal target/inbox reason. Sender-facing target and receipt
projections redact it, so a cancellation cannot identify the reporter; the
reporter receives only the existing idempotent report outcome.

## Read and playback linearization

The persisted-target download transaction checks current credentials, media
state, active blocks, and the reporting actor's report immediately before it
opens the canonical descriptor. A report, delete, or disable that commits
first prevents the open. An already acquired descriptor may finish, matching
the Phase 1 active-delete contract, but grants no later open or cache refill.

The same predicate is media-kind agnostic. The later streamed-track range and
cache implementation must call the persisted-target authorization seam for
every initial open and refill; it must not copy the SQL or cache a positive
decision across requests. Target evaluation also marks a later delivery of
reported media to that actor as `blocked/reported`, before queue or playback
work exists.

## Rights and audit ownership

User-media upload, transmission creation, and explicit replay continue to use
the server-owned current content-policy grant. App uploads additionally require
the per-upload rights acknowledgement, which is a reminder and never evidence
of ownership. Report evidence and the content-free `report.created` audit stay
in the canonical moderation tables. Operator delete/disable decisions retain
their existing least-privilege evidence and audit flow.

## Automated evidence

- Store integration proves one report revokes only the reporter's fetch,
  inbox, replay, and future delivery while an unrelated target and the ready
  media remain available.
- Moderation service coverage proves exact target cancellation and idempotent
  retry without peer cancellation.
- Existing HTTP and Telegram history tests exercise the same common report
  service; direct report and history report routes own no separate persistence.
- Existing media lifecycle, moderation, content-policy, previous-head, race,
  and platform acceptance suites remain the regression boundary.

No real-device, real-network, or production moderation outcome is claimed by
this engineering task.

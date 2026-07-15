# P2 approach-to-Air compatibility handoff

`TASK-260712-2bjdlb` keeps the existing Telegram approach vocabulary while
making the Air model the only shared-runtime authority after cutover.

## Authority boundary

- In `links_authoritative` and `airs_shadow`, `/approach`, `/accept`,
  `/decline` and `/apart` retain the legacy link implementation.
- In `airs_authoritative`, those commands use only Air memberships, invites,
  active pointers and stable `air_id` runtime resolution. Legacy link writes
  remain rejected.
- `rollback_hold` serves no shared runtime. An Air mutation after cutover
  increments divergence, so rollback cannot revive the frozen active-link
  snapshot after an alias leave.

The no-argument `/approach` transaction creates the parked Air, owner
membership, creator pointer and one member invite together. The 12-character
uppercase compatibility code is derived from the invite ID with the Air HMAC
key. SQLite stores only its keyed hash, so a coordinator restart can reproduce
the same code for an idempotent retry without storing plaintext.

## Pairwise behavior after cutover

1. `/approach CODE` consumes the invite into `pending_confirmation` and never
   activates or changes the joining barycenter's current pointer.
2. The joining primary runs `/accept`. The confirmation joins the membership,
   installs its pointer and activates the stable Air. The issuer cannot
   confirm on the joining primary's behalf.
3. A current pointer to another Air blocks implicit confirmation. The bot asks
   the user to leave the current approach instead of silently switching.
4. `/decline` supports claimant decline, issuer cancellation and open-invite
   withdrawal. The abandoned one-member alias Air is tombstoned so both
   barycenters can start again.
5. `/apart` removes only the caller's membership and pointer. If the caller
   owned the Air, ownership transfers to the oldest remaining joined member.
   The remaining membership and pointer survive; a two-member Air parks.

Activation preserves the legacy donor selection and personal-session parking
behavior, then reconciles the serialized runtime by stable Air ID. Duplicate
create, consume and confirm deliveries return the durable result without
creating another Air, runtime or notification broadcast. `/home`, `/status`
and notifications render barycenter titles only and never expose an Air,
membership or legacy link identifier.

## Verification

- Store tests cover idempotent create/consume/confirm, invite-secret storage,
  decline, withdrawal, another-current-Air guard, restart and owner-local
  leave.
- Migration coverage starts with an active legacy link, cuts it over,
  restarts, leaves through the alias and proves unsafe rollback enters
  `rollback_hold` instead of resurrecting the link runtime.
- Telegram-loop coverage proves the joining-side confirmation, donor/runtime
  handoff, human `/home` copy, duplicate-delivery suppression, restart and
  caller-only `/apart` behavior.
- Full Go tests, vet, targeted race tests and exact previous-head rollback are
  the acceptance commands for this change.

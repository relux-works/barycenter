# Idea: Air rooms — many homes, one broadcast

**STATUS: approved direction for phase 2 (2026-07-12); not implemented.** The
canonical implementation plan is now `docs/spec-self-contained-audio.md` §13
and §20. Shipped §12 approaches (pairwise links) remain the runtime canon until
that migration lands. This note retains the original proposal and its design
history.

## Why not chains of links

Extending §12 links transitively (A—B, B—C) makes A and C share an air only
because B linked both — surprising to everyone involved, painful to govern
(who may /apart whom?), and ambiguous for content ownership. A chain is the
wrong shape for "compania of friends listening together".

## Proposal: a separate Air entity

| Entity     | Role                                                                    |
|------------|-------------------------------------------------------------------------|
| Barycenter | The permanent home: members, Pulsars, settings, personal music          |
| Air        | An ephemeral room: 2..N barycenters, one shared track/queue/voice flow  |

User flow:

1. A primary creates an air and gets a join code.
2. Other barycenters enter by code; each primary confirms participation.
3. The current track keeps playing; newly joined homes catch up seamlessly
   (the beta.26 join mechanics).
4. Every track and voice message gets a single ordinal at coordinator
   acceptance time — one deterministic order across all member barycenters.
5. A home that goes dark becomes *sleeping*: it never pauses the others and
   catches up on return (living-air semantics generalized).
6. `/leave_air` detaches ONE barycenter, preserving its personal state; the
   air lives on for the rest.

## Technical readiness

Most of the machinery already exists: the FSM is N-wise over a peer set,
composite home ids (`orbit:slot`) are shipped, and beta.26 added seamless
join-in-progress. The genuinely new work is:

- `airs` + `air_members` tables and the room lifecycle (create/join/confirm/
  leave/dissolve, empty-room GC);
- routing: `stateFor` resolving through air membership instead of pairwise
  `links`;
- explicit voice recipient sets (today a personal voice with >1 potential
  recipient silently becomes broadcast);
- governance texts and role rules for N-party rooms.

## Proposed first-stage limits

- 8 barycenters or 20 concurrently connected Pulsars per air, whichever hits
  first. Spotify audio still streams per home, but phase-2 uploaded tracks add
  coordinator storage/egress; the implementation gate therefore includes
  media quotas and a synthetic 8-barycenter/20-Pulsar load test.

## Resolution of the original open questions

- Air replaces links as the runtime entity; `/approach` and `/apart` remain
  compatibility aliases, and active links migrate to two-member Air rooms.
- Track/voice permissions are explicit per-Air policies; local DND/block is
  always stronger.
- Empty/single-member Air rooms park. Their eventual GC retention remains a
  phase-2 gate decision.
- Join is invite/code based from Pulsar or Telegram. There is no public
  discovery in the approved scope.

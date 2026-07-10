# Idea: Air rooms — many homes, one broadcast

**STATUS: idea under discussion (2026-07-10). NO decision made.** Nothing here
is committed work; §12 approaches (pairwise links) remain the shipped canon.
This note captures the multi-barycenter proposal so the discussion has a fixed
reference instead of scattered chat threads.

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
  first. Audio streams straight from Spotify per home; the coordinator only
  carries commands and voice files, so the practical ceiling is the single
  coordinator + SQLite (no HA) — not the network.

## Open questions (the actual discussion)

- Does Air replace §12 links entirely (a 2-member air == today's approach) or
  coexist with them? Replacement avoids two parallel mechanisms but needs a
  migration for the active link in prod.
- Who may inject tracks/voices in an air: any member home, or per-air policy
  (mirror of takeover_policy)?
- Air persistence: rooms die on empty, on timer, or persist until dissolved?
- Discovery/invite UX: code-only (like approaches) or bot-side room list?

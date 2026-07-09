# Barycenter Multi-Tenant & Shared-Orbit Design (v2.1 draft)

Customer requirements (2026-07-04): the bot/service must be usable by people
with zero context ("as if found in a store") — instructions, descriptions,
centralized support of MANY barycenters (dozens, not thousands); and a live
broadcast must be shareable down a chain — additional partners (polyamory
context) or a group of friends listening to one air and throwing tracks in.

## 1. Vocabulary shift

| v1 term | v2.1 term |
|---|---|
| the system (one couple) | **Orbit** — one shared-listening space (tenant). "Барицентр" in user language |
| node a / node b | **Pulsar (slot)** — N per orbit, dynamically allocated (a, b, c, …) |
| the two users | **Members** with roles |

One coordinator process hosts many orbits. SQLite stays (dozens of orbits =
trivial load); every table gains `orbit_id`.

## 2. Roles

Star-system naming (customer 2026-07-04), decentralized by design — primary
is just the brightest star, not an "owner", and the title is transferable:

| Role | Powers |
|---|---|
| **primary** | orbit administration: /share settings, /revoke slots, /make_primary transfer, delete orbit — plus everything a companion can |
| **companion** | full playback control: links, playlists, voice, skip/pause/vol, periastron/apoastron. Default role for invitees |
| **satellite** | bot-only member (no Pulsar node): adds tracks and voices, no skip/pause/mode. IN SCOPE for M1 (customer) |

`/make_primary @user` transfers the title; if the primary leaves, the oldest
companion inherits it.

## 3. Identity & auth (unchanged principles, new scoping)

- Human identity = Telegram user_id (no passwords, ever). An orbit's member
  list is its allowlist; the bot resolves (user_id -> orbit, role). One user
  can belong to multiple orbits later; MVP: one orbit per user.
- Node identity = per-node 256-bit token issued at pairing, now mapping to
  (orbit_id, slot). Transport wss + TLS as designed. Keychain storage on the
  node. `/revoke <slot>` regenerates.
- Bot chats: the orbit binds to whichever chats its members talk from; group
  chat binding via /start in the group by any member (orbit inferred from the
  inviter's membership).

## 4. Onboarding walkthrough (the "found it in a store" path)

1. Person installs Pulsar (TestFlight/notarized dmg; Store later).
2. First launch: a single screen — "Свяжи меня с твоим Барицентром: напиши
   @barycenter_bot и пришли код" + a code entry field.
3. In Telegram: /start -> bot explains itself in two sentences ->
   "Создать барицентр" button (deep-link payload) -> orbit created, user is
   host -> bot answers with a **pairing code** (TTL 5 min).
4. Code goes into the app -> `POST /pair {code}` -> {orbit, slot, token,
   ws_url} -> node online -> bot: "Пульсар A в сети. Пригласи партнёра:
   <t.me/barycenter_bot?start=inv_XXXX>".
5. Partner taps the invite link -> joins the orbit as partner -> gets their
   own pairing code -> same app flow -> orbit has two pulsars -> the offline
   gate lifts and the air can start.
6. Chain sharing = the same invite link mechanism, issued by any member
   (host-configurable later): /share -> fresh invite link -> new member ->
   optionally pairs their own Pulsar (slot c, d, …) or stays app-less
   (bot-only member who can throw tracks without a home node — cheap and
   useful for the friend-group case).

Support surface: /help (already), a static landing at barycenter.relux.works
(instructions + download links), bot answers for every error in human words
(already the style).

## 5. Sync mechanics with N pulsars

The v1 two-node cycle generalizes cleanly because it never depended on "two"
conceptually — only in code constants:

- In shared/together mode, a Spotify track selected on any Pulsar is reported
  with its audible position and becomes the common element; the usual barrier
  below synchronizes every online home. Telegram links remain an optional
  queue input, not the primary play action.
- load -> ready from **all online pulsars of the orbit** -> resume_at(T) to
  all; T = now + 2·max(rtt_i) + margin.
- ended when **all** report eof (or laggard within 1 s of duration).
- Voice targets become member-sets. Orbit setting `voice_default =
  personal | broadcast`, **default personal** (customer): in a two-member
  orbit the addressee is obvious; in larger orbits an unaddressed voice makes
  the bot ask who it is for. "Broadcast" voices play at every pulsar
  (sender's included).
- Degradation: any pulsar offline >12 s -> orbit pauses (same rule, N-wise);
  the offline gate waits for **all** paired pulsars. A member without a node
  (bot-only listener) never blocks the air.
- Desync journal: max pairwise |started_i - started_j|.

FSM changes: `bothNodes` constant -> orbit peer set; per-orbit Session
instances keyed by orbit_id in the loop; hub connections tagged (orbit, slot).
Protocol: node_id field already a string — slots beyond "a"/"b" are wire-
compatible (golden set gains no new types; register/welcome unchanged).

## 6. Data model (SQLite)

```
orbits(id, title, created_at, takeover_policy, …per-orbit settings)
members(orbit_id, tg_user_id, role, joined_at)
slots(orbit_id, slot, token_hash, paired_at, revoked_at NULL)
invites(orbit_id, code, kind: pair|member, issued_by, expires_at, used_at NULL)
elements/media/events: + orbit_id
settings: global only; per-orbit settings move into orbits
```

Tokens stored hashed (sha256) server-side; the plain value lives only on the
node (Keychain).

## 7. Phasing (each phase shippable)

- **M1 — multi-orbit core**: orbit_id everywhere, per-orbit Session map,
  /start-creates-orbit, pairing codes + POST /pair, invite links, member
  allowlist from DB (drops DUET_TELEGRAM_USERS), two-pulsar orbits only.
  Katya's placeholder dies here.
- **M2 — N pulsars**: peer-set FSM (ready/ended/degraded N-wise), slots c+,
  chain invites for node-owning members. Acceptance: three local pulsars in
  one orbit start a track within the desync target.
- **M3 — bot-only members & listener role**: track/voice contribution without
  a node; role enforcement in command handling.
- **M4 — polish**: landing page, /orbit info, multi-orbit-per-user if wanted.

## 8. Open questions for the customer

1. RESOLVED: primary/companion/satellite, transferable primary.
2. RESOLVED: voice_default orbit setting, default personal.
3. RESOLVED: satellites (bot-only) in M1.
4. Orbit limits: soft-configured (global env + per-orbit override), defaults
   5 pulsars / 10 members — NOT hardcoded (customer).

## 12. Personal barycenters & approaches (CANON, customer decision 2026-07-07)

An orbit (barycenter) is a PERSONAL, permanent space — like an account: your
pulsars, your settings, your solo music. It never dies, merges or gets
absorbed. Togetherness is an APPROACH (сближение) — a separate link entity
between barycenters. Periastron thereby becomes literally correct astronomy:
the closest point of TWO systems, not a mode inside one.

Consequences:
- /create for everyone is the NORM, not a mistake (the two-hosts problem
  dissolves). An invite becomes a proposal to approach, accepted mutually.
- Breaking up is painless: the link dissolves, each side keeps everything.
- Poly/friend groups = a graph of links between personal systems.
- companion/satellite roles remain for guests INSIDE a personal barycenter.

Mechanics (L1 — link of two):
- store: links(id, orbit_a, orbit_b, state proposed|active, proposed_by);
  one active link per orbit in L1.
- While a link is active, ONE shared session lives at the link level; its
  peer set = union of both orbits' slots (M2 peer-set FSM is already N-wise).
  Peer ids are composite "orbit:slot" inside the session; bot texts render
  them as "дом a@<primary name>".
- Bot flow: /approach -> one-time approach code; the other primary sends
  /approach CODE -> the initiator gets an inline confirmation -> both said
  yes -> sessions of both orbits stop, the link session starts (shared,
  idle) with a notify to everyone. /apart (either side) breaks the link:
  the group session ends, each orbit returns to its own solo session.
- Phases: L1 pair link; L2 groups >2 (link graph -> one session per
  connected component); L3 guest members inside a personal barycenter.

# STORY-260712-2ori1t decomposition

## Scope anchor

This story covers the phase-two long-audio path that turns uploaded
`audio_track` media into a bounded-memory main-program source on Windows and
macOS:

- `audio_track` ingest on top of the generic media substrate;
- canonical compressed server-side variants;
- authenticated range or chunk serving with revocation;
- bounded disk cache plus incremental decoder rings on both node platforms;
- buffer-ready synchronization, queue or replace, pause, seek, resume and ended;
- progress, metadata and recovery without full download or unbounded memory;
- coexistence with current clip playback and Spotify-backed main-program flows.

It does **not** own codec-path selection itself, Air membership lifecycle,
explicit target or inbox product behavior, or the phase-two acceptance and beta
gate. Those remain separate stories and are called out below as dependencies.

## Created tasks

1. `TASK-260712-1n5fks` - Add streamed-track persistence and variant schema scaffold
2. `TASK-260712-285pag` - Implement audio_track ingest validation and canonical variants
3. `TASK-260712-3lf8r0` - Serve streamed-track variants with range auth and revocation
4. `TASK-260712-31rkpe` - Land streamed-track wire contract and compatibility policy
5. `TASK-260712-2h6snp` - Add coordinator streamed-track main-program orchestration
6. `TASK-260712-3aj8w2` - Implement macOS streamed-track cache, decoder and player path
7. `TASK-260712-17w78q` - Implement Windows streamed-track cache, decoder and player path
8. `TASK-260712-1fpb9q` - Prove streamed-track memory, sync and regression behavior
9. `TASK-260712-2ubzyf` - Document streamed-track rollout, limits and handoff

## Within-story dependency graph

- `TASK-260712-285pag` blocked by `TASK-260712-1n5fks`
- `TASK-260712-3lf8r0` blocked by `TASK-260712-1n5fks`, `TASK-260712-285pag`
- `TASK-260712-2h6snp` blocked by `TASK-260712-1n5fks`, `TASK-260712-285pag`, `TASK-260712-3lf8r0`, `TASK-260712-31rkpe`
- `TASK-260712-3aj8w2` blocked by `TASK-260712-285pag`, `TASK-260712-3lf8r0`, `TASK-260712-31rkpe`, `TASK-260712-2h6snp`
- `TASK-260712-17w78q` blocked by `TASK-260712-285pag`, `TASK-260712-3lf8r0`, `TASK-260712-31rkpe`, `TASK-260712-2h6snp`
- `TASK-260712-1fpb9q` blocked by `TASK-260712-3lf8r0`, `TASK-260712-2h6snp`, `TASK-260712-3aj8w2`, `TASK-260712-17w78q`
- `TASK-260712-2ubzyf` blocked by `TASK-260712-1fpb9q`

Execution intent:

- Start by extending the generic media schema for track variants and recovery
  fields.
- Once the codec spike fixes the variant contract, land `audio_track` ingest
  and the revocable range-fetch surface.
- Freeze the wire contract in parallel with the server-side streaming surface,
  then teach the coordinator to schedule buffered starts and track seeks.
- Implement macOS and Windows player paths against the same range plus
  capability contract.
- Finish with measured one-hour regression evidence, then document rollout and
  handoff.

## Cross-story dependencies

Hard upstream blockers:

- `TASK-260712-z6h6wh` - generic media ingest persistence scaffold
  - Required before streamed-track schema work can extend the shared media model.
- `TASK-260712-2af2dp` and `TASK-260712-1bnos4`
  - Required before `audio_track` can reuse SubmitMedia validation and resumable upload sessions.
- `STORY-260712-3l1r1u` - codec and streaming player spike
  - Must choose the canonical variant matrix, decoder contract and cache limits before production ingest and wire payloads are finalized.
- `TASK-260712-3mcof4`, `TASK-260712-gj0cko`, `TASK-260712-1aprcb`
  - Provide the ACL snapshot, delete and revocation basis for range fetch.
- `TASK-260712-51y5k9` and `TASK-260712-1g70av`
  - Freeze the transmission and capability contract patterns extended by streamed tracks.
- `TASK-260712-2qpp6w` and `TASK-260712-31vvjt`
  - Supply transmission acceptance and scheduler ownership reused by track queue or replace.
- `TASK-260712-1hqiek`
  - Supplies the render-safe prepared-media baseline the new platform track paths build on.

Bidirectional coordination seams:

- `STORY-260712-3v14m9` - Air rooms and approach migration
  - Air owns membership, join, leave and living-air routing. This story owns the track-specific buffered start, seek and leave-during-track behavior that plugs into those hooks.
- `STORY-260712-ob1tx2` - explicit targets, inbox and transport parity
  - That story owns the final N-recipient target model, inbox and replay semantics. This story extends range authorization and streamed-track delivery to consume that model without inventing a second ACL path.

Downstream consumers:

- `STORY-260712-1qfbiw` - phase-two acceptance, capacity and rollout
  - Consumes the one-hour evidence, memory metrics and compatibility notes created here.

## Recommended implementation constraints

- Keep one server-chosen canonical variant matrix per uploaded track rather than
  per-client transcoding or ad hoc negotiation.
- Reuse the existing coordinator-clock scheduled-start path where possible; the
  recommended default is new `stream_*` load or ready verbs plus the existing
  `resume_at` fire point unless the codec spike proves a stricter alternative.
- Range authorization must stay header-authenticated and revocable; do not
  switch phase-two track delivery to bearer secrets in query strings.
- Seek must rebuild the decoder window from the requested position, not replay
  from zero and not require full local redownload.
- Cache limits should be byte-based and explicit in code and metrics so B1 RSS
  and eviction behavior are testable.

## Completeness check against story AC

- One-hour ingest, canonical variants, start-before-full-download and bounded
  memory are covered by `TASK-260712-285pag`, `TASK-260712-3lf8r0`,
  `TASK-260712-3aj8w2`, `TASK-260712-17w78q` and proven in `TASK-260712-1fpb9q`.
- Cross-target synchronized start, queue or replace, pause, seek, resume and
  ended are covered by `TASK-260712-31rkpe`, `TASK-260712-2h6snp`,
  `TASK-260712-3aj8w2`, `TASK-260712-17w78q` and measured in
  `TASK-260712-1fpb9q`.
- Cache eviction recovery, range resume and delete or abuse revocation are
  covered by `TASK-260712-3lf8r0`, the two platform player tasks, and the
  regression task.
- Quotas, rights, deletion and ACL are handled by `TASK-260712-285pag` and
  `TASK-260712-3lf8r0`, with explicit upstream dependence on the shared ingest
  and target-snapshot stories instead of duplicating ownership here.
- Clip and Spotify non-regression is called out in the coordinator task, both
  platform tasks and the final regression task.

No extra research task was created inside this story because the unresolved
codec-path choice already exists as the explicit blocking story
`STORY-260712-3l1r1u`.

## Workflow note

Several tasks are intentionally blocked on upstream P1 and sibling P2 stories.
That is necessary to keep the long-track implementation from copying the wrong
ACL, protocol or decoder decision into a second code path. The tasks are still
atomic and ready for implementation as soon as those blockers clear.

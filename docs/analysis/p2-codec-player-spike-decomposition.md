# P2 Codec and Streaming Player Spike Decomposition

Story: `STORY-260712-3l1r1u`

## Current implementation anchor

- macOS today has a clip or voice path built around `AVAudioEngine`,
  `VoiceCache`, and file-backed inserts, plus Spotify PCM from the FIFO ring.
- Windows today mirrors the clip or voice path with a ring-fed render engine,
  `VoiceCache`, and a local WAV decoder.
- Coordinator media handling is still canonical WAV for short media; there is
  no bounded-memory long-track path, no `stream_variants` schema, and no range
  transport contract yet.
- `docs/spec-providers.md` sketches a whole-download `AVAudioFile` path for an
  early provider experiment, but that does not satisfy spec section 20.2 or
  B1 because phase 2 requires start before full download and bounded memory on
  two-hour media.

## Task set

1. `TASK-260712-14u0yk` Freeze the codec spike rubric, fixture corpus and measurement harness
   - Blocking foundation task that turns spec section 20.2 and the relevant
     section 20.5 gates into one shared proof harness, artifact format, and
     candidate shortlist.
   - Frozen handoff: `docs/analysis/p2-codec-spike-rubric-fixtures-harness.md`.
2. `TASK-260712-dqdoqj` Prototype canonical stream variants and the range cache contract
   - Defines the server-side transport shape the candidates actually consume:
     `stream_variants`, byte ranges, auth, integrity, and bounded disk cache.
3. `TASK-260712-1vdlkw` Audit codec licenses, patent posture, and distribution constraints
   - Produces the legal and packaging matrix for Media Foundation, pure-Go,
     and bundled decoder families before selection.
4. `TASK-260712-3vkcki` Probe the pure-Go streaming decoder path on both nodes
   - Runs one cross-platform pure-Go candidate through incremental fetch,
     decode, scheduled start, pause, seek, resume, and two-hour RSS checks.
5. `TASK-260712-298tyq` Probe the Windows Media Foundation AppContainer path
   - Tests whether a packaged native Windows bridge can satisfy the same proof
     matrix without sandbox weakening or hidden packaging assumptions.
6. `TASK-260712-1canzv` Probe the bundled signed decoder path on Windows and macOS
   - Evaluates a distributable bundled decoder family, including package size,
     codesign, notarization, and MSIX redistribution evidence.
7. `TASK-260712-ibuaxj` Run the comparative sync, seek, and memory evidence matrix
   - Executes the shared two-hour, seek, skew, cache, and mixed-platform proof
     pack across all viable candidates and preserves rejected-option evidence.
8. `TASK-260712-2eympi` Publish the codec player ADR and implementation handoff contract
   - Selects the production path and defines the exact interfaces, cache
     ceilings, `stream_variants` contract, and fixture set for phase-2
     implementation and acceptance work.

## Execution shape

- Blocking foundation: `TASK-260712-14u0yk`
- Parallel after foundation:
  - `TASK-260712-dqdoqj`
  - `TASK-260712-1vdlkw`
- Candidate prototype path after the shared transport contract:
  - `TASK-260712-dqdoqj` -> `TASK-260712-3vkcki`
  - `TASK-260712-dqdoqj` + Phase-1 Windows packaged baseline ->
    `TASK-260712-298tyq`
  - `TASK-260712-dqdoqj` -> `TASK-260712-1canzv`
- Comparative proof:
  - `TASK-260712-dqdoqj` + `TASK-260712-1vdlkw` +
    `TASK-260712-3vkcki` + `TASK-260712-298tyq` +
    `TASK-260712-1canzv` -> `TASK-260712-ibuaxj`
- Final ADR and handoff:
  - `TASK-260712-dqdoqj` + `TASK-260712-1vdlkw` +
    `TASK-260712-ibuaxj` -> `TASK-260712-2eympi`

## Completeness check

- Covered:
  - candidate shortlist, fixture corpus, proof harness, and artifact format
  - server-side canonical variants, byte-range transport, and bounded cache
  - license, patent, Store, notarization, and redistribution review
  - pure-Go, Media Foundation, and bundled decoder prototypes
  - MP3, AAC, and Opus decode on both platforms where the candidate claims to
    be viable
  - scheduled start, pause, seek, resume, end-of-stream, and two-hour RSS
  - rejected-option evidence and final production-path ADR
  - exact handoff contract for `stream_variants`, cache ceilings, adapter
    seams, fixtures, and downstream acceptance work
- Explicit gap closed with blocker:
  - no candidate task is allowed to invent its own fixtures, thresholds,
    evidence format, or shortlist; `TASK-260712-14u0yk` freezes that first
- External dependency made explicit:
  - `TASK-260712-298tyq` reuses the packaged MSIX and AppContainer posture
    from the phase-one Windows spike instead of creating a second Windows
    baseline
- Diagrams attached:
  - `p2-codec-player-spike-components.puml`
  - `p2-codec-player-spike-sequence.puml`

# STORY-260712-1i0doc decomposition

## Scope anchor

This story closes the phase one Store compliance and acceptance seam that is not
owned by the feature stories for onboarding, ingest, transmission, mixer or the
UI shells. It owns the policy artifacts, report and moderation surfaces,
manifest and metadata honesty, Store asset pack, and the final A1 to A8 plus
phase one exit evidence gate.

## Current implementation findings

- The repository has no privacy policy, Terms of Service, content guidelines,
  IARC worksheet or moderation runbook files yet.
- docs/store-listing.md still markets the old Spotify first product and the
  screenshot guidance is centered on the rejected onboarding only listing.
- docs/acceptance-run.md is still the older duet phase one checklist and does
  not define the self contained A1 to A8 acceptance environment.
- pulsar-win/msix/AppxManifest.xml.in declares network capabilities only; the
  phase one microphone capability from spec section 15.1 is still missing.
- Code search found no coordinator report, block, moderation audit, reporter
  status or operator workflow implementation yet.

## Created tasks

1. TASK-260712-1epb3a - Draft privacy, UGC and rights policy pack
2. TASK-260712-2kec2s - Implement abuse report, block and moderation control plane
3. TASK-260712-pbfz37 - Add Windows report, block and owner delete surfaces
4. TASK-260712-34stvx - Add macOS report, block and owner delete surfaces
5. TASK-260712-dlltnr - Add Telegram and shared history moderation parity
6. TASK-260712-3t9nr8 - Publish moderation mailbox and operator runbook
7. TASK-260712-e1ie4x - Wire microphone declarations and honest RU and EN product copy
8. TASK-260712-2s4e9p - Prepare Store listing, IARC and certification asset pack
9. TASK-260712-1cdoxh - Repair the phase one acceptance environment and regression gate
10. TASK-260712-1xik11 - Run A1 to A8 evidence, non functional gates and Store submission

## Within story dependency graph

- TASK-260712-pbfz37 blocked by TASK-260712-1epb3a and TASK-260712-2kec2s
- TASK-260712-34stvx blocked by TASK-260712-1epb3a and TASK-260712-2kec2s
- TASK-260712-dlltnr blocked by TASK-260712-1epb3a and TASK-260712-2kec2s
- TASK-260712-3t9nr8 blocked by TASK-260712-1epb3a and TASK-260712-2kec2s
- TASK-260712-e1ie4x blocked by TASK-260712-1epb3a
- TASK-260712-2s4e9p blocked by TASK-260712-1epb3a, TASK-260712-pbfz37, TASK-260712-34stvx, TASK-260712-dlltnr, TASK-260712-3t9nr8 and TASK-260712-e1ie4x
- TASK-260712-1xik11 blocked by TASK-260712-2kec2s, TASK-260712-pbfz37, TASK-260712-34stvx, TASK-260712-dlltnr, TASK-260712-3t9nr8, TASK-260712-e1ie4x, TASK-260712-2s4e9p and TASK-260712-1cdoxh

Execution intent:

- Start policy and moderation control plane work immediately and in parallel.
- Land platform declarations and copy once the policy vocabulary is stable.
- Integrate user facing moderation actions separately on Windows, macOS and
  Telegram or shared history instead of hiding channel specific work inside one
  oversized task.
- Repair the acceptance environment before the final evidence run so live and
  automated artifacts are reproducible.
- Finish with the Store asset pack and then the full A1 to A8 plus submission
  gate.

## Cross story dependencies

- STORY-260712-2ve1c8 identity and self service onboarding
  - TASK-260712-1bpog0 provides actor and membership foundations that the
    moderation control plane uses for reporter, blocked actor and disable
    semantics.
  - TASK-260712-m5264f provides control actor APIs and auth boundaries needed
    for report creation, status lookup and operator actions.
  - TASK-260712-38qsku should be reused when rollback coverage expands to the
    new moderation rows.
- STORY-260712-ld674h media ingest and storage
  - TASK-260712-z6h6wh and TASK-260712-gj0cko provide the additive media
    persistence, delete and ACL lifecycle this story must enforce after
    moderation actions.
  - TASK-260712-3huupe should absorb regression cases where delete or disable
    changes fetchability or retention outcomes.
- STORY-260712-25lysg transmission protocol and scheduler
  - Story level dependency until tasks exist. It owns DND, block semantics,
    target receipts and phase one offline behavior that this story must expose
    honestly in policy, UI wording and final evidence.
- STORY-260712-34kbkn Telegram adapter, history and presence
  - Story level dependency until tasks exist. It owns the Telegram and shared
    history shells whose labels and inline actions this story extends with
    moderation actions.
- STORY-260712-2e36uz main UI, local self test and capture
  - TASK-260712-2fe5bz and TASK-260712-3dqc3l provide the Windows and macOS
    routing and history surfaces that moderation actions attach to.
  - TASK-260712-e5mfqj should feed sanitized real screenshots and UI evidence
    into the Store asset pack.
- STORY-260712-30ju1k Windows Store platform spike
  - TASK-260712-13rbnw and TASK-260712-1vtwkl provide the signed MSIX proof
    and Windows 10 or 11 evidence matrix that certification notes and the final
    A1 or A8 pack must reference.
- STORY-260712-fes2jj overlay and interrupt mixer
  - Story level dependency for A3 and A4 evidence. This story should not claim
    overlay or interrupt readiness until the mixer story has produced the live
    and deterministic proof.

## Completeness check against story AC

- Privacy, microphone and UGC obligations from spec section 15 are covered by
  TASK-260712-1epb3a, TASK-260712-2kec2s, TASK-260712-3t9nr8, TASK-260712-e1ie4x and TASK-260712-2s4e9p.
- In product report, block, delete and reporter safe status paths are covered
  by TASK-260712-2kec2s, TASK-260712-pbfz37, TASK-260712-34stvx and TASK-260712-dlltnr.
- RU and EN product and listing honesty are covered by TASK-260712-1epb3a, TASK-260712-e1ie4x and TASK-260712-2s4e9p.
- A1 to A8, phase one non functional gates, migration, rollback and platform
  evidence are covered by TASK-260712-1cdoxh and TASK-260712-1xik11.
- The existing repo gaps for outdated listing copy, missing policy artifacts,
  missing microphone declaration and missing moderation implementation are all
  turned into explicit tasks rather than hidden follow up.

## Workflow note

The board keeps newly created child tasks in backlog while they are unassigned.
They now have scope, acceptance criteria, checklists and within story
dependencies, so a developer can claim any unblocked task directly.

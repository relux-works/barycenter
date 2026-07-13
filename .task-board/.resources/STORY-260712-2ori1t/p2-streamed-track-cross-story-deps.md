# STORY-260712-2ori1t cross-story dependency note

## Hard blockers already linked on tasks

- `TASK-260712-z6h6wh`
- `TASK-260712-2af2dp`
- `TASK-260712-1bnos4`
- `TASK-260712-3mcof4`
- `TASK-260712-gj0cko`
- `TASK-260712-1aprcb`
- `TASK-260712-51y5k9`
- `TASK-260712-1g70av`
- `TASK-260712-2qpp6w`
- `TASK-260712-31vvjt`
- `TASK-260712-1hqiek`
- `STORY-260712-3l1r1u`
- `STORY-260712-3v14m9`
- `STORY-260712-ob1tx2`

## Seam ownership

`STORY-260712-3l1r1u` owns:

- codec and container selection;
- license review and Store compatibility proof;
- exact decoder and cache interface contract used by this story.

`STORY-260712-3v14m9` owns:

- Air persistence and lifecycle;
- join, leave, park and dissolve semantics;
- route membership and living-air catch-up membership decisions.

This story owns:

- how an uploaded track becomes a queueable main-program source;
- how nodes fetch, buffer, seek and resume that source;
- how the coordinator schedules buffered starts and recovers progress.

`STORY-260712-ob1tx2` owns:

- N-recipient target snapshots as a product feature;
- inbox, replay, delete, report and parity UX;
- explicit target and offline semantics in UI and transport flows.

This story owns only the streamed-track side of that seam:

- range authorization against the frozen target model;
- streamed-track receipts and progress fields needed by players;
- track-specific revocation behavior after delete, report or disable.

## Downstream handoff

`STORY-260712-1qfbiw` needs from this story:

- measured one-hour start, seek and RSS evidence;
- mixed clip, Spotify and streamed-track regression results;
- explicit notes on what remains external to the repo test matrix;
- rollout notes for `streamed_tracks` and rollback expectations.

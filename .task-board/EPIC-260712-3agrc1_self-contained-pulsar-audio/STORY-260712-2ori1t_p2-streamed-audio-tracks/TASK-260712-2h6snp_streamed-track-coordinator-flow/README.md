# Orchestrate uploaded tracks as a general main-program source

## Description
Extend the coordinator and Session FSM through a provider-neutral main-program adapter rather than baking a second Spotify-shaped branch into the loop.

## Scope
Represent uploaded track, Spotify and future main sources behind explicit load, audible-position, pause, seek, resume, drain and ended semantics. Implement queue and replace according to the frozen contract, trusted ordering, persisted current source and progress, stream load and generation-ready barriers, scheduled start, seek-to-ready barrier, partial supported targets, rebuffer or lag policy and restart or reconnect. Integrate Air join catch-up to the current audible position and Air leave without owning membership. Serialize clips independently and keep overlay, interrupt and personal pause behavior. End only after decoder and output ring drain.

## Acceptance Criteria
Uploaded tracks queue or replace without corrupting Spotify or clip state; ready supported targets meet timing while unsupported targets are explicit. Pause, seek and resume are generation-safe, living-Air join catches up the main track but never old overlay, leave stops only leavers, coordinator restart preserves one current source and queue, and ended or progress reflects audible output rather than downloaded or decoded-ahead bytes.

# Land clip transmission wire contract and capability negotiation

## Description
Extend the canonical Go protocol and both client mirrors so every phase-one clip transmission message and capability flag is encoded identically and versioned with the golden files.

## Scope
Add protocol message types and payloads for prepare_media, media_ready, play_media_at, media_started, media_ended, media_failed, media_cancelled, and the DND and presence additions required by the clarified contract. Add capability flags media_clip_v1, overlay_mix_v1, and interrupt_resume_v1; update docs/protocol.md; and keep the Go codec, Windows mirror, and Swift contract tests aligned while preserving legacy play_voice compatibility.

## Acceptance Criteria
Every new transmission message round-trips through Go, Windows, and Swift against golden JSON and compatibility tests. Legacy play_voice and solo_voice remain decodable and encodable for mixed-version deployments. The coordinator can tell exactly which targets support clip transmission, overlay mixing, and interrupt resume before choosing the delivery path.

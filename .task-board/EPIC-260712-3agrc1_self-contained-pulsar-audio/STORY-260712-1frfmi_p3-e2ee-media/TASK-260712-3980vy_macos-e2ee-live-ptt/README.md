# Encrypt and authenticate macOS live PTT end to end

## Description
Bridge macOS live sender and receiver frames to the reviewed group state without giving the coordinator plaintext.

## Scope
Derive a fresh per-session key from the authorized epoch, bind Air, targets, sender device, session, generation, sequence, codec and timing into associated data and encrypt every bounded frame with unique nonce. Decrypt and authenticate before jitter decode, reject replay, gaps outside policy, stale epoch and removed sender, rotate or terminate on membership change and keep crypto off capture and render callbacks. Preserve C1-C2 latency, FEC or PLC policy, DND, backpressure and no persistence.

## Acceptance Criteria
All macOS source and target pairings preserve C1-C2 under encryption, coordinator capture cannot decode speech, replay or tampered frames never reach the decoder, membership change terminates or rekeys exactly and callback, buffer and memory bounds remain green. UI claims E2EE only for sessions that completed reviewed key setup.

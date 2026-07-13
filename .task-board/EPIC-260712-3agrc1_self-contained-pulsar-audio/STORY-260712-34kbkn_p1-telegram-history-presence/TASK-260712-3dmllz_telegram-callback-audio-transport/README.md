# Implement secure Telegram callbacks and clip attachment transport

## Description
Parse Phase 1 Telegram updates into typed, authorized events without putting media or queue business logic in the transport.

## Scope
Support callback queries, inline keyboards, voice, audio and document updates. Treat filenames, MIME, duration and size metadata as untrusted hints for the common ingest probe. Encode only opaque integrity-protected callback references within the Telegram Bot API limit; validate expiry and shape, bind callback actors through ActorContext, answer callback queries promptly, and make duplicate delivery idempotent. Remove or disable buttons after terminal or too-late outcomes. Return human errors for unsupported, corrupt, over-limit and phase-two track paths without exposing raw database IDs or secrets.

## Acceptance Criteria
Tests cover voice, audio and document parsing, forged, expired, cross-user, cross-orbit and replayed callbacks, prompt callback acknowledgement and terminal keyboard edits. The adapter emits typed events to common ingest and transmission services, never trusts Telegram metadata as media proof, never contains bespoke queue logic and never exposes identifiers or credentials in visible or callback text.

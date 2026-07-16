# Implement Windows protected range playback and ciphertext cache

## Description
Fetch verify decrypt and play clips and tracks incrementally while storing only ciphertext in the durable cache.

## Scope
Authenticate manifest and sender or group context, authorize epoch and content-key envelope, fetch chunks with canonical ranges, verify AEAD before decode and feed the existing bounded player or overlay graph. Cache ciphertext and public metadata only; hold plaintext in bounded non-pageable memory where practical, never log it and purge revoked, deleted, expired or corrupt entries. Preserve seek generations, start, skew and memory gates, receipts, DND, local ceiling and typed missing-grant or fork failures.

## Acceptance Criteria
Signed Windows plays all valid macOS and Windows protected fixtures without full download, meets Phase 2 hard gates and never decodes unauthenticated bytes. Tamper, replay, wrong target, revoked grant, cache substitution, delete and restart fail safely; disk and crash scans contain no protected plaintext or key and mixed-version behavior never downgrades.

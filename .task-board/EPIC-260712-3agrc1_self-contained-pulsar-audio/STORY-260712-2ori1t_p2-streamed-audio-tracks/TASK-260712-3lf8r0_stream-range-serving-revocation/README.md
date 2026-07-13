# Serve private authenticated stream variants by range

## Description
Implement the ADR range contract with immutable-target authorization, conditional integrity and canonical revocation semantics.

## Scope
Expose opaque variant selection with HEAD where approved, Content-Length, Accept-Ranges, ETag, Cache-Control for authenticated private content, 200 or 206, Content-Range, If-Range and 416 exactly as frozen. Authenticate every request against the accepted transmission target snapshot, variant and media state; cap pathological, overlapping or tiny-range abuse and meter actual egress. Return uniform nonexistence for foreign, deleted or disabled access. A report alone must not enable global denial-of-service: apply only the frozen reporter-local hide or quarantine policy, while moderator delete or disable and sender delete revoke new ranges. Preserve Phase 1 clip endpoints.

## Acceptance Criteria
Nodes resume and seek without full redownload and receive stable bytes for one variant version. Non-targets cannot infer media or variant IDs, intermediaries cannot leak authenticated content and range abuse is bounded. Conditional requests detect replacement, delete or disable blocks future range immediately, plain report follows the reviewed anti-abuse policy, and 200, 206, HEAD, If-Range, 416, quota and revocation tests pass.

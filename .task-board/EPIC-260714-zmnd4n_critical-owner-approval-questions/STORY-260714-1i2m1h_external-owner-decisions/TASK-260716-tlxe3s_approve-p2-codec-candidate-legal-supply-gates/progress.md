## Status
backlog

## Assigned To
ivan-oparin

## Created
2026-07-16T12:22:34Z

## Last Update
2026-07-19T10:39:53Z

## Blocked By
- (none)

## Blocks
- (none)

## Checklist
- [ ] Record qualified counsel identity, date and AAC/LGPL disposition for exact product, hosted conversion and markets
- [ ] Record independent reviewer identity and confirm no codec or package implementation overlap
- [ ] Freeze exact candidate commit, Windows/macOS binary hashes, runtime SBOM and source/build receipts
- [ ] Close and independently re-review P2-CODEC-SUPPLY-001 through P2-CODEC-SUPPLY-006
- [ ] Prove production signatures, macOS notarization, no runtime download and zero known unpatched vulnerabilities
- [ ] Record Ivan Oparin proceed or hold without activating production playback

## Notes
Created from TASK-260712-2g3fkt fail-closed outcome under the owner instruction to accumulate critical approvals separately and continue reversible engineering. Proposed default is bundled FFmpeg 8.1.2 minimal shared, package-local, no CLI, no runtime download and dark-only. This is a proposal, not legal advice or production authorization. Engineering report PR #170 merge affa66ab830696e38e923f217a3b43dd5e95b581; exact reviewed base 496c07272e4a5406b44be8709fa84c9b5932cdda; hosted run 29497274813 passed 4/4.
2026-07-19 owner decision: Ivan Oparin approved bundled FFmpeg 8.1.2 minimal shared, package-local, no CLI, no runtime download and dark-only as the default candidate for qualification. This does not waive counsel, supply-chain, binary-hash, signing/notarization, hostile-input or independent-review gates and does not authorize production activation.

## Precondition Resources
- [p2-independent-codec-supply-review.md](file://TASK-260716-tlxe3s/p2-independent-codec-supply-review.md) — Fail-closed Phase 2 codec supply review and six High findings

## Outcome Resources
(none)

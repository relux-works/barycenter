# Root-audit the final evidence and release decision

## Description
The primary agent performs the last non-delegable audit after the packet and all reviews are complete.

## Scope
Read the final packet, every reviewer report and disposition, C1-C7 raw results, beta days and resets, build and flag hashes, migration and rollback artifacts, privacy and Store copy and git diff created after the first root review. Re-run deterministic checks and spot-reproduce critical paths, verify no unreviewed code or dependency entered the tested build and map every spec requirement to evidence or an explicit hold. Record separate promote or hold decisions and rollback owners for live_ptt, e2ee_media, soundboard_cues and automation.

## Acceptance Criteria
The root-authored audit contains direct line-by-line review of every later diff, reproducible checks and an evidence map with no unexplained gap. No open critical or high finding, invalid beta day, hash mismatch, placeholder input or overstated claim remains. Release is authorized only for capabilities explicitly marked promote; all others remain disabled with exact blockers.

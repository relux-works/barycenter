# Execute final C4-C6 E2EE privacy and report acceptance

## Description
Run C4-C6 after cryptographic code review and privacy review close on the exact root-reviewed build.

## Scope
Exercise clips, tracks, saved cues and live PTT across all platform pairings; concurrent join, leave and revoke; removed and compromised device, new device no-history, explicit grant, recovery success and irreversible loss, coordinator storage and traffic capture, replay, rollback or clone, silent downgrade, truthful metadata disclosure, OS key storage and voluntary report evidence with expiry and moderation. Use the reviewed threat assumptions and do not infer malicious-coordinator protection if excluded.

## Acceptance Criteria
C4-C6 and non-functional key gates pass reproducibly for every claimed path. Removed members cannot decrypt new media, new devices cannot decrypt ungranted history, coordinator artifacts cannot reproduce content and report plaintext exists only after consent. Unsupported paths, metadata and revoke or deletion limits are visible, with no critical or high external or privacy finding open.

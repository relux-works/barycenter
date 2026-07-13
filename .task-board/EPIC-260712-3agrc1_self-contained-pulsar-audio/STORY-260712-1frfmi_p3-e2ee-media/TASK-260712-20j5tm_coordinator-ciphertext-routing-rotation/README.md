# Implement client-driven Air epoch coordination

## Description
Serialize authorized client-produced membership proposals and commits without letting the coordinator own group secrets.

## Scope
Bind device public identities to authorized actors and Air membership, validate and order signed client proposals or commits, notify exact members, track acknowledgements and fork or stale state and require client rotation on join, leave, device revoke or actor disable. Seal transmissions to a reviewed epoch and target snapshot, reject commits from removed or unauthorized devices and provide recovery from delivery loss without reconstructing secrets. The coordinator routes opaque key packages and may enforce membership, but cannot create, unwrap, escrow or log group and content keys.

## Acceptance Criteria
Concurrent membership changes converge or fail closed under the reviewed state machine; removed devices receive no new commit packages or content-key envelopes. Restart, replay, stale client, fork and malicious proposal tests remain bounded and auditable. Storage or traffic capture plus all coordinator state is insufficient to derive any group or content secret.

# Complete external cryptographic implementation review

## Description
A qualified independent reviewer audits the implemented E2EE system and closes critical and high findings.

## Scope
Provide the frozen threat model and design-review hashes, source and dependency diff, protocol and state machines, known-answer, fuzz and attack tests, signed builds, SBOM, metadata disclosure, recovery and report flows and coordinator capture artifacts. Reviewer inspects client key ownership, library use, nonce and key separation, device authentication, group commits, chunk and live framing, secure storage, downgrade and claims. Track every finding, fix and retest; any relevant code or protocol change resets the review.

## Acceptance Criteria
A dated external report names reviewer independence, exact commit and artifacts, findings and retests. No critical or high finding is open; residual risks have owners and claim constraints. This task cannot be self-certified by an implementation agent and e2ee_media remains off until it passes.

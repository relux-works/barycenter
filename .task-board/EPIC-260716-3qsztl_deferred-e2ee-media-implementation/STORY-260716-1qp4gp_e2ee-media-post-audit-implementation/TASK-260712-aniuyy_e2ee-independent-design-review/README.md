# Pass independent cryptographic design review before implementation

## Description
Give the threat model ADR protocol state machines and test vectors to a qualified reviewer and close design findings.

## Scope
Review device authentication, coordinator trust and equivocation, group lifecycle, forward secrecy claims, concurrent membership, nonce and key separation, chunk and live framing, recovery and history grants, report evidence, metadata disclosure, downgrade, deletion limits and library supply chain. Track findings with severity, owner, fix, retest and reviewer disposition. Any protocol-affecting change invalidates the reviewed hash and requires delta review.

## Acceptance Criteria
An independent reviewer can reproduce vectors and signs off the exact document hashes with no open critical or high design finding. Medium or residual risks have explicit product language and owners. Until this task passes, schema, crypto state and routing implementation remain blocked and e2ee_media stays off.

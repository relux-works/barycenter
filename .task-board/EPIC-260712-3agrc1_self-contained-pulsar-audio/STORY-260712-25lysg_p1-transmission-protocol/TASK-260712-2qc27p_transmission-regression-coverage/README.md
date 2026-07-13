# Prove transmission compatibility, ACL and scheduler semantics

## Description
Close the protocol story with adversarial, migration, timing and mixed-version tests across coordinator and all three codecs.

## Scope
Add tests for immutable audience ACL, copied media IDs, trusted acceptance ordering, include-origin defaults, kind-delivery validation, the 60-second overlay rule, one-controller-per-playback-domain serialization, exact barrier formula and timeout, partial readiness, every missed reason, stale play rejection, DND and block, cancel, delete, leave, apart and no-ready cleanup. Prove the whole-transmission mixed-fleet downgrade and legacy after_current behavior. Add migration and rollback fixtures and keep Go, Windows and Swift golden suites green.

## Acceptance Criteria
Automated evidence maps every story AC to a test or an explicitly named real-hardware proof. Tests catch cross-orbit access, caller-controlled ordering, split-mode downgrade, two-source overlap in one approach, late autoplay and orphan timers. Database upgrade and previous-version rollback are additive, and all protocol mirror and compatibility suites pass.

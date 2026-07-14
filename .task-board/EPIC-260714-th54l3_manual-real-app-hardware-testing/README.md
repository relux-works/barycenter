# Manual real-app hardware testing

## Description
Deferred manual acceptance program for immutable signed Pulsar builds. It contains only hands-on real-app, physical-device, production-shaped rollout and multi-day beta verification; coding, unit tests, deterministic integration tests and implementation reviews remain in EPIC-260712-3agrc1.

## Scope
Execute the extracted P1, P2 and P3 manual scenarios in their original strict order on declared Windows and macOS hardware, real audio routes and approved multi-home environments. Preserve build hashes and sanitized raw evidence. A failure creates or reopens engineering work in the development epic; no code change is implemented inside this epic.

## Acceptance Criteria
Every child task has a named operator, admissible environment, immutable build identity, exact steps, sanitized raw artifacts and an honest pass or fail. No CI, mock, simulator or inferred result substitutes for a required physical or real-environment observation. The epic completes only after all manual tasks pass or have explicit product-approved holds; it never blocks best-effort engineering execution.

# BUG-260728-28mp7v: windows-onboarding-stays-unpaired-until-relaunch

## Description
On Windows, successful in-process Create a new Barycenter writes valid DPAPI credentials and creates a new coordinator Barycenter, but the already-running Pulsar shell remains in the unpaired tray loop and does not authenticate until the app is relaunched.

## Scope
Windows first-device onboarding state transition only: after credentials are persisted, reload the authenticated client state and replace the unpaired shell/session in-process. Preserve DPAPI boundaries and the distinction between new-Barycenter onboarding and device pairing. Air protocol, coordinator lifecycle and macOS behavior are outside scope.

## Acceptance Criteria
After Create a new Barycenter succeeds on Windows, the same running process loads the new credentials, establishes the authenticated node session, exits unpaired UI state and exposes the paired Air-capable shell without requiring restart. Coordinator nodes_connected reflects the Windows node, no duplicate actor or slot is created, and regression coverage proves the transition while preserving DPAPI secrecy and recovery semantics.

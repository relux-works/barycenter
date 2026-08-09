# BUG-260728-2ybeny: windows-duplicate-instances-reconnect-storm-blocks-air-join

## Description
Windows permits multiple standalone Pulsar processes to run concurrently with the same Barycenter identity. Three instances contended for orbit 10, producing repeated WebSocket close 1006 reconnects and preventing a GUI Air Review/Confirm from committing membership.

## Scope
Windows process lifecycle and single-instance enforcement for the standalone Pulsar application. Ensure one interactive process owns a Barycenter identity and subsequent launches activate the existing window instead of opening competing coordinator sessions. Preserve command-line test tooling and intentional non-GUI helpers. Air protocol and coordinator membership semantics are outside scope.

## Acceptance Criteria
Launching Pulsar repeatedly on Windows leaves exactly one GUI application instance for the user profile and Barycenter identity, activates the existing window, and maintains one stable authenticated WebSocket session without reconnect storms. Air Join Review/Confirm can commit while the app is online. Regression coverage proves duplicate launch handling, stale-process recovery and no interference with approved helper/test binaries.

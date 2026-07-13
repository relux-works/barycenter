# Package and document a reproducible signed MSIX probe

## Description
Make the smallest signed MSIX probe reproducible, including manifest declarations, signing route and run instructions for hardware execution.

## Scope
Update the MSIX build and documentation so the probe can be produced as the smallest signed package under the current Partner Center identity and appContainer or packagedClassicApp settings. Capture the exact manifest declarations, signing route, install commands, and artifact layout needed for Windows 10 and 11 execution.

## Acceptance Criteria
Another engineer can build the probe, obtain a signed MSIX through the documented path, install it, and reproduce the probe scenarios without guessing any manifest or signing step. The package keeps the current sandbox posture and requests only the declarations justified by the task one decision.

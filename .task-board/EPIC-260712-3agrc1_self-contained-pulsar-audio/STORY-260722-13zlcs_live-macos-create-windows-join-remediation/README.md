# STORY-260722-13zlcs: live-macos-create-windows-join-remediation

## Description
Close the production gap that prevents an owner from creating a Barycenter in the macOS app and joining the currently installed Windows app without terminal commands.

## Scope
Add a first-run macOS path into the main Create/Join shell; expose control-authorized one-time device-invite issuance with explicit copy/hide/expiry and secret redaction; safely enable the existing self-service onboarding routes on the live coordinator after backup, migration and rollback preflight; build, sign and install a self-contained local macOS app candidate; verify ordinary GUI launch and publish one concise Mac-create to Windows-join owner handoff. Preserve existing Telegram pairing as optional fallback. Exclude subjective audio, microphone, VoiceOver and hardware acceptance, which remains in EPIC-260714-th54l3.

## Acceptance Criteria
The live coordinator registers Create, device-invite issue and Join routes without regressing health or legacy nodes. A fresh macOS app launched normally can create a Barycenter, requires explicit recovery export before activation, and can issue a redacted one-time device invite from the UI. The installed Windows app can consume that code through Join. No terminal is needed for the owner flow, secrets never enter logs or durable UI state, automated tests/builds/CI and independent reviews pass, and the local Mac app plus exact hashes are installed and handed off without claiming the final manual result.

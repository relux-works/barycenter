# Implement Windows automation administration

## Description
Add schedule principal status history and emergency disable controls to the signed Windows UI.

## Scope
Provide schedule create, edit, enable, disable and delete with IANA timezone, DST and quiet-hour explanation; scoped-principal issue with one-time secret display, metadata list and revoke; feature status, history attribution, pending cancel and orbit emergency disable. Require explicit confirmation for secret issuance and destructive actions, support copy without persistence, redact screenshots and accessibility values and keep manual soundboard usable when automation is off.

## Acceptance Criteria
Authorized Windows roles can understand the next fire time and skipped or repeated behavior, manage schedules and principals and stop automation quickly. Secrets are never redisplayed or persisted, unauthorized roles cannot infer configuration, accessibility and packaged tests pass and disabling automation leaves manual cue and other audio paths honest.

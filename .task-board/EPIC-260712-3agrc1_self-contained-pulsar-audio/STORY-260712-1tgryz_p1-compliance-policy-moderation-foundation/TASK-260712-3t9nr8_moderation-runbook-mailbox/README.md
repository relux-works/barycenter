# Operationalize the moderation mailbox and runbook

## Description
Turn Store-required report handling and Microsoft removal requests into a least-privilege, auditable human process with no invented recovery promise.

## Scope
Create or verify the approved mailbox and accountable rotation; document intake, triage, prohibited-content and urgent escalation, evidence-access minimization, reporter-safe communication, Microsoft request verification, no_action, delete and disable procedures, audit export, retention and backup behavior, operator credential issue and revoke, and incident escalation. Map every step to a real control-plane command. State which actions are reversible and that physically deleted audio is not promised recoverable; provide correction procedures for mistaken reversible disables without restoring forbidden content.

## Acceptance Criteria
A named operator can handle a normal report and a verified Microsoft removal or disable request end to end using only documented least-privilege tools. Every evidence access and action is audited, sensitive audio is not emailed or put in normal logs, reporter updates reveal no third-party data, unavailable owners have escalation coverage, and no runbook step depends on a placeholder mailbox or impossible delete rollback.

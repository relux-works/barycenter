# Provision Barycenter inbound mail routing

## Description
DNS inspection on 2026-07-14 found no MX records for barycenter.live, so support@barycenter.live, moderator@barycenter.live and moderation-urgent@barycenter.live cannot yet be claimed as deliverable.

## Scope
Owner/provider action: select and verify a private destination address, enable Cloudflare Email Routing or an equivalent provider, publish required MX/SPF records, create all three aliases, test delivery without sensitive user audio, and record primary/backup acknowledgment.

## Acceptance Criteria
Public DNS exposes the provider-required MX records; each approved alias accepts a synthetic non-sensitive delivery test to a verified Ivan Oparin destination; sender authentication guidance is recorded; the destination is not committed to the repository; and the moderation runbook records the evidence timestamp and accountable rotation.

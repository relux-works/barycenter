## Status
backlog

## Assigned To
ivan-oparin

## Created
2026-07-14T16:26:31Z

## Last Update
2026-07-19T10:39:53Z

## Blocked By
- (none)

## Blocks
- (none)

## Checklist
(empty)

## Notes
Critical owner input needed later: provide or select the private verified destination mailbox. Proposed default: Cloudflare Email Routing, with all three public aliases initially forwarding to one Ivan Oparin-controlled destination and Ivan acting as primary, backup and escalation owner per approved defaults. This is an external provider-side change and is not performed or invented by repository work. Discovery evidence: dig barycenter.live MX returned no answer on 2026-07-14; A records resolve through Cloudflare.
2026-07-19 owner decision: Ivan Oparin approved the proposed Cloudflare Email Routing default for support@barycenter.live, moderator@barycenter.live and moderation-urgent@barycenter.live, initially forwarding to one Ivan-controlled private verified destination. This approves the provider/routing approach only; the private destination remains intentionally absent from git, DNS/provider mutation and synthetic delivery are still not executed, and completion evidence remains open.

## Precondition Resources
(none)

## Outcome Resources
(none)

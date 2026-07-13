# Implement transmission API and immutable audience resolution

## Description
Accept ready media, validate the delivery request, seal its audience and expose safe status and cancellation views.

## Scope
Implement POST, GET status and POST cancel endpoints on the actor and control-token model. Resolve this_pulsar, own_barycenter, current_air as the current pairwise approach in phase one, and explicit audiences into immutable node targets at acceptance; apply microphone-excludes-origin and file-includes-origin defaults with the explicit play-here override; enforce media ownership, readiness, expiry, clip-kind and delivery compatibility and the 60-second overlay limit. Preserve a coordinator-issued intake timestamp for Telegram and app ordering, never a caller-supplied timestamp. If overlay capability is missing on any mandatory target, record one visible whole-transmission effective_delivery=after_current. If exact interrupt capability is missing, return requires_confirmation with ordered overlay and after_current alternatives; accept a fallback only through an explicit confirmed request.

## Acceptance Criteria
Authorized create, status and cancel calls seal exact targets and expose only sender or audience-safe data. Invalid kind-delivery combinations and over-60-second overlays are rejected with actionable alternatives. Origin defaults match the media source. A client cannot manipulate FIFO time. Mixed fleets never split one transmission across modes, and interrupt never changes semantics without explicit user confirmation. Cancel affects only contract-approved nonterminal states.

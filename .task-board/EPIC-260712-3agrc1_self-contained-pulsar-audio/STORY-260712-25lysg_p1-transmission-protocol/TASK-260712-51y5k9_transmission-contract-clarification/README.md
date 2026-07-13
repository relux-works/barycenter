# Freeze transmission, DND and downgrade contracts

## Description
Close every protocol and policy gap that could otherwise produce incompatible clients, unsafe ordering or silent mixed-version behavior.

## Scope
Define exact HTTP request, response and status-code shapes; receipt and aggregate-state enums; WebSocket payloads for prepare, ready, scheduled play, started, ended, failed, cancelled, DND and presence; DND modes, timestamp semantics and ownership; audience resolution and include-origin defaults for microphone versus file; clip-kind and delivery compatibility; the 60-second overlay limit; prepare deadline and scheduling formula; stale-play rejection; cancellation and expiry rules; and the sender-delete policy for queued, prepared, scheduled and already-playing media, including a click-free stop or completion decision and exact receipts. For overlay, any mandatory target lacking capability yields one visible effective_delivery=after_current. For interrupt, unsupported exact pause or resume yields requires_confirmation with overlay then after_current suggestions and no transmission is scheduled until explicit fallback confirmation. Define accepted_at as coordinator-issued at initial intake and never caller-controlled.

## Acceptance Criteria
One reviewed contract note gives all three codecs, APIs, scheduler, history and bot surfaces the same field names, enums, defaults and error behavior. It states the three-second deadline, exact schedule formula, whole-transmission capability behavior, interrupt confirmation, active-delete behavior, no remote DND bypass and authorization boundaries. No implementation task must invent timing, ordering, origin, cancellation, delete, capability or downgrade semantics.

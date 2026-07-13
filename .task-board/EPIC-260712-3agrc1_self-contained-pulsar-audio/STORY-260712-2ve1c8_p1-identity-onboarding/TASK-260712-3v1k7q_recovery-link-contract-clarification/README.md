# Clarify recovery and Telegram link contracts

## Description
Close the API and policy gaps the source-of-truth leaves implicit so implementation tasks can proceed without inventing behavior.

## Scope
Define the recovery endpoint path, request and response shape, uniform errors, rate limits, secret rotation and control-token revocation; define that the recovery secret is displayed once, is not silently persisted by the app, and recovery reissues only the control credential while preserving the current node credential if present. Define Telegram link desired-role input, default role policy, code entropy and expiry, and same-orbit, foreign-orbit, already-linked and concurrent-consume conflict behavior. Record the approved contract for every downstream task.

## Acceptance Criteria
The reviewed note freezes recovery request, response, errors, brute-force controls, one-time display and post-recovery credential state, plus Telegram role selection, single-use consume and conflict behavior. Loss of the sole installation and unsaved recovery secret is stated as unrecoverable. Downstream API and clients need no product guess and no secret enters logs, URLs or analytics.

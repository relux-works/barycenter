# Write Pulsar Audio product brief

## Description
Produce a source-backed product document that explains what Pulsar Audio does, who it is for, which alternatives it competes with, which capabilities exist or are committed, and which future bets are technically and commercially plausible.

## Scope
Read the product and engineering source docs and current epic. Research primary competitor documentation. For Spotify, Apple Music and Yandex Music assess both provider-supported integration and relevant open-source GitHub projects, including maintenance, license, authentication/playback method and production risks. Describe a provider-neutral cross-subscription synchronization model without claiming rights or reliability that have not been verified.

## Acceptance Criteria
docs/product-pulsar-audio.md is written in Russian and contains a one-line category, emotional promise, jobs and segments, non-targets, product principles, status-labelled capability map, competitor comparison, differentiated positioning, future roadmap, cross-provider architecture hypothesis, provider-by-provider feasibility, product metrics, guardrails and cited sources. .research contains the search log. Every external product claim has a primary URL or is explicitly marked unverified. task-board validate and document link checks pass. Changes land in a separate commit from the E2EE board split.

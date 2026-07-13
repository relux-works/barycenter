# Build localized shared delivery labels

## Description
Centralize stable RU and EN sender, origin, audience, requested and effective delivery, confirmation and receipt wording for every app and bot surface.

## Scope
Resolve actor display names, origin barycenters, current approach peers, This Pulsar, own Barycenter and current approach audiences, include-origin copy, delivery labels, downgrade and confirmation states, and exact receipt reasons. Use locale keys rather than loop-specific strings. Provide privacy-safe fallbacks for deleted or missing actor, slot and approach metadata and never expose database, Telegram, node or composite orbit-slot identifiers.

## Acceptance Criteria
Windows, macOS and Telegram can render the same semantic labels in RU and EN, including requested versus effective delivery and interrupt confirmation. Missing or deleted metadata produces stable localized human text. Golden fixtures prove no raw identifiers and no transport-specific wording leaks from the shared model.

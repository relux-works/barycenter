# Implement Windows saved and active Air management UI

## Description
Render the complete Air lifecycle in the packaged Windows shell over opaque shared control-plane models.

## Scope
Show saved Airs, exactly one active Air, pending invite or joining-primary confirmation, members, capacity and effective policies. Implement create, invite, join, confirm or decline, activate or switch, leave and permitted dissolve or policy changes; require confirmation for disruptive switch, leave or dissolve and show track or overlay effects honestly. Reuse localized names, secure invite handling and canonical errors; no raw IDs, public discovery or target or inbox assumptions.

## Acceptance Criteria
Windows can manage multiple saved Airs and one active Air with keyboard, screen-reader and high-DPI access. Role, stale invite, capacity, concurrent change and offline errors are honest. Disruptive actions cannot occur accidentally, secrets are redacted, two-party aliases remain understandable and all commands use the common Air API.

# Air creation failure and onboarding architecture

Date: 2026-07-27  
Board item: `BUG-260727-1hjfxi`

## Outcome

The reported Air creation failure is not a concurrent edit by another client.
Production exposes the Air UI while Air rooms are still in shadow mode:

```json
{
  "phase2": {
    "air_rooms_enabled": false,
    "air_authority_state": "airs_shadow"
  }
}
```

In this mode every Air read or mutation fails the coordinator's
`requireAirsAuthoritativeTx` gate with `ErrAirRevision`. The macOS client maps
that internal rollout guard to the user-facing text "The Air changed elsewhere.
Refresh and try again." No Air is committed by the failed create request.

There are three separate defects:

1. The app exposes Air management without checking coordinator capabilities.
2. The coordinator overloads `revision_conflict` for both real stale revisions
   and the Air rollout authority gate.
3. The identity bootstrap screen calls `createOrbit` but labels the operation
   "Create an air", creating two different user-visible objects with the same
   name.

The macOS mutation state also makes recovery brittle. A failure calls
`refresh(force: true)` before `mutationInFlight` is cleared, so the refresh is
discarded by the guard. Mutation and projection refresh failures are collapsed
into one error even when a mutation may already have committed.

## Product model

The implementation can keep its current security boundaries, but the product
must give each object one responsibility:

| Object | Meaning | User visibility |
| --- | --- | --- |
| Pulsar installation | This Mac or Windows installation and its device credential | Devices/settings |
| Barycenter | The user's persistent private home: devices, members, settings and personal state | First-run setup and settings |
| Air | A shared room containing two or more Barycenters | Air list, room picker and invitations |

An Air invite must never attach a bare installation. An installation first
belongs to exactly one Barycenter; its Barycenter then joins an Air.

## Target user experience

### First launch on the first device

Show one setup page:

- **Start a new Barycenter** — recommended for the first device.
- **Connect this device** — accepts an invite from an existing Barycenter.
- **Try audio locally** — remains available without identity.

Do not mention Air on this page. A suggested Barycenter name can be derived
from the local account or device and edited before submission.

After the server creates the Barycenter and the credentials are stored in the
OS credential store, activate the app immediately and navigate to Home.
Recovery is a Barycenter safety task, not an Air creation step.

Recommended recovery behavior:

- Show a persistent, non-blocking "Protect your Barycenter" card in Home and
  Settings until recovery is configured.
- Let the primary export a recovery file or copy/print a recovery phrase.
- Make the action resumable. If the original one-time payload is no longer
  available, an authenticated primary rotates the recovery secret and exports
  the replacement.
- Explain the risk before dismissal, but do not leave the app in a misleading
  half-created state.

This keeps the server hash-only recovery model and OS-protected device
credentials while removing the file-save dialog from the critical path.

### Connect a second device to the same Barycenter

On the existing Mac:

1. Open **Settings → Devices → Add device**.
2. Receive a short-lived code, QR code and deep link.

On Windows:

1. Choose **Connect this device** on first launch.
2. Paste the code or open the deep link.
3. Review: "Connect this Windows PC to Barycenter ‘Home’?"
4. Confirm and enter Home.

This is a device invite. It is not an Air invite.

### Create and invite to an Air

The Air page is available only when the authenticated feature contract says
Air rooms are enabled.

1. Press **New Air**.
2. Enter a name and create.
3. The server creates the parked Air and an initial short-lived Air invite as
   one idempotent workflow.
4. The success page immediately offers Copy link, Copy code and Show QR.

The user does not need to understand "saved membership" versus an "active
pointer". The Air list shows simple states:

- **Active now**
- **Ready**
- **Waiting for others**
- **Invitation pending**

Opening a ready Air activates it. If another Air is active, the app asks one
plain question: "Switch from ‘Family’ to ‘Friends’?"

### Join an Air

Use one general **Enter invite code** entry point. The code preview identifies
its type before any mutation:

- "Connect this device to Barycenter ‘Home’", or
- "Join Air ‘Friends’ with Barycenter ‘Home’".

For an Air invite, the joining Barycenter's primary confirms membership. The
UI explains this as approval by the owner of the joining Barycenter, without
exposing internal role names such as `primary`.

## Capability and error contract

The app must fetch an authenticated capabilities document during bootstrap.
Air navigation and actions are derived from a typed state:

```text
unavailable | disabled | enabled | temporarilyUnavailable
```

`airs_shadow` maps to `disabled`, not to a data conflict. The production app
must not render a create form for disabled features.

The coordinator should return a dedicated code such as
`air_rooms_not_enabled` for the authority gate. `revision_conflict` remains
reserved for mutations that actually supplied a stale Air, membership, policy
or active-pointer revision.

Client mutation state must distinguish command execution from projection sync:

```text
idle
  -> submitting(commandID)
  -> committed(receipt)
  -> syncing
  -> succeeded
```

Retryable transport failures reuse the same idempotency key until the outcome
is resolved. A committed mutation followed by a refresh failure is shown as
"Created; syncing" rather than "Create failed". Conflict handling finishes the
mutation state, reloads, and only then offers a retry against current data.

## Implementation slices

1. **Production safety gate**
   - Hide or disable the Air surface unless `air_rooms_enabled=true`.
   - Add a dedicated rollout-disabled error code.
   - Do not cut production to `airs_authoritative` until the existing migration
     and rollback runbook is executed and verified.

2. **Terminology and navigation**
   - Rename identity actions to "Start a new Barycenter" and "Connect this
     device".
   - Remove Create/Join identity items from the normal sidebar after identity
     is active; move device management to Settings.
   - Reserve "Air" exclusively for multi-Barycenter rooms.

3. **Reliable command state**
   - Model mutation receipt and projection sync separately.
   - Preserve idempotency keys across retries.
   - Queue refresh after mutation cleanup instead of dropping it while busy.
   - Add composition tests for rollout-disabled, commit-then-refresh-fails,
     revision conflict and retry-after-transport-loss.

4. **Guided Air flow**
   - Create Air plus initial invitation as one workflow.
   - Add typed invite preview and a unified code/deep-link entry point.
   - Replace technical saved/active copy with room states and a switch
     confirmation.

5. **Resumable recovery**
   - Activate locally stored credentials immediately.
   - Track recovery readiness separately from identity readiness.
   - Support authenticated recovery-secret rotation if the one-time export
     payload is lost before backup.

## Acceptance scenarios

1. With production in `airs_shadow`, Air creation controls are absent and the
   app explains that Airs are not enabled; no mutation is sent.
2. On a clean Mac, creating a Barycenter activates Home without mentioning Air.
3. A Windows installation joins the Mac's Barycenter using a device invite and
   sees the same Barycenter identity.
4. Creating an enabled Air returns an immediately shareable invite and leaves
   a visible Air card even if the follow-up projection refresh is offline.
5. An Air invite preview cannot be confused with a device invite.
6. A genuine stale revision reloads current state and allows one deliberate
   retry without duplicating the mutation.
7. Recovery can be completed after onboarding or rotated later by an
   authenticated primary.

## Product decision

The recommended architecture makes recovery setup non-blocking and resumable.
This changes the current invariant that a newly created Barycenter cannot
activate before a recovery file is saved. It improves completion and avoids the
current half-created state, while accepting a clearly disclosed window in which
loss of the only device could still lose access. Product approval is required
before implementation.

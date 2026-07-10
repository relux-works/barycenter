# Decision log

Short records of non-obvious engineering decisions. Newest first.

## 2026-07-10 — Spotify pause is a PERSONAL pause

**Problem.** Pausing a Pulsar through the Spotify app was invisible to the
system: the node only faded its local gain, `playback` stayed playing and the
coordinator was never told. Any legitimate mechanism — a ready-timeout retry,
a resume_at already in flight, the advance to the next element, a liveness
catch-up — would then lawfully restart playback over the user's pause within
seconds (the ghost-resume report, 2026-07-10).

**Decision.** The pause becomes a first-class personal event. The node
detects a user pause (a `paused` daemon event while its own playback state is
still playing — coordinator-driven pauses flip the state first), cancels any
in-flight resume timer and sends `user_pause`. The coordinator detaches just
that home: excluded from the current element's barriers (with the H1/H2-class
re-checks) and from sealing of subsequent elements, while the air keeps
playing for everyone else. Play in Spotify sends `user_resume` and the home
catches back up at the live position through the living-air join. The mental
model: **bot `/pause` pauses the air for everyone; Spotify pause pauses only
you.** The last active home pausing degrades to the ordinary global pause —
the last listener stopping IS the air stopping.

**Bounds.** Runtime-only state: a coordinator restart or the node going
offline clears the personal pause (the node loses its local flag anyway), and
catch-up resumes the home — the safer default when state is lost. Additive
protocol (`user_pause`/`user_resume` + goldens in all three codecs); old
nodes simply never send them and keep the previous behavior.

## 2026-07-10 — Spotify handoff keeps its leader audible; voice FIFO is non-preemptible

**Problem.** Live two-home logs showed that the first Spotify-first
implementation paused and reloaded the Pulsar where the user had already
started the track. It also mistook stale coordinator-owned daemon loads for
phone choices, so two homes could resurrect different old album contexts
within milliseconds of each other. Voice inserts lost the same race.

**Decision.** A phone selection is a leader handoff. The initiating Pulsar is
relabelled without stopping; followers load paused, seek to the leader's future
audible position and join at a scheduled time. Events with
`play_origin=go-librespot` are internal and never become `external_playback`.
A follower timeout/restart degrades only that home; the healthy air continues.

Voice messages are ordered by Telegram acceptance time, not ffmpeg completion.
Once a voice block starts, a Spotify selection is queued after it rather than
cutting it off. Bot queue labels always include the sender.

The current personal-voice target is either one Pulsar or everyone. In an air
with more than two recipient homes, “everyone except the sender” is not yet an
expressible target set and therefore falls back to broadcast. This must become
an explicit target list before L2 links three or more personal barycenters.

## 2026-07-09 — Spotify on a Pulsar is the together-mode control surface

**Problem.** Starting a track on a Pulsar during shared mode was detected as
external playback, but the default `user` takeover policy switched the whole
session to solo. Users still had to copy a Spotify link into Telegram to start
the synchronized air.

**Decision.** A track selected on any Pulsar while the session is shared is a
leader event, not a reason to leave shared mode. The node reports the Spotify
URI plus its audible position; Barycenter creates a shared element and runs the
existing load/ready/resume barrier for all connected homes. An idle shared air
always adopts the selection. In a busy air, `takeover_policy=user` adopts the
new selection and `coordinator` protects the current broadcast. Explicit
`/solo` remains the only way this interaction exits together mode.

**Compatibility.** `external_playback.position_ms` is optional. Old nodes keep
working and start adoption at position zero; the golden protocol contract and
all three codecs carry the new field.

## 2026-07-07 — Pairing credentials in the Data Protection keychain (F2)

**Problem.** After a Sparkle update the app silently fell back to the
onboarding window: the pre-F2 login-keychain item's access control was bound
to the app on disk, and the freshly-updated binary was denied without a
prompt. A user who had paired lost their pairing on every update.

**Decision.** Store credentials in the **Data Protection keychain**
(`kSecUseDataProtectionKeychain = true`), `kSecAttrAccessibleAfterFirstUnlock`,
in the app's **default** access group — no explicit `kSecAttrAccessGroup`,
therefore no `keychain-access-groups` entitlement.

**Why no explicit group.** DP items are keyed by the app's code signature,
which is stable across updates of the same identity (Developer ID). An
explicit team access group would require an entitlement that self-signed dev
builds cannot carry; the default group needs none and works on every build.
Access still survives updates because the Developer ID signature is stable.

**Migration.** On first load after upgrade, `CredentialsStore.load` reads the
old login-keychain item (and, older still, the JSON file), re-saves into the
DP keychain, and deletes the source. One-way, idempotent.

**Related.** A dev-era `~/duet/node.yml` pointing at `127.0.0.1` used to
hijack a paired app onto a dead local coordinator; the app now retires it to
`.retired` on launch (only the default path, only when it points local).

## 2026-07-08 — Windows code signing: EV cert, not Azure Trusted Signing (F6)

**Context.** The direct-download Windows channel (.exe/.msix that runs without
a SmartScreen warning) needs a code-signing certificate. Azure Trusted /
Artifact Signing was the cheapest option ($10/mo) but is **available only to
organizations in the USA, Canada, EU & UK**. Relux Works, LLC is Armenian
(C=AM) — ineligible; individual signing is US/Canada only too.

**Decision.** Buy an **EV code-signing certificate with cloud signing**
(SSL.com eSigner, or Certum SimplySign as the budget alternative). EV gives
instant SmartScreen trust from the first download (OV would show a warning
until reputation accrues — unacceptable for a first-time user). Cloud signing
is mandatory post-2023 (EV keys must live on a FIPS HSM; no token in CI).

**Wiring.** CI scaffold is in release.yml (commented sslcom/esigner-codesign
steps for the inner exe and the MSIX). Activation = set the eSigner secrets,
set AppxManifest Publisher to the cert's exact subject DN (MSIX requires
Publisher == signer subject), uncomment the two steps.

**Microsoft Store (Partner Center)** stays the preferred long-term channel
(Microsoft signs, no cert, broader country support) — pursued separately.
EV direct-download is the channel that does not depend on Store review timing.

## 2026-07-08 (update) — Windows: Microsoft Store, not a paid cert

Relux Works, LLC passed Microsoft Store developer verification (email +
business + employment, Armenian entity accepted). The Store signs submitted
MSIX packages itself — no code-signing certificate needed, best UX for
end users (one-click install, no SmartScreen, Store auto-updates). This
SUPERSEDES the EV-cert plan above: SSL.com eSigner (~$1250/yr) is dropped;
the EV scaffold in release.yml stays commented as a fallback only if a
direct-download channel is ever needed. Next: reserve the "Pulsar" app name,
match AppxManifest Publisher/Identity to the Store-assigned values, submit.

## 2026-07-08 — Store submission automation (msstore CLI)

Azure AD app `pulsar-store-ci` (client 30b823e6-…, tenant 19420c32-…) drives
the Microsoft Store submission API from CI. Secrets MSSTORE_TENANT_ID /
MSSTORE_CLIENT_ID / MSSTORE_CLIENT_SECRET set. `.github/workflows/store-submit.yml`
is workflow_dispatch (never auto — beta tags must not publish to the Store).
Remaining before it works: (1) link the app in Partner Center → Account
settings → User management → Azure AD applications as Manager; (2) repo var
STORE_SELLER_ID; (3) account verification complete. The exact msstore submit
subcommand is confirmed on the first real run (blind until the account is live).

## 2026-07-08 — Store API access WORKING (msstore CLI)

Proven end-to-end: store-auth-check ran `msstore apps list` and returned
product 9P26FDCWV1GC "Pulsar Barycenter" ReluxWorksLLC.PulsarBarycenter.
Hard-won steps: (1) Partner Center had NO associated Entra tenant — associated
adminrelux.onmicrosoft.com (19420c32-…) via its GLOBAL ADMIN UPN
ivan@adminrelux.onmicrosoft.com (NOT admin@relux.works — relux.works isn't a
verified Entra domain; admin@relux.works is only the MSA). (2) Linked
pulsar-store-ci as Manager(Windows). (3) msstore CLI is NOT a NuGet dotnet
tool — download the self-contained binary from
github.com/microsoft/msstore-cli releases (MSStoreCLI-linux-x64.tar.gz, binary
`msstore`). (4) On Linux it stores creds via libsecret → needs libsecret-1-0 +
a running freedesktop secret service: run inside `dbus-run-session` with an
unlocked gnome-keyring. store-submit.yml uses the same wrapper. Remaining for a
real submission: listing metadata + screenshots (Timur) on the product.

# P3 E2EE threat model and honest claims v1

`TASK-260712-2e2ymn` freezes the security question before a primitive,
library, container or recovery design is selected. The machine-readable source
of truth is `acceptance/phase3/e2ee-threat-model-v1.json`.

Decision: the threat model is ready for the two engineering spikes. It does
not authorize E2EE implementation, enable `e2ee_media_v1`, accept an
independent review or permit an E2EE product claim. All four remain false or
`not-run` in the contract.

## Trust decomposition

The coordinator has two logically different roles. As a delivery and storage
service it is untrusted for content: it may route signed client-produced public
membership state, ciphertext and disclosed metadata, but it may never create,
unwrap, escrow, log or recover a content secret. A malicious delivery service
is in scope for content confidentiality and integrity. It may still withhold,
delay, partition or reorder traffic, so availability is not an E2EE promise.

As the current identity service, the coordinator can bind actors to device
credentials. That role is more powerful. MLS explicitly depends on correct
authentication-service behavior, while a malicious delivery service cannot by
itself recover the group key. Therefore a coordinator-issued credential alone
is not a verified-device claim. Device fingerprints or safety codes, visible
device-change warnings and an independently reviewed consistency or
transparency design must make a substituted device or inconsistent device set
detectable. Until that exists, identity-service equivocation blocks the
verified E2EE claim rather than being waved away.

This split follows the [MLS architecture](https://www.rfc-editor.org/rfc/rfc9750.html),
which describes an untrusted delivery service, a security-critical
authentication service, per-device clients, state-loss recovery limits and
metadata exposure. The architecture also notes that delivery-service forks
can require out-of-band epoch-authenticator comparison. The threat model uses
the full network-control posture from [RFC 3552](https://www.rfc-editor.org/rfc/rfc3552.html).

## Assets and attackers

The protected assets are audio plaintext, decoded samples, content and session
keys, device private keys, epoch state, recipient wraps, history grants,
content-dependent manifest fields, report plaintext after export and every
local plaintext cache or temporary file.

| Actor or failure | What the protected path promises | Exact limit |
|---|---|---|
| storage, backup or traffic capture | no protected plaintext or content secret | disclosed identifiers, sizes, duration and timing remain visible |
| honest-but-curious coordinator | cannot read protected audio through normal services or observability | still knows the disclosed routing and policy metadata |
| malicious delivery coordinator | cannot decrypt or inject content accepted as an honest sender; forks, stale epochs and replay fail or become detectable | can deny service, delay traffic and perform traffic analysis |
| malicious identity coordinator | device or credential equivocation is detected and blocks verified status | prevention depends on the reviewed verification/consistency design, not MLS alone |
| malicious group member | cannot forge another verified sender or derive epochs outside authorized membership | can copy all plaintext and keys legitimately received while authorized |
| compromised or cloned device | fresh client-owned rotation can restore future security after removal and an honest update | compromise exposes that device's accessible plaintext and keys |
| lost device or state | a surviving authorized device or explicit user-held capability may recover scoped access | without either, protected history may be irrecoverable |
| moderator | sees report and routing metadata only | an explicit evidence export creates a controlled plaintext copy |
| local OS compromise | no cryptographic promise while the endpoint is compromised | malware/admin/debugger/screen/audio access is outside the guarantee |
| physical recipient recording | no prevention claim | an authorized recipient can record or redistribute rendered audio |

[RFC 9420](https://www.rfc-editor.org/rfc/rfc9420.html) supplies the current-
epoch removal property and explains why group state remains message-protected
even over an untrusted transport, while still recommending TLS or QUIC to
reduce metadata and denial attacks. [RFC 9750](https://www.rfc-editor.org/rfc/rfc9750.html)
also makes the endpoint-compromise and state-loss limits explicit.

## Protected and excluded paths

Clips, tracks, saved cues and live PTT are required protected paths, but none
is protected in the current product. A future clip, track or cue claim applies
only to the exact immutable item carried in the reviewed authenticated
container. A live claim applies only to fresh session keys and frames
authenticated before jitter decode. A cue inherits the protection state of its
underlying immutable media revision; a protected room name does not upgrade a
plaintext cue.

Telegram upload and delivery are never called Pulsar E2EE because Telegram and
the bot already saw plaintext. Spotify playback/control stays in the provider
boundary and is not protected by Pulsar E2EE. A legacy or mixed-version target
is either explicitly excluded by the sender or makes the protected send fail.
There is no plaintext fallback.

An E2EE recipient may deliberately export one evidence copy. Before export,
the UI must identify the exact item and disclose that the plaintext leaves the
E2EE boundary and becomes accessible to authorized moderators. The copy is
purpose-limited, access-controlled, audited, short-lived and deletable. There
is no coordinator or moderator decrypt endpoint behind that flow.

## Honest metadata disclosure

The coordinator and its backups may see:

- account, actor and device identifiers;
- Orbit, Air, group and membership identifiers;
- exact recipient and target snapshots;
- group epoch and public commit identifiers;
- protocol, container and capability versions;
- media class, delivery mode and policy state;
- ciphertext size, chunk count and declared duration;
- upload, send, play, receipt, report, revoke and delete timing;
- network addresses, connections and request metadata;
- retention and privacy-safe audit state.

Audio plaintext, decoded samples, all content/session/epoch/history secrets,
user filenames, titles, captions, waveform/loudness analysis, local decrypt
caches and manifest fields not required for routing remain encrypted or local.
The disclosure is conservative: an implementation may reveal less, but UI or
policy copy must not promise less until a reviewed protocol demonstrates it.

## Mandatory security requirements

The JSON contract contains 22 normative requirement IDs. The non-negotiable
boundaries are:

- secrets originate and remain on Pulsar clients; coordinator rows, logs,
  metrics, traces, crash reports and backups never contain them;
- every device key has actor-visible verification state and device changes are
  conspicuous;
- signed client-owned commits advance the epoch after join, leave, role
  removal, credential revoke or suspected compromise;
- group, epoch, sender device, target snapshot, media identity, container
  version and chunk/frame position are authenticated before plaintext release;
- replay, fork, stale epoch, generation and downgrade are application-level
  policies and fail closed;
- nonce uniqueness survives multiple devices, crashes and restarts, with
  separate domains for media chunks, live frames, wraps and exports;
- new devices get no history without an explicit scoped grant;
- removal and deletion stop new grants and fetches but never promise erasure
  of material another endpoint already obtained;
- local plaintext and obsolete keys are bounded and wiped on every terminal
  path the OS permits;
- observability, moderation, analytics and search have no hidden decrypt path.

[RFC 5116](https://www.rfc-editor.org/rfc/rfc5116.html) requires nonce
uniqueness for a fixed AEAD key and states that AEAD itself does not supply
replay or access-control policy. [RFC 9180](https://www.rfc-editor.org/rfc/rfc9180.html)
is not selected by this task; its stated non-goals—replay, downgrade and
plaintext-length hiding—are recorded so a downstream HPKE-based design cannot
silently inherit properties it does not provide.

## C4-C6 mapping

| Scenario | Required result | What it does not prove |
|---|---|---|
| C4 membership crypto | after B is removed and a valid client commit advances the epoch, B cannot decrypt later clips, tracks, cues or live PTT; new D has no earlier history without a grant | erasure of plaintext, keys or recordings B already obtained |
| C5 coordinator privacy | coordinator storage, backups, logs and captured traffic cannot produce playable protected content; observed metadata matches this document | availability, anonymity, traffic-shape hiding or protection from malicious endpoints |
| C6 report | a recipient deliberately exports one disclosed evidence copy into the existing moderation workflow | a general server decrypt capability or invisible proactive scanning |

## Product claim rules

Even after implementation, the phrase “end-to-end encrypted” is item- and
path-specific. It is allowed only when all intended endpoints negotiated the
reviewed capability, the actual item uses the protected container/session, the
shown devices have the required verification state, and the independent design
and implementation reviews passed on the same hash.

Allowed claim shapes after those gates are:

- “End-to-end encrypted between the verified Pulsar devices shown for this
  item.”
- “The coordinator routes this protected item but cannot read its audio
  content.”
- “This report copy leaves end-to-end encryption and will be available to
  authorized moderators.”

Forbidden claims include “all Pulsar audio is E2EE”, “deletion erases every
copy”, “E2EE makes users anonymous”, “moderators can inspect protected content
without a report copy”, or any implication that TLS, a private Air, Telegram,
Spotify or a protected sibling capability upgrades this item. Current legal
and Store text continues to say the shipped paths are not E2EE.

## Review entry and residual risk

Independent design review cannot start from prose alone. The packet must pin
the exact threat-model bytes; library/algorithm/version, license, SBOM and CVE
decision; container and nonce domains; cross-platform vectors; join/remove,
fork/replay, history grant, recovery, report and downgrade state machines;
device-verification/equivocation design; plaintext/key lifecycle; reproducible
negative/fuzz results; and claim language. Critical and high findings must be
closed on the same reviewed hash.

Residual risks remain explicit: traffic analysis and denial of service;
identity-service equivocation until detected; malicious recipient exfiltration;
endpoint/OS compromise; irrecoverable key loss; the new plaintext created by a
report; long-lived ciphertext in backups; and integration failures in entropy,
nonce, deletion or constant-time behavior. These risks are owned or disclosed,
not relabeled as cryptographic success.

The structure is data-centric in the sense described by the
[NIST SP 800-154 initial public draft](https://csrc.nist.gov/pubs/sp/800/154/ipd).
That draft is cited only as methodology guidance and is not represented as a
final standard.

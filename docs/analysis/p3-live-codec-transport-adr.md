# P3 live codec and transport spike

Task: `TASK-260712-lo7a68`

Decision date: 2026-07-16

Owner: Ivan Oparin

## Decision

Freeze one engineering profile, but keep production live PTT disabled:

- libopus 1.6.1, 48 kHz mono, `OPUS_APPLICATION_VOIP`;
- 20 ms frames, 24 kbit/s constrained VBR, complexity 5;
- DTX off, in-band FEC on with expected loss 2%, PLC for gaps FEC cannot recover;
- 400-byte encoded payload ceiling, 40-byte application header, 440-byte maximum binary message;
- the existing authenticated WSS/TCP stack is the temporary engineering transport;
- each recipient has an independent eight-frame/160 ms queue and drops its oldest unsent live frame on overflow;
- QUIC DATAGRAM is deferred; it is not silently introduced as an unbuilt native dependency.

The formal verdict is **engineering-profile-frozen-production-no-go**. Wire and
client engineering may continue behind a disabled capability, but no release may
advertise or enable live PTT until the four High blockers in
`acceptance/live-ptt/codec-transport-decision-v1.json` close.

## Why 20 ms

The local pinned benchmark processed 240 seconds of synthetic mono audio. The
20 ms profile averaged 169.701 microseconds encode and 34.631 microseconds
decode per frame, a combined real-time factor of 0.010217. Its measured payload
was 57 bytes p50, 70 bytes p95 and 77 bytes maximum. Codec state was 50,136
bytes; the probe's bounded working buffers were 52,240 bytes. The 10 ms profile
also fit comfortably but doubled message frequency and consumed slightly more
codec CPU per audio second. These are single-host engineering measurements, not
cross-platform or real-speech quality evidence.

The official libopus 1.6.1 release pins the source SHA-256 to
`6ffcb593207be92584df15b32466ed64bbec99109f007c82205f0194572411a1`.
RFC 6716 defines the interactive codec controls, including frame duration,
complexity, loss resilience, FEC, VBR and DTX. RFC 8854 describes the important
one-frame nature and overhead tradeoff of Opus in-band FEC. Sources:
[libopus 1.6.1](https://opus-codec.org/release/stable/2026/01/14/libopus-1_6_1.html),
[RFC 6716](https://www.rfc-editor.org/info/rfc6716/), and
[RFC 8854](https://www.rfc-editor.org/info/rfc8854/).

DTX stays off because press/release framing must not depend on an implicit
silence state. FEC may reconstruct only the immediately preceding exposed gap;
otherwise the decoder performs PLC at the scheduled playout point. Nothing
waits past the jitter deadline.

## Transport result

The deterministic model injects 2% loss independently on each leg, bounded
jitter, rare path spikes, reordering for datagrams and a slow recipient. It is
deliberately more adverse than interpreting 2% as a single end-to-end rate.

| Profile | p50 | p95 | p99 | Maximum | Meaning |
|---|---:|---:|---:|---:|---|
| 20 ms WSS/TCP | 272.266 ms | 458.432 ms | 534.549 ms | 699.367 ms | 763 modeled retransmission/HOL events; TCP delivered every application frame |
| 20 ms QUIC DATAGRAM | 248.896 ms | 289.769 ms | 413.045 ms | 517.411 ms | 735 FEC recoveries and 39 PLC frames; no retransmission |

Both modeled profiles fit the C2 p50 800 ms and p95 1500 ms envelope. This is a
budget plausibility result only. RFC 6455 defines WebSocket over TCP; Apple
documents `URLSessionWebSocketTask` as WebSocket framing over TCP/TLS. TCP
therefore cannot promise expired-frame discard below its byte stream. RFC 9221
provides unreliable QUIC datagrams; MsQuic supports them, but receiving is off
by default and must be negotiated. Sources:
[RFC 6455](https://www.rfc-editor.org/info/rfc6455/),
[Apple URLSessionWebSocketTask](https://developer.apple.com/documentation/foundation/urlsessionwebsockettask),
[RFC 9221](https://www.rfc-editor.org/info/rfc9221/), and
[MsQuic settings](https://github.com/microsoft/msquic/blob/main/docs/Settings.md).

WSS remains the engineering baseline because Gorilla WebSocket 1.5.3 is already
pinned in the coordinator and Windows app and Foundation supplies the macOS
client. QUIC DATAGRAM remains the architectural preference if physical WSS
evidence fails, but MsQuic 2.5.8 would add a native client/server integration,
package, signing, notarization and update surface that has not been built or
measured. See the [Gorilla 1.5.3 release](https://github.com/gorilla/websocket/releases/tag/v1.5.3)
and [MsQuic 2.5.8 release](https://github.com/microsoft/msquic/releases/tag/v2.5.8).

The queue model held the fast recipient at zero drops while a recipient six
times slower filled only eight frames and dropped 1,658 oldest frames. This
proves the algorithmic bound and isolation, not coordinator production memory.

## Binary framing boundary

The spike reserves a fixed 40-byte network-order header: `BP` magic, version,
flags, 128-bit session ID, uint32 sequence, uint64 capture-monotonic timestamp,
uint16 payload length, fixed frame duration/channels/sample-rate/codec fields and
zero reserved bytes. Payload length is at most 400 and exact message length must
equal header plus payload. Invalid fixed fields, state or sequence are rejected
before allocation. The following wire-contract task may add control-plane
messages, but may not widen these codec or allocation bounds without rerunning
the spike.

## License, SBOM and hostile-input boundary

The reference implementation is BSD-3-Clause and its binary notice obligations
must ship. The Opus project publishes royalty-free patent grants with termination
conditions and third-party IPR caveats; this ADR is not legal advice. Preserve
the exact source, build recipe, binary hashes, notices and SBOM and rescan each
release. See the [official Opus license page](https://opus-codec.org/license/).

No zero-vulnerability claim is made. CVE-2026-40614 is a PJSIP 2.16-and-earlier
Opus wrapper buffer-sizing flaw, not a libopus 1.6.1 finding, but it demonstrates
why our wrapper must validate the 400-byte ceiling before copying or allocating.
The exact future binaries still require current scanners and hostile-frame tests.
Source: [NVD CVE-2026-40614](https://nvd.nist.gov/vuln/detail/CVE-2026-40614).

## Why production is blocked

The repository's Windows release is currently CGO-free and stages no libopus
binary. The Swift package stages no libopus target or nested library. There is no
Windows benchmark, no macOS arm64 benchmark, no signed MSIX load receipt, no
hardened-runtime/notarization receipt, no exact runtime SBOM, and no hostile
decoder corpus result. A Homebrew dylib on one x86_64 host does not prove any of
those properties and will never be downloaded on first run.

Finally, the deterministic model cannot test speech intelligibility or an
acoustic mouth-to-ear path. `TASK-260712-1rzqh9 — live-ptt-regression-evidence`
in `EPIC-260714-th54l3 — manual-real-app-hardware-testing` owns the physical
Windows-Windows, Windows-macOS and macOS-macOS two-home matrix, 2% impairment,
100-cycle lifecycle run and calibrated p50/p95 evidence. Until that task and the
three package/security blockers pass, production live PTT stays unavailable.

The macOS receiver implementation and its explicit system-decoder/FEC boundary
are documented in [p3-macos-live-jitter-receiver.md](p3-macos-live-jitter-receiver.md).
The matching bounded capture/encode path and its explicit system-encoder/FEC
boundary are documented in
[p3-macos-live-capture-sender.md](p3-macos-live-capture-sender.md).

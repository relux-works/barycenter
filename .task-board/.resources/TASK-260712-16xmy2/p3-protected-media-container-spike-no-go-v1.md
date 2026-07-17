# P3 protected-media container preparation spike: production no-go v1

Date: 2026-07-17

Task: `TASK-260712-16xmy2`

Baseline: `e4121a3f5217b7a471ef0d4f34125de6f1f95de2`

Machine-readable decision: `acceptance/phase3/protected-media-container-spike-v1.json`

## Decision

The repository experiment passes, but the production decision is **no-go**.
No production codec, media container, local preparation toolchain, or
cryptographic suite is selected. E2EE implementation, feature enablement and
product claims remain blocked.

This is the required fail-closed outcome because the upstream Phase 2
codec/player ADR is still `no-go`, P2 whole-object integrity and independent
performance review findings remain open, and there is no signed Windows/macOS
physical evidence. A structural encryption experiment cannot manufacture the
missing decoder, packaging, hardware or review evidence.

## What the repository probe proves

`scripts/e2ee_container/probe` is an isolated Go 1.25.12 standard-library
experiment named `pmc-probe-v1`. It is deliberately not registered as
`e2ee_media_v1` and contains no production codec.

The probe freezes a candidate-neutral structure:

- a 144-byte authenticated public header and separately authenticated private
  manifest;
- at most 1 MiB of plaintext per independently authenticated record;
- distinct HKDF-SHA256 domains for the public manifest, private manifest and
  media chunks;
- AES-256-GCM experiment nonces formed from a four-byte random prefix and a
  64-bit counter, with reserved counters for both manifests;
- chunk AAD binding the full public-header hash, record index and exact
  plaintext length;
- an exact byte-range rule and a resumable-upload rule that rolls a partial
  record back to the preceding authenticated boundary;
- fail-closed deterministic tests for header/manifest/chunk tamper,
  truncation, reorder, cross-container substitution, and insertion of a
  stale-epoch record into a current container,
  wrong keys and unsafe bounds.

The deterministic fixture container SHA-256 is
`1ed44e2c5e5739c97840d2d82ccb6582e16647686159a85578b0516eb74398b8`.
The four-chunk 4 MiB synthetic case limits structural overhead to 512 bytes.
A 2 MiB synthetic input with two records has 252 bytes of structural overhead
and authenticates a two-hour declared duration. These are repository
structure checks, not application performance evidence.

The probe computes exact record offsets and authenticated resume boundaries on
a complete in-memory byte slice. It does not implement or prove HTTP range,
resumable upload, storage revocation, cache invalidation or retry behavior.

An authenticated record from an earlier epoch cannot be inserted into a
different current header. However, cryptography alone cannot distinguish a
replay of an entire previously valid container. The later protocol must track
container identity, epoch and accepted generation and reject stale delivery.

Every caller would have to guarantee that a master-key, salt and nonce-prefix
tuple is never reused. This spike has no group/key lifecycle capable of making
that production guarantee; the next group-crypto spike and the later protocol
contract own it.

## What was not proved

No signed or packaged Windows/macOS application was run. There is no real
codec preparation path, no Swift implementation, and no Windows-to-macOS or
macOS-to-macOS vector. Audible start, seek and scheduled skew, physical RSS,
disk and CPU, and a two-hour physical run are all `not-run` in manual epic
`EPIC-260714-th54l3`.

The self-test's local microsecond and heap observations are intentionally not
acceptance evidence. They omit decoding, sandbox/package behavior, real audio,
OS buffering, cache pressure, power and hardware diversity.

The CLI keeps its synthetic plaintext in memory, writes no plaintext or keys
to disk, prints no secrets, and best-effort clears several buffers. Go and the
operating systems do not guarantee compiler-proof zeroization, exclusion from
crash/swap/hibernation state, or cleanup of future codec buffers. No production
plaintext-lifecycle claim is made.

## Toolchain, license and supply boundary

The probe pins Go `1.25.12`, uses only the Go standard library, downloads no
runtime executable and introduces no third-party codec. The Go project is
distributed under its BSD-style license, so this repository-only experiment
adds no new third-party codec license obligation.

That does not approve a production supply chain. A production codec SBOM,
license approval, signed artifacts and update path do not exist. Go's AES
source also warns that the non-hardware implementations are not constant-time;
therefore this spike cannot approve the AES experiment on every intended
architecture without a reviewed implementation and hardware policy.

Primary references:

- Go HKDF source: <https://go.dev/src/crypto/hkdf/hkdf.go>
- Go AES source and constant-time caveat: <https://go.dev/src/crypto/aes/aes.go>
- Go `cipher.AEAD` nonce contract: <https://pkg.go.dev/crypto/cipher>
- Go license: <https://go.dev/LICENSE>

## Unblock conditions

Production remains blocked until all of the following are accepted against the
same candidate and exact source/build receipts:

1. a production codec/container and Store-safe local preparation path;
2. a reviewed group-key lifecycle and production crypto suite;
3. an independent Swift implementation with cross-platform vectors;
4. authenticated HTTP range, resumable upload, revocation and full-container
   replay-state integration;
5. signed Windows/macOS physical pairing and Phase 2 performance evidence;
6. a bounded, reviewed plaintext lifecycle; and
7. independent cryptographic/container design review with critical and high
   findings closed.

Until then, downstream work may use only candidate-neutral contracts and test
doubles. It may not generate or play production protected media, silently
downgrade, download executable code at runtime, or make an E2EE claim.

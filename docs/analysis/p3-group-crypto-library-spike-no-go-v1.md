# P3 group cryptography and library spike: production no-go v1

Date: 2026-07-17

Task: `TASK-260712-3er89x`

Baseline: `747f6862706d7a6f68be1cc2013f3f69ff8ce84c`

Machine-readable decision: `acceptance/phase3/group-crypto-library-spike-v1.json`

## Decision

RFC 9420 Messaging Layer Security is the only standardized protocol candidate
that fits Pulsar's asynchronous two-to-many Air membership, offline recipients,
client-authored membership changes, forward secrecy and post-compromise
requirements. No production library, cryptographic provider, cipher suite,
serialization or platform binding is selected. The production result is
**no-go**, and E2EE remains blocked, disabled and unclaimed.

This task did not write a new group protocol or download an unselected crypto
stack. Doing either would bypass the explicit maintained/audited-library gate.
The local environment also has no Rust toolchain, while the product clients are
Swift and Go. Tool installation alone would not close the audit, binding,
signed-package, cross-implementation or secret-lifecycle gaps.

## Protocol assessment

MLS supplies the standardized group wire format, KeyPackages, Welcome,
proposals, commits, epochs, per-sender generations, cipher-suite registry and
key schedule needed for the later protocol contract. RFC 9420 requires
applications to use `PrivateMessage` for application data, protect against
downgrade using member capabilities, avoid KeyPackage reuse and delete consumed
secret representations. RFC 9750 models a largely untrusted delivery service
and explains why sender-key redistribution is not an equivalent group protocol.

MLS deliberately does not solve all Pulsar requirements. The application still
owns device verification, authentication-service equivocation detection,
concurrent commit/fork resolution, message-loss policy, target/media binding,
history grants, recovery, mixed-version behavior and actual secure storage.
Coordinator credentials alone cannot establish a verified-device claim.

Pairwise sender keys or a custom tree protocol are rejected. If a sender key is
learned, an attacker can keep decrypting that sender until a new key is
distributed, and group-wide redistribution grows linearly. Inventing a custom
replacement would violate this task's no-custom-cryptography rule and discard
the standardized MLS analysis and interop ecosystem.

## Candidate findings

### OpenMLS 0.8.1

The exact release is `openmls-v0.8.1`, commit
`47dbedecad0c1fd8eb5368d582250ebfcc1e1ce6`, MIT licensed. It advertises all
three RFC 9420 suites relevant to the comparison. The assessed libcrux provider
is version 0.3.1 with libcrux primitives 0.0.6 and hpke-rs 0.6.0; those versions
include the February 2026 dependency advisory fixes.

OpenMLS has the strongest audit story of the candidates. SRLabs reported eight
findings, including one High; the maintainer says the findings were remediated
for 0.8.1 except for one still-open Low. The release is also beyond the two
published OpenMLS advisory ranges.

It is still a no-go for this product. The official test matrix tests Intel
Windows and Intel macOS, while Apple arm64 is build-only. The repository exposes
no official Swift or Go application binding, and there is no Pulsar signed MSIX,
notarized macOS, cross-client interop, full dependency audit or storage proof.
An audited Rust core is necessary but not sufficient evidence for the FFI and
application lifecycle that would handle every secret.

### mls-rs 0.55.2

The exact tag is `0.55.2`, commit
`42131c9959efb1d3928428259bc89853027f730d`, dual Apache-2.0/MIT licensed,
requiring Rust 1.82 or newer. It has a broad upstream Ubuntu/macOS/Windows test
matrix and a UniFFI crate. However, its own README states that it has not yet
received a full third-party security audit.

The UniFFI crate is 0.13.0, uses UniFFI 0.31 and the OpenSSL provider rather
than the separate CryptoKit provider. Its official foreign tests are Kotlin and
Python, not Swift. The Windows upstream workflow dynamically provisions
OpenSSL, SQLCipher and SQLite through vcpkg. The upstream interop workflow
compares mls-rs feature variants with mls-rs, not an independent
implementation. None of that proves a closed Store package or Pulsar
Swift/Windows interoperability.

### MLS++

MLS++ has only the `v0.1.0` tag from 2021. The assessed main snapshot is
`92aaa4134fa45ec39957a7c81a342401fba7feb2`, but there is no current versioned
release corresponding to it. The BSD-2-Clause C++17 library requires OpenSSL or
BoringSSL and nlohmann/json, with vcpkg recommended for developer builds. No
external audit, supported Swift/Go binding or signed Store supply closure was
found in the authoritative repository material. It is not selectable.

## Evidence not run

No RFC known-answer suite was run against a selected product stack because no
stack passed the entry gates. No independent-implementation interop,
Windows-to-Windows, Windows-to-macOS or macOS-to-macOS vector was run. Offline
multi-device join, concurrent commit/fork, removed-member stale epoch,
application replay, secret deletion/rollback, performance and memory remain
`not-run`.

No signed MSIX or notarized macOS application was built for MLS. Those real-app
and physical results remain in manual epic `EPIC-260714-th54l3`; repository CI
must not reinterpret upstream builds as Pulsar hardware evidence.

## Frozen handoff to the protocol contract

The next task may specify only RFC 9420 semantics and candidate-neutral
interfaces. It must keep all of these rules fail-closed:

1. current verified clients, never the coordinator, author membership commits;
2. authentication-service device bindings require independent verification;
3. KeyPackages are one-time and group identifiers are freshly generated;
4. the application owns accepted epoch, generation, fork and replay state;
5. secure transport remains mandatory despite message protection;
6. consumed and obsolete secret representations must actually be deleted;
7. new-device history requires an explicit client-owned grant;
8. the coordinator never generates, unwraps, escrows or recovers content keys;
9. capability or cipher-suite downgrade is never silent; and
10. no suite or serialization identifier is advertised before selection and
    independent review.

## Unblock conditions

Production needs one exact audited library/provider/suite/binding stack; closed
critical and high findings with explicitly reviewed residuals; independent
cross-implementation known-answer and lifecycle vectors; signed Windows/macOS
packaging; verified-device/equivocation design; client storage, deletion,
rollback, history and recovery proof; a complete SBOM/license/notices/CVE/update
plan; and independent cryptographic design review.

Primary sources:

- [RFC 9420](https://www.rfc-editor.org/rfc/rfc9420.html)
- [RFC 9750](https://www.rfc-editor.org/rfc/rfc9750.html)
- [OpenMLS 0.8.1](https://github.com/openmls/openmls/releases/tag/openmls-v0.8.1)
- [OpenMLS audit summary](https://blog.openmls.tech/posts/2026-05-27-independent-audit/)
- [mls-rs 0.55.2](https://github.com/awslabs/mls-rs/tree/0.55.2)
- [MLS++ assessed snapshot](https://github.com/cisco/mlspp/tree/92aaa4134fa45ec39957a7c81a342401fba7feb2)

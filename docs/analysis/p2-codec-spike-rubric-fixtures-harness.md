# P2 codec/player spike rubric, fixtures and harness

- Date frozen: 2026-07-15
- Task: `TASK-260712-14u0yk`
- Normative contract: `acceptance/codec-spike/rubric-v1.json`
- Evidence template: `acceptance/templates/codec-spike-evidence-v1.json`
- Status of this handoff: proof protocol ready; no decoder candidate selected

## Outcome

Every decoder candidate now receives the same frozen inputs, commands,
aggregation rules and rejection criteria before prototype results exist. The
repository can validate the contract, generate a content-addressed corpus,
serve it through an authenticated range/fault endpoint and evaluate complete
synthetic or real-hardware evidence. A synthetic pass is labelled
`engineering-pass` and has `finalClaim: false`; the normal evaluator rejects it
as final evidence.

The long encoded files are generated into private evidence storage rather than
committed to Git. One exact `fixture-lock.json` binds every byte, toolchain
configuration, duration and decoded shape. All candidate and comparative runs
must reference the same lock SHA-256. Regenerating a lock starts a new matrix;
mixing locks is invalid evidence.

## Frozen proof map

| Spec proof | Shared check and artifact |
| --- | --- |
| Decode MP3, AAC and Opus | Six 12-second smoke files plus six one-/two-hour files; fixture lock records codec, container, size, SHA-256, 48 kHz stereo shape and duration; `decode_expected_pcm` has zero-failure semantics. |
| Range/chunk fetch | `range_harness.py` request JSONL across normal, no-range, slow, reset, truncated, corrupt, ETag-change and revoked profiles. |
| Pause, seek and resume | Three seek-heavy two-hour VBR fixtures, fresh seek generation, marker-band comparison and zero failure lifecycle checks. |
| Bounded memory | Process-tree RSS at 1 Hz; ≤200 MiB peak, ≤16 MiB one-hour-to-final-window growth and absolute slope ≤1 MiB/hour. |
| Scheduled start | Three warmups plus exactly 30 retained samples for Windows↔Windows, Windows↔macOS and macOS↔macOS; nearest-rank p95 ≤100 ms. |
| Start and seek latency | Three warmups plus exactly 30 retained samples per candidate/build/pairing/node/fixture; nearest-rank p95 ≤5 s and ≤3 s. |
| Start before full download | First-audio byte count must be less than content length on every one-hour fixture. |
| Store/AppContainer | Exact package SHA, architecture, identity, OS build and sandbox receipt are mandatory evidence fields; runtime executable download is forbidden. |
| License suitability | The exact shortlist flows to `TASK-260712-1vdlkw`; a candidate cannot pass merely because it decodes. |

Timed failures remain samples. The evaluator requires exact group coverage and
does not trim outliers, accept extra cherry-picked runs or replace a failed run.
Decoder crashes, hangs, security faults, output after revoke, render-callback
I/O/allocation/wait/lock behavior, missing samples and unknown fixture locks are
hard rejection.

## Corpus

The generator uses synthetic, non-copyrighted stereo marker tones at 48 kHz.
Frequency bands change every ten seconds, so seek checks can compare the
expected position with decoded output without listening or speech recognition.

The long corpus is:

- MP3 CBR, one hour, 192 kbit/s;
- MP3 VBR quality 4, two hours, seek-heavy;
- AAC-LC/M4A fast-start CBR, one hour, 160 kbit/s;
- AAC-LC/ADTS VBR quality 2, two hours, seek-heavy;
- Opus/Ogg CBR, one hour, 128 kbit/s;
- Opus/Ogg VBR, two hours, seek-heavy.

The matching smoke set covers the same six codec/container/rate shapes in 12
seconds. Eight hostile derivatives cover truncated MP3/ADTS/MP4/Ogg, oversized
ID3, MP4 atom-size overflow, an MP3 mid-frame bit flip and an Ogg CRC fault.
The hostile set must be rejected without process crash or unbounded resource
use; it is not expected to decode successfully.

Fixture generation is pinned to signed FFmpeg 8.1.2 source and records the
complete `ffmpeg -version` configuration digest. The generator refuses a
missing or different version and never downloads an executable. FFmpeg 8.1.2
is the 2026-06-17 stable release published and signed by the FFmpeg project:
<https://ffmpeg.org/download.html>. The frozen official tarball SHA-256 is
`464beb5e7bf0c311e68b45ae2f04e9cc2af88851abb4082231742a74d97b524c`;
the detached-signature SHA-256 is
`0a0963fccd70597838073f3e31b20f4a4d8cc2b5e577472c9a5a1f22624246f8`.
Signature verification against the frozen FFmpeg release-key fingerprint is
mandatory before a corpus lock is accepted.

## HTTP range and fault contract

The local harness serves only files whose bytes match the fixture lock. It
rejects symlinks, unsafe paths and mismatched sizes/hashes at startup. Every
request needs both an exact bearer and `X-Codec-Spike-Target`; authorization
failure is non-disclosing `404`. Tokens are absent from the ready file and
sanitized request log.

Normal behavior includes single byte ranges (closed, open and suffix), `206`
with exact `Content-Range`, `416` with total length, strong content-derived
ETags, `If-Range`, `If-None-Match`, private cache control and a whole-object
SHA-256 header. Multiple ranges are deliberately rejected in this spike.

Fault profiles are deterministic:

- `no_range` ignores a valid Range and returns whole-object `200`;
- `slow_256kbit` throttles body chunks;
- `reset_mid_body` closes the socket after half the declared body;
- `truncate_body` ends after half the declared body;
- `corrupt_chunk` preserves length but flips a response byte;
- `etag_flip` rotates the validator after the first request;
- `revoked` returns `410`;
- `normal` is the control.

Cache eviction is a client action, not a server fiction. Evidence samples
app-private cache bytes at 1 Hz and after eviction, revoke and shutdown. The
candidate declares a provisional ceiling; `TASK-260712-dqdoqj` owns the final
production range/cache contract and exact cache ceiling.

## Candidate shortlist

The shortlist is an experiment inventory, not an approval:

1. `native-canonical-aac-v1`
   - Windows Media Foundation Source Reader with inbox MP3/AAC support;
   - macOS Audio File Stream Services plus AudioConverter/AVAudioEngine;
   - canonical AAC-LC/M4A server variant for unsupported source combinations.
   Microsoft documents native MP3 and AAC but does not list Ogg/Opus, so the
   Windows probe must not assume inbox Opus support:
   <https://learn.microsoft.com/windows/win32/medfound/supported-media-formats-in-media-foundation>.
   Apple documents incremental Audio File Stream parsing and seeking for MP3,
   ADTS and AAC:
   <https://developer.apple.com/documentation/audiotoolbox/audio-file-stream-services>.
2. `pure-go-composite-v1`
   - `github.com/hajimehoshi/go-mp3@v0.3.4`;
   - `github.com/llehouerou/go-aac@v0.0.0-20260119142340-5f2857eb82ad`;
   - `github.com/Eyevinn/mp4ff@v0.54.0`;
   - `github.com/pion/opus@v0.1.0`, including its Ogg reader.
   These versions are frozen for measurement only. Correctness, maturity,
   license, patent, fuzz/security and memory evidence remain mandatory.
3. `bundled-ffmpeg-8.1.2-v1`
   - a minimal library bridge around libavformat/libavcodec/libswresample;
   - signed inside MSIX and codesigned/notarized inside the macOS bundle;
   - no CLI spawn and no first-run download.

Windows must blind-build amd64 and arm64; macOS targets Apple Silicon. The
minimum real timing matrix uses packaged Windows x64 and packaged Apple-Silicon
macOS nodes. Exact OS builds, hardware, package hashes and topology are evidence
data rather than hidden assumptions.

## Evidence evaluation

`evaluate_evidence.py` computes nearest-rank p95, maximum and absolute-maximum
from retained samples. It constructs the expected Cartesian groups from the
three pairings, two nodes, six long fixtures and seek subset. It rejects a
missing or unexpected group, wrong unit/count, any unsuccessful sample,
incomplete fixture/range coverage, incomplete provenance, an unknown candidate
or any failed zero-failure check.

Repository-only use is explicit:

```sh
python3 scripts/codec_spike/validate_contract.py
python3 scripts/codec_spike/generate_fixtures.py --plan
python3 -m unittest scripts/codec_spike/test_codec_spike.py
python3 scripts/codec_spike/evaluate_evidence.py evidence.json --engineering
```

Actual corpus creation and serving:

```sh
python3 scripts/codec_spike/generate_fixtures.py \
  --output private-codec-corpus \
  --lock private-evidence/fixture-lock.json

CODEC_SPIKE_TOKEN=<secret> python3 scripts/codec_spike/range_harness.py \
  --fixtures private-codec-corpus \
  --lock private-evidence/fixture-lock.json \
  --target <opaque-target> \
  --ready-file private-evidence/range-ready.json \
  --log private-evidence/range-requests.jsonl
```

## Honest remaining work

- This task does not choose a decoder, prove a license, ship a variant/cache
  protocol or claim physical p95/RSS/skew.
- Candidate implementation starts only after this contract lands. Every probe
  records rejected options as well as successes.
- If no complete Windows+macOS combination passes all gates and the downstream
  license review, the final ADR must be `no-go` rather than moving a threshold.
- Real packaged Windows/macOS and cross-machine audible evidence remains in the
  manual-test epic. Repository correctness makes those runs reproducible; it
  does not replace them.

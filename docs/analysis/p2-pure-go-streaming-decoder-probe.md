# P2 pure-Go streaming decoder probe

## Decision

`pure-go-composite-v1` remains **rejected**. The probe deliberately does not
import or distribute `github.com/llehouerou/go-aac`: the audited exact version
is GPL-2.0-only, identifies itself as a FAAD2 port and is forbidden by the
approved proprietary static-client posture. A two-module research build proves
the remaining technical boundaries without pretending that MP3 plus Opus is a
complete candidate.

The bounded result is also technically insufficient:

- `go-mp3@v0.3.4` produces first PCM incrementally when given an `io.Reader`,
  but that mode cannot seek. Giving it an `io.ReadSeeker` makes construction
  scan the complete MP3 to build its frame index before first PCM.
- `pion/opus@v0.1.0` plus its Ogg reader produces first PCM incrementally, but
  the Ogg API is forward-only and exposes no random-seek contract.
- neither missing seam may be hidden behind full-file allocation, CGo or a
  render-thread read.

The final comparative ADR may reuse the explicit adapter/ring seam, but it must
not select this exact composite.

## Exact build boundary

The isolated Go module pins only:

- `github.com/hajimehoshi/go-mp3@v0.3.4` (Apache-2.0);
- `github.com/pion/opus@v0.1.0` (MIT).

The validator checks the runtime package graph, forbids `go-aac`, rejects any
C/C++/CGo source in transitive runtime packages and verifies `CGO_ENABLED=0`
inside every emitted binary. The same source cross-builds darwin/arm64,
windows/amd64 and windows/arm64. The research binary owns no network and emits
PCM only into a fixed 1 MiB ring drained by the existing scheduling seam; it
does no render-thread I/O.

## Local evidence

On Intel macOS, both MP3 fixtures reached first PCM after 621 and 237 source
bytes. Seek-capable construction consumed 289,818 bytes for the 289,197-byte
CBR file and 52,674 bytes for the 52,437-byte VBR file, proving the complete
pre-scan. Both Opus fixtures reached first PCM after 16,264 and 19,410 bytes,
then decoded to drain, but recorded the missing seek API. Maximum observed
underlying reads were 636 bytes for MP3 and 255 bytes for Ogg/Opus; ring use
peaked at 7,680 bytes.

Both AAC fixtures are explicit zero-read `reject-forbidden-module` results.
The 3,859,360-byte darwin/amd64 binary has SHA-256
`cf11cf91da72b1edc5cb40de39c83c4dac29d6cce307741150a18210325152c4`.
Cross-build receipts bind the CGo-free binaries and hashes; hosted Windows,
macOS and Linux/race receipts supersede these local values for final task
tracking.

## Hosted platform evidence

Dedicated run `29450704499` passed all three jobs on exact code head
`dc1ac49`: native macOS ARM64, native Windows amd64 and Linux amd64 with the Go
race detector. Every platform reproduced the same fixture decisions and byte
counts. Scheduled skew was at most 5 ms; heap-system end state was 7,962,624
bytes on macOS and 8,093,696 bytes on Windows. These are Go heap measurements,
not process RSS.

The native CGo-free macOS ARM64 research binary was 3,568,866 bytes with
SHA-256 `b6e81e4fabb847837da8a36c49a8b66648fd7e02aaf1beeac7cf586296a9f74c`.
The native Windows amd64 binary was 3,746,304 bytes with SHA-256
`5625b40ddbc92e4c8b461b1416e552da64d13651aa5fb938a3b80e09efa1ab79`.
All jobs also emitted darwin/arm64, windows/amd64 and windows/arm64 cross-builds.
The Windows-hosted hashes differ from the Linux/macOS-hosted cross-build hashes,
so the receipts pin each artifact rather than claiming cross-host bit-for-bit
reproducibility. That supply-chain difference is retained for the comparative
matrix and cannot promote this already rejected candidate.

`go test -race` exercises eight concurrent Opus decoders. Hostile tests feed
truncated and bit-flipped MP3 plus truncated and CRC-corrupt Ogg data and fail
if either dependency panics. Pause, resume, drain and pre-read cancellation are
checked without asynchronous I/O. The command records Go heap-system bytes,
not process RSS; no two-hour RSS, AppContainer install, physical machine,
audible-output or production package claim is made. Those measurements remain
manual in `EPIC-260714-th54l3`, and the candidate is already rejected before
they could promote it.

## Reproduction

```sh
python3 scripts/codec_spike/validate_pure_go_probe.py
cd scripts/codec_spike/purego_probe
CGO_ENABLED=0 go vet ./...
CGO_ENABLED=0 go test ./...
go test -race ./...
cd ../../..
bash scripts/codec_spike/run_pure_go_probe.sh .temp/purego-probe
```

Candidate rejection is a successful probe outcome. A forbidden module, CGo
runtime package, oversized ring/read, panic, unexplained decode failure,
missing cross-build or evidence claiming `passed=true` fails closed.

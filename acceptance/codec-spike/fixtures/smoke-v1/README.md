# Bundled decoder smoke fixtures v1

These six deterministic, synthetic, 48 kHz stereo fixtures are repository test
inputs. They contain generated sine waves and no user or licensed source media.
Their exact digests and minimum durations are frozen in
`acceptance/codec-spike/bundled-probe-v1.json`.

They were generated on 2026-07-15 with the Homebrew FFmpeg 8.1.2_1 CLI from a
12-second `sine=frequency=997:sample_rate=48000` source, duplicated to stereo.
The CLI is a fixture-generation tool only: it is GPL-enabled and is neither
linked into nor copied into the prototype package. The packaged prototype is
built separately from the pinned pristine FFmpeg 8.1.2 source archive using the
LGPL-only configure allowlist in the contract.

The fixed variants are AAC/ADTS, AAC/M4A, MP3 CBR, MP3 VBR, Ogg Opus CBR and
Ogg Opus VBR. Regeneration is an explicit contract revision: update every digest,
review the generator/tool license boundary, and rerun all bundled-probe evidence.

# Implement Windows protected clip and track sending

## Description
Normalize encode chunk-encrypt sign and upload protected media locally with no server plaintext path.

## Scope
Use the approved local preparation toolchain and Windows key state to process recorded clips, selected files, tracks and saved-cue media into the canonical chunked container. Keep plaintext in bounded private memory or explicitly reviewed short-lived app storage, sign or authenticate manifests, generate unique keys and nonces, resumably upload ciphertext and client-produced envelopes and clean drafts after confirmed publication or explicit cancel. Preserve rights reminder, quotas, progress, retry and no-downgrade target confirmation.

## Acceptance Criteria
Signed Windows sends produce cross-platform valid protected media and coordinator ffmpeg never receives plaintext. Interrupted upload resumes without nonce or object reuse, cancellation and crash clean plaintext under the documented limit, unsupported recipients cannot cause silent plaintext fallback and manifests match golden and tamper vectors.

# Move Telegram voice intake onto SubmitMedia without changing legacy behavior

## Description
Refactor the Telegram voice path so the bot becomes a transport adapter over the common SubmitMedia service while preserving current voice acceptance ordering, defaults and compatibility WAV output.

## Scope
Replace direct DownloadVoice to media.Process ownership with a Telegram adapter that submits raw bot media through SubmitMedia, maps ready results back to the legacy queue path and keeps personal or broadcast defaults, FIFO by acceptance time, failure replies and old-node play_voice compatibility unchanged. Inline delivery actions, history and presence remain in the Telegram story.

## Acceptance Criteria
Default Telegram voice still lands first after the current element with the same acceptance-order guarantees even if processing finishes out of order. Bot-side failures reflect common ingest statuses instead of bespoke voice-pipeline branches. Regression tests prove legacy node playback, ordering and current bot reply behavior remain unchanged for the default path.

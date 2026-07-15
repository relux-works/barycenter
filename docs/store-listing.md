# Microsoft Store listing source (Pulsar Phase 1)

This is the current self-contained Phase 1 listing handoff for Product ID
`9P26FDCWV1GC`. The old Spotify-first draft has been retired. Exact copy-paste
fields live in the versioned package below:

- [`listing-en-US.json`](store/phase1/listing-en-US.json)
- [`listing-ru-RU.json`](store/phase1/listing-ru-RU.json)
- [`certification-notes.json`](store/phase1/certification-notes.json)
- [`iarc-answer-profile.json`](store/phase1/iarc-answer-profile.json)
- [`screenshots.json`](store/phase1/screenshots.json)
- [`partner-center-package.json`](store/phase1/partner-center-package.json)

The Store category is `Music`, the price is `Free`, and the listing languages
are `en-US` and `ru-RU`. The primary product is private short-audio recording,
routing and history. Spotify and Telegram are optional integrations, never a
prerequisite or the first listing claim. Phase 1 coordinator processing and
the absence of end-to-end encryption are explicit limitations.

The EN short description is:

> Record or choose short audio, route it inside a private Barycenter, and
> review delivery history. Accountless setup and local self-test are built in;
> Spotify and Telegram are optional.

The RU short description is:

> Записывайте или выбирайте короткое аудио, направляйте его в приватном
> Барицентре и смотрите историю доставки. Настройка без аккаунта и локальная
> проверка встроены; Spotify и Telegram необязательны.

The JSON files are authoritative because the validator applies the current
Partner Center character/count limits, approved locale URLs and shipped-claim
evidence. Validate the engineering package with:

```sh
cd coordinator
GOTOOLCHAIN=go1.25.12 go run ./cmd/store-listing-check
```

The submission gate is deliberately fail-closed:

```sh
GOTOOLCHAIN=go1.25.12 go run ./cmd/store-listing-check --require-ready
```

It will fail until the exact MSIX is frozen, twelve real localized screenshots
and their hashes exist, WACK evidence has been reviewed, Partner Center has
generated the IARC result, and Ivan Oparin records `proceed`. No repository or
CI result is represented as those manual/external observations.

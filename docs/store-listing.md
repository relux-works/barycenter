# Microsoft Store listing draft (Pulsar)

For Partner Center submission (goal v2.1 F6). The Store overwrites the MSIX
Identity/Publisher with the reserved app identity on submission.

- **Category:** Music
- **App name:** Pulsar
- **Price:** Free

## Short description (EN)

One music for two homes — or more. Pick a Pulsar speaker in Spotify and play:
the same track starts in sync at every connected home. Telegram adds pairing,
queues and voice notes between songs. Bring a Premium Spotify account per home.

## Short description (RU)

Одна музыка на несколько домов. Выбери колонку Пульсар в Spotify и включи
трек — он синхронно заиграет у всех подключённых домов. В Telegram остаются
подключение, очередь и голосовые между песнями. Свой Spotify Premium в каждом доме.

## Feature bullets

- Synchronized broadcast across homes (periastron), precise to a fraction of
  a second.
- Start together playback directly from Spotify on either Pulsar; no track
  link needs to be sent to the bot.
- Voice notes slotted between tracks, privately or to everyone.
- Approaches: link two barycenters into one shared air, part anytime.
- Zero configuration — a code from the bot pairs the app; no files, no setup.
- Verifiable builds: every release carries SLSA provenance attestation.

## Privacy

Pulsar collects no personal data. It stores only a pairing token (kept on
the device) to talk to the coordinator. Spotify account credentials never
leave your computer. Audio never travels between homes — only playback
commands and short voice-note files do.

## Support

Site & guide: https://barycenter.live/guide
Source: https://github.com/relux-works/barycenter

## Screenshot requirements (cert rejection 2026-07-09, policy 10.1.1.3)

The Russian listing's screenshots showed only the onboarding/code window and
failed certification as "splash/login only". Every listing language needs
>=1366x768 images of the product IN USE, e.g.:

1. Tray menu open: connection line, playing track, periastron mode.
2. Spotify device picker with "Pulsar A" selected — the phone-sees-speaker
   moment.
3. The Telegram bot chat: queue, /now, a voice message between tracks — the
   primary control surface.
4. The onboarding window at most as a trailing shot, never the only one.

Fix procedure: a failed submission becomes editable in Partner Center — swap
the images in RU and EN listings there and resubmit the same package. CLI
submissions clone the listing, so the fix persists automatically afterwards.

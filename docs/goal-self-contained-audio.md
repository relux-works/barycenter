# Goal: самодостаточный Pulsar Audio

Дата: 2026-07-12. Source of truth:
[`docs/spec-self-contained-audio.md`](spec-self-contained-audio.md).

## Результат

Реализовать спецификацию тремя последовательными поставляемыми фазами. Pulsar
должен передавать и синхронно воспроизводить голос и пользовательское аудио
между Барицентрами без обязательных Spotify и Telegram. Spotify остаётся
опциональным источником, Telegram — адаптером общих application services.

UX, API, protocol, данные, mixing, безопасность и acceptance определяет source
of truth; этот goal их не переопределяет.

## Фаза 1 — Store-ready clips

Выполнить §19 и доказать A1–A8:

- подтвердить microphone, toggle hotkey и file picker в Store
  MSIX/AppContainer;
- дать main UI с Create, Join и локальным Try locally;
- реализовать app actor, раздельные node/control credentials, recovery и
  invites без зависимости от Telegram;
- записывать/загружать clips и доставлять их себе, своему Барицентру или
  текущему approach;
- реализовать overlay, interrupt и after-current с prepare barrier, receipts,
  DND/block и ducking без остановки main timeline;
- перевести бота на общий ingest, сохранив legacy voice;
- закрыть privacy, UGC/reporting, RU/EN listing и Store certification без
  Spotify test account.

## Фаза 2 — Air и длинное аудио

После gate фазы 1 выполнить §20 и доказать B1–B7:

- codec/player spike выбирает лицензируемый Store-compatible bounded-memory
  streaming path;
- tracks получают queue/replace, pause/seek/resume и synchronized start без
  полной загрузки в RAM;
- Air объединяет 2..N Барицентров; approach мигрирует в compatibility alias без
  транзитивных цепей и дублей;
- explicit targets, ACL snapshots, receipts и offline inbox работают для N
  участников при совместимости production orbits и старых nodes.

## Фаза 3 — near-live PTT

После gate фазы 2 выполнить §21 и доказать C1–C7:

- hold-to-talk передаёт progressive audio с заданной latency и fallback toggle;
- общий для clips/self-test/PTT capture DSP с AEC/noise suppression/diagnostics
  проверен на Windows и macOS;
- client-owned E2EE защищает clips, tracks, saved cues и live PTT без silent
  downgrade и включается только после threat model, spikes, design/code review;
- durable soundboard и at-most-once scoped automation соблюдают DND, target ACL,
  recipient local volume ceiling, revoke и audit;
- `live_ptt`, `e2ee_media`, `soundboard_cues` и `automation` выпускаются
  независимыми flags и не получают claim без собственного evidence.

## Инварианты

- Фазы выполняются по порядку; частично готовая фаза не считается завершённой.
- Rollout: additive DB → coordinator → nodes → internal orbit → platform matrix
  → постепенное включение за feature flags.
- Production state и pairing сохраняются; migrations аддитивны.
- Protocol change включает golden JSON, Go/Swift codecs, Windows mirror и
  contract tests. Legacy `play_voice`, старые nodes и Spotify не ломаются.
- Render callback не получает I/O, allocation или blocking operations.
- Secrets/audio content не попадают в repo, logs, fixtures или screenshots.
- Нельзя молча ослаблять AppContainer, tenant ACL, DND, local volume ceiling или
  retention; отклонение требует документированного решения пользователя.
- Готовность подтверждается evidence, требуемым соответствующим A/B/C сценарием.
  Внешний gate не симулируется: выполнить доступную работу и назвать точный
  оставшийся шаг.
- Результат агента в `to-review` не принимается по self-report. Root-agent сам
  проводит построчное diff-review, сопоставляет реализацию с AC и source of
  truth, перечитывает затронутые security/protocol/migration/audio seams и
  повторно запускает релевантные плюс регрессионные проверки. Рискованные
  изменения дополнительно проходят независимый reviewer task. Только после
  этого задача может перейти в `done`.

## Завершение

Goal завершён только когда A1–A8, B1–B7 и C1–C7 имеют воспроизводимое evidence;
unit/integration/golden/migration/security/platform suites зелёные; проверены
Windows↔Windows, Windows↔macOS и macOS↔macOS; rollback сохраняет данные и legacy
operation; документация, policies, runbook и Store listing соответствуют коду.

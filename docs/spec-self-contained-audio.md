# Самодостаточный Pulsar: приватный аудиоэфир без обязательных Spotify и Telegram

| Поле | Значение |
|---|---|
| Версия | 0.2 |
| Дата | 2026-07-12 |
| Статус | Утверждённое продуктовое направление; реализация не начата |
| Область | Windows и macOS Pulsar, Barycenter coordinator, Telegram-адаптер, Microsoft Store |
| Базовые документы | `docs/spec.md`, `docs/v2-multitenant-design.md`, `docs/goal-v2.1.md`, `docs/idea-air-rooms.md` |
| Агентский goal | `docs/goal-self-contained-audio.md` |

## 0. Статус и сила документа

Этот документ расширяет существующий музыкальный Barycenter самостоятельным
аудиоканалом. Он не меняет поведение продакшена до реализации соответствующей
фазы.

Продуктовые решения, принятые здесь:

1. Spotify перестаёт быть обязательным условием работы Pulsar и становится
   одним из опциональных источников аудио.
2. Telegram перестаёт быть обязательным интерфейсом. Бот остаётся полноценным
   дополнительным пультом над теми же командами домена.
3. Пользователь может создать Барицентр, подключить Pulsar, записать голос и
   воспроизвести его без аккаунта Spotify и без Telegram.
4. Короткие сообщения и длинные аудиофайлы — разные типы медиа с разными
   трактами воспроизведения.
5. Немедленная голосовая передача может звучать поверх текущей музыки с
   ducking, не останавливая и не рассинхронизируя основной эфир.
6. Парные `approach` остаются совместимым текущим механизмом в фазе 1. В фазе 2
   они мигрируют в явные комнаты Air из 2..N Барицентров.
7. Реальный потоковый push-to-talk, AEC и end-to-end encryption относятся к
   фазе 3, а не маскируются под обычную загрузку готового голосового файла.

Теги в документе:

- **[ТРЕБОВАНИЕ]** — обязательное поведение.
- **[РЕШЕНИЕ]** — выбранная архитектура или продуктовый дефолт.
- **[SPIKE]** — выбор должен быть подтверждён экспериментом до реализации
  зависимой части.

## 1. Определение продукта

### 1.1 Позиционирование

Короткая формулировка:

> Pulsar соединяет несколько мест в приватный аудиоэфир. Говорите по горячей
> клавише, отправляйте записи и собственное аудио, слушайте его во всех
> сближенных Барицентрах. Spotify Connect — дополнительный источник музыки.

Store-формулировка:

> Приватный аудиоканал между домами: голосовые объявления, записи и
> синхронное воспроизведение пользовательского аудио. Spotify Connect доступен
> как опциональная интеграция.

Продукт больше не описывается как «обвязка над Spotify». Его ядро — доставка,
планирование и синхронное воспроизведение аудио между доверенными местами.

### 1.2 Основная ценность

- Один жест превращает компьютер в push-to-talk точку для близких людей.
- Сообщение может прозвучать немедленно, дождаться конца трека либо стать
  самостоятельным элементом эфира.
- Один и тот же сценарий доступен из Pulsar и Telegram.
- Пользователь видит, куда сообщение ушло, кто был онлайн и где оно прозвучало.
- Музыкальные провайдеры дополняют эфир, но не владеют onboarding и не блокируют
  тестирование продукта.

### 1.3 Не цели

В рамках трёх фаз не строятся:

- публичная социальная сеть или каталог открытых комнат;
- магазин музыки или средство обхода DRM;
- полноценный телефонный/видеозвонок с открытым микрофоном;
- публичное радио для неограниченной аудитории;
- автоматическая AI-модерация содержимого;
- peer-to-peer работа между домами без coordinator;
- мобильное нативное приложение — Telegram остаётся мобильным пультом до
  отдельного решения.

## 2. Словарь и доменная модель

| Термин | Значение |
|---|---|
| **Барицентр / Orbit** | Постоянное приватное пространство: участники, Pulsar-ноды, настройки и история |
| **Pulsar / Slot** | Конкретная аудиоточка на компьютере, подключённая к Барицентру |
| **Air** | Временный общий эфир из 2..N Барицентров; в фазе 1 его роль выполняет текущий pairwise approach |
| **Actor** | Источник пользовательского действия: установка Pulsar или Telegram-пользователь |
| **Media item** | Загруженное или записанное аудио с метаданными и политикой хранения |
| **Clip** | Короткая запись, целиком подготавливаемая до воспроизведения |
| **Track** | Длинное аудио, воспроизводимое потоково с pause/seek/resume |
| **Transmission** | Намерение доставить media item выбранной аудитории определённым способом |
| **Main program** | Основной музыкальный/файловый эфир и его очередь |
| **Overlay** | Короткая вставка поверх main program без остановки его временной шкалы |

### 2.1 Типы media item

| `kind` | Назначение | Тракт |
|---|---|---|
| `voice_clip` | Микрофон, Telegram voice, короткий речевой файл | Полная подготовка и вставка |
| `audio_clip` | Короткий сигнал, запись, джингл | Полная подготовка и вставка |
| `audio_track` | Длинная запись или собственный музыкальный файл | Потоковый player, фаза 2 |
| `builtin_cue` | Лицензированный встроенный тестовый звук | Локально или как clip |

### 2.2 Способы доставки

| `delivery` | Пользовательское имя | Семантика |
|---|---|---|
| `overlay` | Говорить сейчас | Начать поверх main program; музыка продолжает идти и приглушается |
| `interrupt` | Прервать и продолжить | Плавно остановить main program, проиграть clip, продолжить с прежней позиции |
| `after_current` | После текущего | Поставить clip первым после текущего элемента |
| `queue` | В очередь | Добавить track в хвост основной очереди |
| `replace` | Играть сейчас | Завершить/припарковать текущий main program и начать track |

`overlay`, `interrupt` и `after_current` допустимы только для clip. `queue` и
`replace` — только для track. Клиент не должен предлагать бессмысленные
комбинации.

### 2.3 Аудитория

| `audience` | Значение |
|---|---|
| `this_pulsar` | Только текущая нода; self-test и локальная проверка |
| `own_barycenter` | Все разрешённые ноды собственного Барицентра |
| `current_air` | Все разрешённые ноды текущего approach/Air |
| `explicit` | Явный набор Барицентров или Pulsar-нод |

**[РЕШЕНИЕ]** Аудитория разрешается в явный список получателей в момент
принятия transmission координатором. Позднее присоединение к Air не меняет уже
отправленное сообщение.

Для микрофона originating Pulsar по умолчанию исключается из `current_air`,
чтобы пользователь не слышал собственный голос с задержкой. Для файла
originating Pulsar включён. В UI всегда есть явный переключатель «проиграть
также здесь».

## 3. Инварианты

1. Spotify credentials никогда не нужны для clip playback.
2. Отказ или отсутствие go-librespot не мешает локальному тесту, загрузке и
   воспроизведению clip.
3. Telegram transport не содержит уникальной бизнес-логики; он вызывает тот
   же application service, что и Pulsar.
4. Удалённая команда не может поднять локальную master-громкость выше
   установленного пользователем предела и не может обойти DND.
5. Микрофон не записывается без явного текущего действия пользователя.
6. Во время overlay основной музыкальный буфер продолжает потребляться:
   временная шкала main program не останавливается и не переполняется.
7. Два media overlay не звучат одновременно в одном Air. Они сериализуются по
   времени принятия coordinator.
8. Старые `play_voice` и Telegram voice сохраняют поведение при смешанном
   парке версий.
9. Все новые protocol messages добавляются вместе с golden JSON, Go codec,
   Swift codec и Windows mirror tests.
10. Миграции базы только аддитивны до отдельного подтверждённого cleanup-релиза.
11. Секреты, содержимое записей и локальные пути не попадают в обычные логи.
12. Пользовательское аудио доступно только участникам разрешённой аудитории.
13. Результат implementation-агента не принимается по self-report: root-agent
    сам читает весь diff построчно, сопоставляет каждый hunk с этой спецификацией
    и acceptance criteria и повторно запускает релевантные проверки. Crypto,
    realtime audio, privacy/Store, automation и migration/recovery дополнительно
    проходят независимое review; проверяется тот же hash, который идёт в beta.

## 4. Пользовательские сценарии

### 4.1 Первый запуск без внешних аккаунтов

Первый экран предлагает три равноправных действия:

1. **Создать Барицентр** — создать приватное пространство и сделать эту
   установку primary actor.
2. **Подключиться к существующему** — ввести invite/pair code либо открыть
   deep link.
3. **Попробовать локально** — проверить микрофон и аудиовыход без сети и без
   сохранения записи на сервере.

После создания Барицентра пользователь сразу попадает в основной экран. Pulsar
уже может записывать и воспроизводить audio clip на этом компьютере. Экран
предлагает пригласить другое место или связать Telegram, но не блокирует этим
основную функцию.

### 4.2 Локальный self-test

Self-test работает до pairing и без coordinator:

1. Пользователь выбирает вход и выход.
2. Нажимает «Записать 5 секунд» либо «Воспроизвести тестовый звук».
3. Запись хранится только во временном локальном файле.
4. Пользователь слышит результат через тот же mixer/output, который использует
   сетевой clip.
5. Временный файл удаляется при закрытии или явной команде.

Встроенный звук должен быть создан Relux Works либо иметь документированную
лицензию для redistributable использования.

### 4.3 Голос по горячей клавише

Фаза 1 использует toggle-модель:

1. `Ctrl+Shift+Space` начинает запись.
2. Pulsar показывает постоянно видимый красный индикатор, текст «Идёт запись»
   и проигрывает короткий start cue.
3. Повторное нажатие завершает запись и отправляет её с последними выбранными
   audience/delivery.
4. `Esc` отменяет запись и удаляет временный файл.
5. При достижении лимита запись останавливается автоматически, а пользователь
   получает явное уведомление.

Горячая клавиша настраивается. Если комбинацию заняло другое приложение,
Pulsar остаётся рабочим через кнопку и предлагает выбрать другую комбинацию.

Настоящий hold-to-talk появляется в фазе 3 после отдельного AppContainer spike.

### 4.4 Отправка короткого файла

Пользователь выбирает файл стандартным системным picker либо перетаскивает его
в окно. Pulsar показывает:

- имя и определённый формат;
- длительность;
- размер;
- audience;
- допустимые способы доставки;
- короткое напоминание, что отправлять можно только аудио, на которое у
  пользователя есть права.

В фазе 1 файл длиннее clip limit не загружается как WAV: UI объясняет, что
потоковые длинные записи появятся в фазе 2.

### 4.5 Отправка длинного файла

Начиная с фазы 2 пользователь выбирает `Играть сейчас` или `В очередь`.
Файл становится main program, получает progress, pause/seek/resume и не
загружается целиком в RAM.

### 4.6 Telegram

Бот принимает:

- voice message → `voice_clip`;
- audio attachment → `audio_clip` или `audio_track` по длительности;
- audio document с проверяемым MIME/signature → соответствующий media kind.

После обработки бот показывает inline actions:

- `Сейчас поверх`;
- `Прервать`;
- `После текущего`;
- `В очередь` — только для track;
- выбор audience, если доступно больше одной.

Прежнее голосовое без дополнительных действий сохраняет дефолт
`after_current` и текущую personal/broadcast политику.

### 4.7 История и входящие

Точные Phase 1 routes, authorization, pagination, DND/block ownership и
Telegram callback/default-enqueue races заморожены в
[`p1-history-presence-telegram-contract-v1`](analysis/p1-history-presence-telegram-contract-v1.md).

Основной экран показывает последние transmissions:

- отправитель и исходный Барицентр;
- название/тип и длительность;
- audience и delivery;
- `обрабатывается`, `готово`, `проигрывается`, `проиграно`, `частично`,
  `истекло`, `ошибка`;
- количество готовых/проигравших получателей;
- replay, delete, report, mute sender.

Фаза 1 хранит историю online-доставки. Полноценный offline inbox с правилами
позднего воспроизведения появляется в фазе 2.

## 5. Основной UI Pulsar

### 5.1 Главное окно

Минимальная структура:

1. **Header**: Барицентр, текущий Air/approach, connection state.
2. **Presence**: online/offline/DND для доступных Барицентров и Pulsar-нод.
3. **Action area**: большая кнопка записи, level meter, file picker/drop zone.
4. **Routing**: audience, delivery, «проиграть также здесь».
5. **Now playing**: main program и активный overlay.
6. **History/queue**: последние transmissions и основная очередь.
7. **Local controls**: input, output, master ceiling, DND, hotkey.
8. **Integrations**: Spotify и Telegram как необязательные подключения.

### 5.2 Tray/menu bar

Tray остаётся быстрым интерфейсом:

- открыть Pulsar;
- начать/закончить запись;
- connection/presence summary;
- DND toggle;
- текущий main program/overlay;
- выбранные input/output;
- hotkey;
- настройки, re-pair, quit.

Tray больше не должен создавать впечатление, что приложение умеет только
подключить Spotify.

### 5.3 Доступность и локализация

- RU и EN интерфейсы поставляются одновременно до повторной Store submission.
- Все действия доступны клавиатурой.
- Recording/DND/error не кодируются только цветом.
- Кнопки и level meter имеют screen-reader labels.
- UI корректен при 125%, 150% и 200% Windows scaling.
- Уведомления не крадут focus во время записи.

## 6. Самостоятельная identity и onboarding

### 6.1 Разделение полномочий

**[РЕШЕНИЕ]** Установка Pulsar имеет две разные capability identities:

| Секрет | Назначение |
|---|---|
| `node_token` | WebSocket playback node, heartbeat, scoped media download |
| `control_token` | Пользовательские команды, upload, invite/admin в рамках роли |

`node_token` не даёт административных прав. Оба секрета генерируются как
минимум из 256 бит случайности, на сервере хранятся только hash, на Windows —
через DPAPI/Credential Locker, на macOS — Keychain.

### 6.2 Actor model

Actor отделяет роль человека от транспорта:

```text
actor
  kind = app_installation | telegram_user
  display_name
  external_ref / token_hash

membership
  orbit_id
  actor_id
  role = primary | companion | satellite
```

Существующие Telegram members мигрируют в actors без изменения ролей. Команды
бота и приложения после авторизации получают одинаковый `ActorContext`.

### 6.3 Создание Барицентра

`Create Barycenter` одной транзакцией создаёт:

- orbit;
- app installation actor с ролью primary;
- первый slot;
- node/control credentials;
- recovery secret, показываемый один раз;
- audit event.

Никаких username/password, email или Telegram на этом пути нет.

Защита от массового создания в фазе 1:

- rate limit по IP и installation attempt;
- не более 5 созданий в час с одного IP по умолчанию;
- audit/alert при аномальном росте;
- отсутствие публичного discovery;
- возможность server-side disable actor/orbit.

### 6.4 Подключение другой установки

Primary/companion с правом invite создаёт короткоживущий device invite. Новый
Pulsar вводит код или открывает deep link, получает actor membership и slot.

Существующий Telegram `/pair` остаётся совместимым. В UI это один экран
`Подключиться к существующему`, а не Telegram-специфическая история.

### 6.5 Связь с Telegram

Приложение генерирует одноразовый `link bot` code. Пользователь отправляет его
боту, после чего Telegram actor присоединяется к существующему membership либо
создаётся как companion/satellite по выбранной политике. Бот не становится
владельцем identity Pulsar.

### 6.6 Recovery

- Recovery secret содержит не менее 128 бит энтропии и показывается один раз.
- Сервер хранит только hash.
- Recovery может перевыпустить control credential, но не восстанавливает
  старый node token.
- Любой другой primary может отозвать потерянную установку и пригласить новую.
- Потеря единственной установки и recovery secret считается невосстановимой;
  это ясно объясняется при создании.

## 7. Захват и локальная подготовка

### 7.1 Microphone capture

Фаза 1:

- системный default capture device с явным picker;
- mono PCM capture с последующей серверной нормализацией;
- локальный level meter без отправки samples;
- hard limit 180 секунд по умолчанию;
- upload limit 50 MiB;
- временный файл в app-private storage;
- запись удаляется после подтверждённой загрузки или отмены;
- при recording локальный main program приглушается, чтобы уменьшить bleed;
- AEC/noise suppression не обещаются до фазы 3.

Разрешение микрофона запрашивается в момент первого явного нажатия Record, а не
при запуске приложения.

### 7.2 File input

Фаза 1 принимает как clip:

- WAV;
- MP3;
- M4A/AAC;
- OGG/Opus;
- FLAC.

Расширение и клиентский MIME не считаются доказательством формата. Авторитетны
server-side signature probe и `ffprobe`.

### 7.3 Hotkey

Windows использует системную регистрацию горячей клавиши. Phase 1 toggle не
зависит от события key-up. Регистрация и освобождение комбинации происходят в
том же UI thread/message loop, который владеет tray.

**[SPIKE P3-HOTKEY]** До настоящего hold-to-talk проверить low-level hook/raw
input в Store MSIX с текущим `TrustLevel="appContainer"`, поведение при lock
screen, UAC desktop, Remote Desktop и конфликте с accessibility tools.

## 8. Media ingest и хранение

### 8.1 Общий ingest service

Входы приложения и Telegram сходятся в один application service:

```text
SubmitMedia(actor, source, input, requested_kind)
  -> validate
  -> persist processing record
  -> normalize/transcode
  -> mark ready
  -> CreateTransmission(...)
```

Telegram transport больше не управляет очередью напрямую после получения
готового файла.

### 8.2 Phase 1 clip pipeline

1. Клиент создаёт upload session.
2. Загружает файл multipart/streaming upload с control token.
3. Coordinator проверяет quota, declared size и magic bytes.
4. `ffprobe` проверяет тип и длительность с timeout.
5. `ffmpeg` применяет существующий speech chain:
   high-pass, compressor, loudnorm `I=-14`, `TP=-1.5`, `LRA=11`.
6. Canonical phase-1 output: PCM s16le, 44.1 kHz, stereo WAV — совместим с
   текущими macOS/Windows voice players.
7. Сохраняются duration, measured loudness, true peak, hash и size.
8. Исходный upload удаляется после успешной подготовки.

Ошибки не оставляют partially-ready media. Повторный запрос с тем же idempotency
key возвращает исходный результат.

### 8.3 Почему длинный файл не является clip

PCM s16le 44.1 kHz stereo занимает около 176.4 KB/s:

- 3 минуты — около 32 MB на каждого скачивающего получателя;
- 1 час — около 635 MB;
- текущий Windows decode в interleaved float32 потребовал бы около 1.27 GB RAM
  на час.

Поэтому увеличение `max_voice_s` не является реализацией long audio. Phase 2
обязана иметь сжатое хранение, range/chunk fetch и bounded-memory decoder.

### 8.4 Retention

Дефолты:

| Данные | Retention |
|---|---|
| Неуспешный upload | не более 24 часов |
| Готовый clip | 7 дней |
| Transmission metadata/history | 30 дней |
| Reported content | до 30 дней для review, с ограниченным доступом |
| Audit без содержимого | 90 дней |

Пользователь может удалить собственный media раньше. Delete немедленно
запрещает новые downloads, отменяет pending transmissions и ставит bytes в
очередь физического удаления. Backups следуют опубликованной backup retention;
это честно описывается в privacy policy.

### 8.5 Quotas и защита

Phase 1 defaults на actor/orbit:

- 10 upload starts в минуту;
- не более 3 одновременно обрабатываемых файлов;
- 1 GiB новых clip bytes в сутки на orbit;
- 180 секунд и 50 MiB на clip;
- ffmpeg/ffprobe timeout и memory/CPU limits контейнера;
- file names никогда не используются как server path;
- content hash deduplication только внутри одного orbit, чтобы не создавать
  cross-tenant existence oracle.

## 9. Transmission scheduler

### 9.1 Отдельный overlay controller

**[РЕШЕНИЕ]** `overlay` не становится новым состоянием основной Session FSM.
Он живёт в отдельном `OverlayController` на Air/orbit:

- main program продолжает `PLAYING` и обновляет позицию;
- overlay имеет собственные prepare/ready/start/end события;
- следующий overlay ждёт окончания предыдущего;
- pause/skip main program не должны ошибочно завершать overlay;
- `/apart`/leave Air отменяет ещё не начавшиеся transmissions для покинувшей
  стороны.

`after_current` продолжает использовать существующий `KindVoice` и
`EnqueueVoice` до общей миграции media elements.

### 9.2 Prepare barrier

Для синхронного `overlay`/`interrupt`:

1. Coordinator отправляет `prepare_media` всем online eligible targets.
2. Нода скачивает, проверяет hash, декодирует/открывает файл и отвечает
   `media_ready`.
3. Coordinator ждёт ready deadline, по умолчанию 3 секунды после готовности
   media на сервере.
4. Если готов хотя бы один target, выбирается
   `T = now + max(2*maxRTT + 250ms, 500ms)`.
5. Ready targets получают `play_media_at(T)`.
6. Неуспевшие targets получают receipt `missed_not_ready` и не начинают clip
   с середины. В фазе 2 он доступен им в inbox.
7. Если не готов никто, transmission получает `failed` с понятной ошибкой.

Target start skew после barrier: не более 100 ms в нормальной сети.

### 9.3 Ordering

- Порядок — coordinator acceptance time, tie-breaker ULID.
- Параллельная обработка файлов не меняет порядок принятых transmissions.
- Новый overlay во время overlay встаёт следующим в overlay FIFO.
- Overlay длиннее 60 секунд по умолчанию не принимается: UI предлагает
  `interrupt` или `after_current`.
- `interrupt` также сериализуется с overlay; два голоса не микшируются между
  собой.

### 9.4 Offline semantics

| Delivery | Offline target в фазе 1 | Фаза 2 |
|---|---|---|
| `overlay` | Не проигрывать поздно; receipt `missed_offline` | Положить в inbox, только ручной replay |
| `interrupt` | Не проигрывать поздно | Inbox, только ручной replay |
| `after_current` | Следует существующей persistent queue политике | Явный TTL и inbox |
| `queue/replace` | Не поддерживается | Persistent main queue |

Никакое «говорить сейчас» не должно внезапно автоматически прозвучать через
несколько часов после восстановления компьютера.

## 10. Audio mixer

### 10.1 Overlay

Формула до master gain:

```text
output = limiter(main_program * duck_gain + overlay * overlay_gain + cues)
```

Требования:

- main program ring/stream читается непрерывно;
- duck target по умолчанию `-12 dB`;
- attack `250 ms`, release `600 ms`;
- coordinator планирует pre-duck до первого speech sample;
- при паузах речи sidechain может частично возвращать музыку, не создавая
  pumping;
- overlay input нормализован к `-14 LUFS`, `TP <= -1.5 dBTP`;
- post-mix limiter не допускает clipping;
- master gain и local volume ceiling применяются последними;
- при отсутствии main program overlay звучит с обычным gain;
- limiter/duck не выполняют allocation, I/O или blocking locks в render
  callback.

Windows `Engine.Render` меняется с `voice REPLACES music` на две независимые
ветки. macOS сохраняет `AVAudioSourceNode + AVAudioPlayerNode`, но получает
явный sidechain/duck controller и одинаковые параметры.

### 10.2 Interrupt and resume

1. Main program fade-out 250 ms.
2. Фиксируется audible position.
3. Провайдер ставится на pause; buffered tail обрабатывается без скачка.
4. Clip проигрывается через media branch.
5. Main program восстанавливается с сохранённой позиции и fade-in 120 ms.

Если текущий source не поддерживает точный pause/resume, его adapter обязан
вернуть capability error. Policy fallback: `overlay`, затем `after_current` —
только после явного подтверждения пользователя, не молча.

### 10.3 Local recording bleed

В фазе 1 Pulsar приглушает собственный main program во время microphone
capture. Это уменьшает, но не устраняет echo. UI не обещает echo cancellation.
Полноценные AEC/noise suppression входят в фазу 3.

## 11. API и протокол

Все имена ниже логические; точные URL могут быть адаптированы к существующему
router, но семантика обязательна.

Точный нормативный контракт Phase 1 для HTTP, WebSocket, scheduler, receipts,
DND, downgrade, cancel и delete зафиксирован в
[`docs/analysis/p1-transmission-contract-v1.md`](analysis/p1-transmission-contract-v1.md).
Примеры ниже остаются обзором и не переопределяют этот контракт.

### 11.1 HTTP API

| Method | Path | Auth | Назначение |
|---|---|---|---|
| `POST` | `/v1/onboarding/orbits` | bootstrap/rate limited | Создать orbit + actor + slot |
| `POST` | `/v1/device-invites` | control token | Создать invite другой установки |
| `POST` | `/v1/device-invites/consume` | invite code | Присоединить установку |
| `POST` | `/v1/telegram-links` | control token | Создать bot link code |
| `POST` | `/v1/media/uploads` | control token | Создать upload session |
| `PUT` | `/v1/media/uploads/{id}` | scoped upload token | Передать bytes |
| `GET` | `/v1/media/{id}` | node/control token + audience ACL | Скачать canonical media |
| `DELETE` | `/v1/media/{id}` | owner/control policy | Удалить media |
| `POST` | `/v1/transmissions` | control token | Создать доставку |
| `GET` | `/v1/transmissions/{id}` | audience member | Status/receipts |
| `POST` | `/v1/transmissions/{id}/cancel` | sender/admin | Отменить pending transmission |
| `POST` | `/v1/reports` | control token | Report inappropriate media |

Upload session выдаёт короткоживущий scoped token и поддерживает idempotency
key. Long-lived control token не кладётся в query string.

### 11.2 WebSocket additions

```json
{
  "type": "prepare_media",
  "payload": {
    "transmission_id": "tr_...",
    "media_id": "md_...",
    "kind": "voice_clip",
    "file_url": "https://...",
    "sha256": "...",
    "duration_ms": 4200
  }
}
```

```json
{
  "type": "media_ready",
  "payload": {
    "transmission_id": "tr_...",
    "decoded_duration_ms": 4200
  }
}
```

```json
{
  "type": "play_media_at",
  "payload": {
    "transmission_id": "tr_...",
    "t_coord_ms": 1780000000000,
    "delivery": "overlay",
    "duck_db": -12,
    "attack_ms": 250,
    "release_ms": 600
  }
}
```

Новые client events:

- `media_started`;
- `media_ended`;
- `media_failed`;
- `media_cancelled`;
- `set_dnd`/presence update;
- фаза 2: `stream_load`, `stream_ready`, `stream_seek`;
- фаза 3: live stream signalling/chunks.

### 11.3 Capability negotiation

Register/welcome объявляют capabilities:

- `media_clip_v1`;
- `overlay_mix_v1`;
- `interrupt_resume_v1`;
- `stream_track_v1` — фаза 2;
- `air_v1` — фаза 2;
- `live_ptt_v1` — фаза 3;
- `e2ee_media_v1` — фаза 3.

Coordinator использует overlay только если все выбранные mandatory targets его
поддерживают. При смешанном парке дефолт — legacy `after_current`; downgrade
отображается отправителю.

### 11.4 Legacy mapping

- Старый Telegram voice продолжает создавать legacy media + `KindVoice`.
- Новый ingest может создать compatibility WAV и legacy `play_voice` для
  старой ноды.
- Новая нода понимает и legacy `play_voice`, и новые media messages.
- Удаление legacy protocol разрешено только отдельной будущей версией после
  подтверждённого отсутствия старых нод.

## 12. Данные

Логическая схема; миграции сохраняют существующие таблицы до cleanup:

```text
actors(
  id, kind, display_name, external_ref, control_token_hash,
  created_at, revoked_at
)

memberships(
  orbit_id, actor_id, role, joined_at, left_at
)

media_items(
  id, owner_orbit_id, actor_id, kind, source, title,
  mime, codec, duration_ms, size_bytes, sha256,
  storage_key, loudness_json, status,
  created_at, expires_at, deleted_at
)

transmissions(
  id, media_id, source_orbit_id, actor_id, air_id,
  audience_kind, delivery, include_origin,
  accepted_at, expires_at, status
)

transmission_targets(
  transmission_id, orbit_id, slot,
  status, ready_at, started_at, ended_at, error_code
)

blocks(
  owner_actor_id, blocked_actor_id, blocked_orbit_id, created_at
)

reports(
  id, reporter_actor_id, media_id, reason, details,
  status, created_at, reviewed_at
)
```

Фаза 2 добавляет:

```text
airs(id, title, created_by_actor_id, status, max_barycenters, created_at)
air_members(air_id, orbit_id, role, status, joined_at, left_at)
stream_variants(media_id, codec, bitrate, storage_key, size_bytes)
```

Target rows являются также ACL snapshot для media download. Одного знания
`media_id` недостаточно.

## 13. Air: несколько Барицентров

Точный нормативный контракт Phase 2 для lifecycle, saved membership,
one-active pointer, invite/confirmation, policy, Telegram aliases и
single-authority cutover/rollback зафиксирован в
[`p2-air-lifecycle-policy-contract-v1`](analysis/p2-air-lifecycle-policy-contract-v1.md).
Обзор ниже не переопределяет этот контракт.

### 13.1 Модель

В фазе 2 `Air` заменяет pairwise link как runtime-сущность общего эфира:

```text
Air
  ├── Barycenter A
  ├── Barycenter B
  └── Barycenter C
```

- Один Барицентр может состоять в нескольких сохранённых Air, но иметь только
  один активный общий эфир одновременно.
- В Air входят Барицентры, не отдельные Telegram users.
- Любой Барицентр может покинуть Air, не разрушая его для остальных.
- Если остаётся меньше двух Барицентров, Air паркуется; GC policy применяется
  позднее.
- Первая версия: до 8 Барицентров или 20 online Pulsars.
- Join требует invite и подтверждения primary присоединяющегося Барицентра.

### 13.2 Совместимость с approach

- Двухсторонний `/approach` создаёт Air из двух Барицентров.
- `/apart` означает leave текущего Air для вызывающего Барицентра.
- Активные pairwise links мигрируют транзакционно в двухчленные Air.
- Нет транзитивного распространения A—B—C через цепь links.
- Group session key переходит с отрицательного link id на air id.

### 13.3 Права

Air имеет явные policies:

- кто может приглашать Барицентры;
- кто может отправлять overlay;
- кто может ставить track в main queue;
- можно ли участникам использовать `replace`;
- локальные DND/block всегда сильнее Air policy.

## 14. Presence, DND, receipts и inbox

### 14.1 Presence

Для каждого target показываются только полезные статусы:

- online/offline;
- audio output ready/degraded;
- DND;
- поддержка требуемого media capability;
- current playback state.

Mic state и названия локальных процессов не публикуются.

### 14.2 DND

Локальные режимы:

- `allow_all`;
- `messages_only` — ничего не autoplay, только inbox;
- `muted_until(timestamp)`;
- block конкретного actor/orbit.

В первых трёх фазах нет удалённого «emergency bypass». Он слишком легко
ломает доверие и безопасность.

### 14.3 Receipts

Target state machine:

```text
accepted -> preparing -> ready -> scheduled -> playing -> played
                      \-> failed
accepted -> missed_offline | missed_dnd | missed_not_ready | blocked
```

Отправитель видит агрегат и детали только в рамках разрешённой аудитории.

### 14.4 Inbox

Фаза 2:

- missed overlay/interrupt виден получателю до media expiry;
- никогда не autoplay после долгого offline;
- replay создаёт локальное или targeted новое воспроизведение;
- delete by sender снимает item из inbox, если политика/retention позволяет;
- DND messages переходят в inbox с причиной.

## 15. Privacy, безопасность и Store compliance

### 15.1 Microphone privacy

- В MSIX объявляется `DeviceCapability Name="microphone"`.
- В macOS bundle добавляется `NSMicrophoneUsageDescription`; TCC permission
  также запрашивается только из явного Record action.
- Permission запрашивается при первом Record.
- Отказ не блокирует file playback и local builtin test.
- Во время capture есть визуальный и звуковой индикатор.
- Нет скрытого always-on pre-roll или фоновой записи.
- Privacy policy описывает capture, upload, processing и retention.

### 15.2 User-generated content

Пользовательское аудио считается UGC. До Store submission обязательны:

- Terms of Service / Content Guidelines в приложении или на сайте;
- in-app `Report` у каждого доступного чужого media item;
- block/mute sender;
- административная возможность disable/delete media и actor;
- documented moderation mailbox/runbook;
- возможность выполнить запрос Microsoft на удаление/disable;
- корректный IARC questionnaire для user communication/UGC.

Для file upload показывается правило: пользователь имеет право передавать этот
контент. Pulsar не позиционируется как средство публичной ретрансляции
коммерческой музыки.

### 15.3 Tenant isolation

- Media GET проверяет node/control token и target ACL.
- Air membership само по себе не даёт доступ к старому media, если orbit не
  был target конкретного transmission.
- После leave новые transmissions недоступны; уже полученные clips следуют
  history/retention policy.
- URL не содержит bearer secret.
- Download поддерживает revocation без смены media id.

### 15.4 Store testability

Certification reviewer без Telegram и Spotify должен выполнить:

1. Launch.
2. `Try locally` → builtin cue и microphone loopback.
3. `Create Barycenter` без username/password.
4. Record clip.
5. Target `This Pulsar`.
6. Услышать clip и увидеть played receipt.
7. Открыть main window/history/settings.

Certification notes прямо говорят, что внешняя учётная запись не требуется.
Spotify тестируется только как optional integration и не является блокирующей
частью primary functionality.

Store screenshots показывают main window, запись, routing, history и optional
Spotify integration, а не только login/onboarding.

### 15.5 Reporting operations

Минимальный административный workflow:

1. Report создаёт immutable audit entry.
2. Support видит reporter, media metadata, target scope и разрешённую evidence
   copy.
3. Решение: no action, delete media, disable actor, disable orbit.
4. Reporter получает status без раскрытия чужих персональных данных.
5. Все действия журналируются.

## 16. Ошибки и ожидаемое поведение

| Ошибка | Поведение |
|---|---|
| Microphone denied | Понятная инструкция открыть Windows Settings; file и builtin cue работают |
| Нет capture device | Record disabled, picker/diagnostics; остальные функции работают |
| Hotkey conflict | Кнопка работает, предложить другую комбинацию |
| Coordinator offline во время записи | Сохранить локальный draft до явного retry/delete; не утверждать «отправлено» |
| Upload оборвался | Resume/retry по upload session; idempotency исключает дубли |
| Unsupported/corrupt media | Не создавать transmission; назвать формат/причину |
| ffmpeg timeout | Status failed, bytes по retention failed-upload |
| Target offline/DND | Receipt; не проигрывать поздно автоматически |
| Часть targets не ready | Запустить на ready subset и показать partial delivery |
| Все targets не ready | Failed; предложить retry/inbox, не терять media |
| Mixer overload/underrun | Telemetry + local degraded state; main program не должен зависнуть |
| Leave Air во время prepare | Удалить target до schedule |
| Leave Air во время playback | Локально stop/fade transmission для ушедшего Барицентра |
| Sender deletes active clip | Завершить уже начатое или fade-stop согласно policy; новые fetch запрещены |

## 17. Наблюдаемость

Метрики без содержимого:

- upload bytes/duration/failures;
- processing latency и ffmpeg failures;
- release-to-ready и release-to-audible;
- ready deadline misses;
- start skew max/p95;
- overlay duration и queue depth;
- DND/offline/blocked delivery counts;
- mixer underruns, limiter hit rate, ring fill;
- stream buffer/seek metrics в фазе 2;
- live mouth-to-ear latency в фазе 3.

Structured events используют media/transmission ids, но не original filename,
caption, transcript, audio bytes, bearer tokens или локальный filesystem path.

Health checks отдельно показывают media processor readiness и storage free
space; обычный `/healthz` не должен быть green при неработающем обязательном
clip processor.

## 18. Rollout и совместимость

Feature flags:

- `self_service_onboarding`;
- `app_media_upload`;
- `overlay_media`;
- `interrupt_media`;
- `air_rooms`;
- `streamed_tracks`;
- `live_ptt`;
- `e2ee_media`;
- `soundboard_cues`;
- `automation`.

Порядок rollout каждой фазы:

1. Аддитивная DB migration.
2. Coordinator принимает, но не отправляет новые protocol messages.
3. Node release с capability и legacy support.
4. Включение на internal test orbit.
5. Live matrix Windows↔Windows, Windows↔macOS, macOS↔macOS.
6. Постепенное включение по orbit.
7. Store/public rollout после telemetry review.

Rollback coordinator не должен уничтожать новые rows. Старый coordinator может
игнорировать unknown data, а feature flag выключает новые commands.

## 19. Фаза 1 — Store-ready self-contained clips

### 19.1 Цель

Pulsar имеет полезную, полностью тестируемую primary functionality без Spotify
и Telegram: создать Барицентр, проверить звук, записать/загрузить короткий clip,
отправить его себе или в текущий approach и проиграть поверх/вместо/после музыки.

### 19.2 Scope

#### P1.0 Обязательный Windows platform spike

До изменения доменной модели минимальный подписанный MSIX с текущими
`TrustLevel="appContainer"` и `RuntimeBehavior="packagedClassicApp"` должен на
реальной Windows 10/11 доказать:

- системный microphone permission prompt и WASAPI/выбранный capture API;
- capture с default и выбранного input device;
- toggle `RegisterHotKey` из tray message loop;
- стандартный file picker без broad filesystem capability;
- продолжение активной записи при скрытом main window;
- корректную остановку capture при quit, suspend/session lock и revoke
  permission.

Если текущий Go audio stack не может легально получить capture в AppContainer,
spike выбирает WinRT/Media Foundation bridge. Ослабление sandbox или переход на
`runFullTrust` не делается молча: это отдельное решение с Store/security review.

#### P1.1 Main UI и onboarding

- RU/EN main window по §5.
- `Create`, `Join`, `Try locally`.
- Local microphone/output self-test и встроенный cue.
- Presence summary, routing, history, settings.
- Новый tray menu.

#### P1.2 Identity

- app actor + memberships;
- node/control token split;
- transactional self-service orbit creation;
- DPAPI/Keychain storage;
- recovery secret;
- device invite;
- optional Telegram link;
- миграция существующих Telegram members без потери ролей.

#### P1.3 Capture и upload

- microphone picker/capture;
- toggle hotkey;
- visible/audible recording indication;
- short file picker/drop;
- resumable/idempotent upload;
- server validation и canonical WAV pipeline;
- limits/quotas/retention из §8.

#### P1.4 Clip delivery

- audience: this Pulsar, own Barycenter, current pairwise approach;
- delivery: overlay, interrupt, after_current;
- prepare/ready/play barrier;
- receipts и partial delivery;
- DND и block;
- overlay FIFO.

#### P1.5 Mixer

- additive Windows media branch;
- continuous main ring consumption;
- ducking + limiter;
- macOS parity;
- interrupt/resume for Spotify main program;
- deterministic render tests и live audio tests.

#### P1.6 Telegram adapter

- voice проходит через общий ingest service;
- inline delivery actions;
- legacy default after_current;
- одинаковые receipts/названия sender/target.

#### P1.7 Compliance

- privacy policy;
- UGC terms/content guidelines;
- report/block/admin disable;
- IARC update;
- microphone capability;
- RU/EN listing и реальные screenshots;
- certification notes со сценарием без external account.

### 19.3 Не входит в фазу 1

- файлы длиннее clip limit;
- потоковое воспроизведение;
- Air из более чем двух Барицентров;
- offline inbox/replay beyond local history;
- настоящий hold-to-talk/live streaming;
- AEC/E2EE;
- soundboard/schedules.

### 19.4 Acceptance scenarios

**A1. Clean Store path.** На чистой Windows без Spotify и Telegram: install →
Try locally → builtin cue → microphone loopback → Create Barycenter → record
clip → This Pulsar → played receipt. Ни одного credential от команды.

**A2. Two Pulsars.** Primary создаёт device invite; чистая вторая установка
join; первая записывает 10-секундный clip, выбирает own Barycenter и включает
origin; обе online ноды начинают его с межнодовым skew ≤100 ms.

**A3. Overlay continuity.** Во время Spotify playback отправляется 10-секундный
overlay. Музыка ducked, продолжает временную шкалу, возвращается без seek/skip;
позиционная ошибка после overlay ≤200 ms, нет ring overflow/underrun burst.

**A4. Interrupt.** Clip немедленно прерывает Spotify, затем трек продолжается
с прежней audible position с допуском 500 ms.

**A5. Existing queue.** Telegram voice без выбора режима остаётся первым после
current element; старый порядок voice FIFO не меняется.

**A6. DND/offline.** DND target не autoplay; offline target не слышит clip
после позднего reconnect; sender видит точную причину.

**A7. Failure recovery.** Обрыв upload и coordinator reconnect не создают
дубликатов; локальный draft можно retry/delete.

**A8. Store certification.** Reviewer выполняет A1 по инструкции; listing
показывает реальные primary screens; submission не требует Spotify account.

A3/A4 проверяют optional Spotify integration внутренними тестовыми
установками. Они не входят в инструкции Store reviewer и не возвращают
требование отдельного certification Spotify account.

### 19.5 Нефункциональные ворота

- 15-секундный clip: stop-record → audible p95 ≤4 s на нормальном broadband;
- synchronized start skew p95 ≤100 ms после ready barrier;
- application memory ≤250 MiB для максимального phase-1 clip;
- no render-thread allocations/blocking introduced by mixer;
- 100 consecutive overlays в automated render test без deadlock/leak;
- все unit/integration/golden suites green;
- live Windows hardware proof обязателен до Store upload.

### 19.6 Exit gate

Фаза завершена только когда A1–A8 доказаны логами/screenshots/tests, новая
Store submission принята либо единственные оставшиеся замечания не связаны с
testability, metadata или self-contained functionality.

## 20. Фаза 2 — Air rooms и потоковое пользовательское аудио

### 20.1 Цель

Pulsar поддерживает общий эфир 2..N Барицентров и длинные пользовательские
аудиофайлы как полноценный main program с bounded-memory streaming.

### 20.2 Обязательный codec/player spike

**[SPIKE P2-CODEC]** До реализации выбрать Windows/macOS совместимый тракт:

- Media Foundation decoder;
- проверенный pure-Go decoder;
- подписанный/bundled decoder;
- canonical server-side codec/variants.

Spike обязан доказать:

- decode MP3/AAC/Opus минимум;
- range/chunk fetch;
- pause/seek/resume;
- bounded memory на 2-часовом файле;
- синхронный scheduled start;
- Store/AppContainer совместимость;
- лицензионную допустимость выбранных библиотек.

Выбор кодека не фиксируется догадкой в фазе 1.

### 20.3 Scope

#### P2.1 Stream media

- `audio_track` ingest;
- compressed canonical variant(s);
- content-length/range support;
- bounded disk cache;
- incremental decoder/ring;
- prepare до достаточного buffer, не полного download;
- pause/seek/resume/ended;
- track metadata и progress;
- queue/replace;
- максимум по умолчанию: 2 часа и 500 MiB input до уточнения quotas.

#### P2.2 Air rooms

- `airs`/`air_members`;
- create/invite/join/confirm/leave/park/dissolve;
- 2..8 Барицентров, до 20 online Pulsars;
- one active Air per Barycenter;
- pairwise approach compatibility/migration;
- Air policies;
- living-air join/catch-up.

#### P2.3 Explicit recipients

- current Air/own/explicit target sets;
- include/exclude origin;
- target snapshot ACL;
- personal delivery для любого количества участников;
- никакого fallback «если >1 адресата, отправить всем».

#### P2.4 Inbox и delivery lifecycle

- missed clips в inbox;
- replay/delete;
- explicit TTL;
- delivery receipts per target;
- no late autoplay;
- history/queue pagination.

#### P2.5 Telegram parity

- audio/document → track;
- Air selection;
- queue/replace controls;
- status and receipts;
- file limit/rights errors человеческим текстом.

### 20.4 Acceptance scenarios

**B1. One-hour track.** Загрузить и проиграть часовой файл на Windows и macOS;
start до полного download; RSS каждой ноды ≤200 MiB; pause/seek/resume без
полной перезагрузки.

**B2. Three Barycenters.** Создать Air A+B+C минимум с пятью Pulsars; clip и
track доходят ровно один раз всем target nodes; нет транзитивных дублей.

**B3. Living Air.** Один Барицентр offline не блокирует остальных; вернувшийся
track-получатель catch-up к audible position; старый overlay не autoplay.

**B4. Leave.** B покидает Air во время main track: B fade/stop, A+C продолжают;
новые media недоступны B; personal orbit state сохраняется.

**B5. Explicit target.** Сообщение A→C недоступно B на API, в UI и по прямому
знанию media id.

**B6. Mixed versions.** Phase-1 node в Air получает совместимый clip fallback,
но не блокирует stream track: coordinator показывает unsupported target и
следует явной policy, не молчит.

**B7. Rights/abuse.** File upload требует принятой content policy; report,
delete и actor disable реально блокируют дальнейший fetch.

### 20.5 Нефункциональные ворота

- track start p95 ≤5 s при достаточной сети;
- start skew p95 ≤100 ms после buffer-ready barrier;
- seek-to-audio p95 ≤3 s;
- memory bounded независимо от duration;
- media storage and egress metrics/quotas работают;
- migration active pairwise link → Air проходит с rollback rehearsal;
- 8 Барицентров/20 Pulsars проходят synthetic coordinator load test.

### 20.6 Exit gate

B1–B7 доказаны, pairwise compatibility остаётся green, а Air и long audio
включены минимум на одном реальном многодомовом beta-сценарии без critical
incidents семь дней.

## 21. Фаза 3 — near-live push-to-talk, качество и приватность

### 21.1 Цель

Сделать Pulsar ощущаемым как настоящий приватный intercom: удержал кнопку,
говоришь, другие места слышат с малой задержкой; при этом capture понятен,
эхо контролируется, а содержимое защищено end-to-end там, где это возможно.

### 21.2 Scope

#### P3.1 Hold-to-talk и streaming transport

- подтверждённый global key-down/key-up path;
- hold gesture и fallback toggle;
- progressive audio chunks во время capture;
- jitter buffer;
- late join запрещён либо начинается с явно допустимого live edge;
- backpressure/network loss handling;
- start/end/cancel signalling;
- overlay duck синхронизирован с live stream.

#### P3.2 Capture quality

- acoustic echo cancellation;
- noise suppression;
- automatic gain control с отдельным input gain ceiling;
- headphone/speaker mode;
- input health diagnostics;
- Windows и macOS parity либо честная capability matrix.

Один и тот же capture DSP используется для записываемых clips, локального
record-then-play и live PTT. Он не дублируется в трёх независимых реализациях.
Input gain ceiling не равен receiver-side local volume ceiling: последний по-
прежнему применяется последним в playback/mixer graph и не контролируется
coordinator. AEC использует явный синхронизированный render reference; speaker
route без пригодного reference не может называться accepted молча.

#### P3.3 End-to-end encryption

**[SPIKE P3-E2EE]** Выбрать стандартизованный, независимо проверяемый group-key
protocol и точные Store-compatible библиотеки для Orbit/Air. Собственные
криптографические primitives не проектируются.

- device/group/content keys создаются и хранятся только на Pulsar clients;
- coordinator сериализует подписанные client-produced membership commits и
  маршрутизирует ciphertext/metadata, но не создаёт, не unwrap и не escrow
  content secrets;
- client-owned rotation выполняется при join/leave/revoke;
- новое устройство не получает старую историю без явного разрешения;
- локальная нормализация/encoding заменяет server-side ffmpeg для E2EE media;
- clips, tracks и saved cues используют versioned chunked AEAD container:
  manifest, target snapshot, epoch и chunk index аутентифицированы, seek/range
  не требуют полной расшифровки файла;
- live PTT frames получают отдельный session key, nonce/replay discipline и
  аутентифицируются до jitter decode;
- report flow позволяет пользователю добровольно приложить расшифрованную
  evidence copy; UI явно говорит, что эта purpose-limited copy покидает E2EE
  boundary и становится доступна moderation;
- recovery и multi-device key transfer документированы;
- metadata leakage описана честно.

Revoke/delete запрещают новые grants/fetch, но не обещают стереть уже скачанные
keys или plaintext с чужого устройства. Recovery требует surviving authorized
device либо отдельно threat-modeled user-held recovery capability; если их нет,
потеря защищённой истории может быть необратимой. Telegram upload не называется
E2EE, поскольку Telegram/bot уже видел plaintext. Unsupported target никогда не
вызывает silent plaintext downgrade: пользователь явно исключает его либо send
не происходит.

E2EE остаётся выключенным до threat model, library/container spikes,
independent protocol-design review, cross-platform vectors и внешнего review
реализации с закрытыми critical/high findings.

#### P3.4 Soundboard и automation

- сохранённые короткие cues;
- configurable hotkeys;
- schedules/quiet-hour aware announcements;
- optional webhook/local automation API с scoped tokens;
- каждый automation event виден в history и может быть быстро отключён;
- никакой automation не обходит DND.

Saved cue — owner-scoped durable reference на canonical ready MediaItem либо
hash-pinned builtin asset. Его pinning, quota, retention, delete, report и actor
disable используют общие media services; обычный cleanup clips не должен
молча ломать soundboard. Scheduler хранит IANA timezone, имеет явные DST и
no-catch-up semantics, атомарно claim-ит execution и не создаёт дубль после
restart или clock jump. Token показывается один раз, хранится только как hash и
немедленно revoke-ится. Manual soundboard и automation включаются раздельно;
ни один trigger не открывает microphone capture.

### 21.3 Acceptance scenarios

**C1. Hold PTT.** 100 циклов press/hold/release из разных foreground apps без
залипшего микрофона, потерянного release или продолжения capture после lock.

**C2. Latency.** На реальных двух домашних сетях mouth-to-ear p50 ≤800 ms,
p95 ≤1500 ms; packet loss 2% не делает речь неразборчивой и не ломает main
program после завершения.

**C3. Echo.** Speaker и headphone matrix на Windows/macOS для recorded clips и
live PTT не создаёт разборчивого возвратного эха у удалённого слушателя и не
разрушает near-end speech при double-talk; деградация AEC явно видна.

**C4. Membership crypto.** После удаления B из Air новые clips, tracks, cues и
live PTT A↔C не могут быть расшифрованы B; новый D не читает прошлую историю
без grant.

**C5. Coordinator privacy.** Содержимое E2EE test transmission невозможно
воспроизвести из coordinator storage/traffic capture; metadata соответствует
документированной модели.

**C6. Report.** Получатель E2EE media может отправить добровольную evidence copy
и report проходит moderation workflow без скрытого server decryption.

**C7. Automation safety.** Schedule respects IANA timezone, frozen DST/no-
catch-up rules, DND и recipient local volume ceiling; restart/clock jump не
создаёт duplicate event, revoke token немедленно останавливает новые events.

### 21.4 Нефункциональные ворота

- live jitter buffer bounded;
- reconnect не продолжает старую capture session;
- key material только в OS secure storage и никогда в логах/crash reports;
- external security review закрывает critical/high findings;
- root-agent построчно проверил implementation diff и повторил доступные suites;
- independent realtime, crypto, automation, privacy/Store и migration/recovery
  reviewers закрыли critical/high findings на том же build hash;
- privacy policy и Store disclosure обновлены до rollout;
- seven-day real beta без stuck capture, runaway automation или key-loss
  incident. Material code/config change или такой incident обнуляет семь дней.

### 21.5 Exit gate

C1–C7 доказаны. `live_ptt`, `e2ee_media`, `soundboard_cues` и `automation`
включаются раздельно. Невозможность закончить E2EE не блокирует качественный
live PTT, но приложение не называет соединение end-to-end encrypted до
прохождения C4–C6 и security reviews. Manual soundboard может быть выпущен без
automation, если его собственные gates доказаны.

## 22. Сводная зависимость фаз

| Возможность | Фаза 1 | Фаза 2 | Фаза 3 |
|---|---:|---:|---:|
| Работа без Spotify | Да | Да | Да |
| Работа без Telegram | Да | Да | Да |
| Local self-test | Да | Да | Да |
| In-app create/join | Да | Да | Да |
| Toggle recording hotkey | Да | Да | Да |
| Clip upload | Да | Да | Да |
| Overlay/interrupt/after-current | Да | Да | Да |
| Pairwise approach delivery | Да | Compatibility | Compatibility alias |
| Air 2..N | Нет | Да | Да |
| Long streaming track | Нет | Да | Да |
| Offline inbox | Нет | Да | Да |
| True hold/live PTT | Нет | Нет | Да |
| AEC/noise suppression | Нет | Нет | Да |
| E2EE media | Нет | Нет | Да, после review |
| Soundboard/schedules | Нет | Нет | Да |

## 23. Решения, отложенные до phase gates

Эти вопросы не блокируют фазу 1:

1. **P2-CODEC:** Media Foundation, pure-Go или bundled decoder; решается
   измеряемым spike и license review.
2. **P2 storage economics:** точные paid/free quotas после telemetry реального
   clip usage.
3. **Air lifecycle:** срок хранения parked Air после beta data; runtime-модель
   и migration при этом уже определены.
4. **P3-HOTKEY:** low-level hold implementation под Store AppContainer.
5. **P3-E2EE:** group key protocol, local encoding и moderation evidence.
6. **Automation surface:** webhook против локального plugin API после threat
   model.

Ни один из этих вопросов не должен возвращать Spotify или Telegram в роль
обязательного аккаунта.

## 24. Ссылки

- Основная текущая спецификация: `docs/spec.md`.
- Multitenant/orbit модель: `docs/v2-multitenant-design.md`.
- Предложение Air: `docs/idea-air-rooms.md`.
- Текущая Windows/Store цель: `docs/goal-v2.1.md`.
- Protocol compatibility notes: `docs/protocol.md`.
- Store listing/certification notes: `docs/store-listing.md`.
- Microsoft Store Policies, включая UGC §11.12:
  https://learn.microsoft.com/en-us/windows/apps/publish/store-policies
- Windows app capabilities и microphone declaration:
  https://learn.microsoft.com/en-us/windows/apps/package-and-deploy/app-capability-declarations
- Win32 `RegisterHotKey`:
  https://learn.microsoft.com/en-us/windows/win32/api/winuser/nf-winuser-registerhotkey

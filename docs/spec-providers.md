# barycenter: провайдерский слой. Спецификация ранней версии

| Поле | Значение |
|---|---|
| Версия | draft 0.2 (0.1 заказчика + архитектурная интеграция) |
| Дата | 2026-07-05 |
| Статус | Ранняя спецификация: развилки [ВАРИАНТЫ], дефолты [РЕШЕНИЕ] (заказчик), привязки к текущему коду [АРХ] (агент). К обсуждению до внесения в основную спеку |
| Базовый документ | docs/spec.md v1.3 + docs/v2-multitenant-design.md (M1/M2 shipped) — НЕ v1.2: ядро уехало, см. §0 |
| Рамки | Yandex Music как второй провайдер; смешанные дома; оба режима; политика недоступных треков |

## 0. [АРХ] Fit с фактической архитектурой (v2.1, prod)

Драфт 0.1 писался к v1.2. Реальность, в которую слой встаёт:

| Драфт говорит | Фактически есть | Следствие для этой спеки |
|---|---|---|
| «пара», «нода a/b» | Орбиты×слоты: мультитенантный координатор (M1), peer-set FSM (M2, N домов) | «Участник» ниже = слот орбита. Доступность и провайдер — per (orbit, slot) |
| Tailscale | Публичный wss://barycenter.relux.works, токены слотов hashed в БД, pairing-коды | Топология не меняется этим эпиком |
| elements.uri | elements(orbit_id, uri) + Playlist.Tracks []uri (U10) | ctid-переход затрагивает и плейлист-слой: Tracks []uri -> []ctid, миграция |
| «settings: provider per нода» | Таблица slots(orbit_id, slot, token_hash, paired_by…) | provider = колонка slots; /provider пишет туда, EnsurePeer не трогается |
| спека 6.3 librespot | Наш форк relux-works/go-librespot (pure-Go decoders, CGO=0), бандлится в Pulsar.app | Spotify-движок уже наш; Яндекс-путь Y1 вообще без демона |
| «плеер вставок» | PlayerCore.playVoice: VoiceCache (LRU, Bearer) -> AVAudioFile -> AVAudioPlayerNode; T_local-старт уже есть у offset_test-кликов | Y1 = ровно этот тракт: VoiceCache переименовывается в MediaCache, scheduleSegment(при позиции)+start(at: AVAudioTime(T_local)) |
| громкость -14 LUFS | Наш mixer: setMusicGain (raised-cosine), volume glide; голосовые уже нормируются к -14 на координаторе (ffmpeg loudnorm) | r128-гейн Яндекса = pre-gain плеер-ноды; фолбэк loudnorm на ноде возможен, но у нас ffmpeg НЕ бандлится в Pulsar.app — фолбэк (b) переезжает на координатор ЛИБО в spike решаем бандлить ffmpeg (вес!) [ВНИМАНИЕ-РЕВЬЮ] |
| «копия токена у координатора для резолва» | Инвариант goal v2: «Spotify account credentials never leave the node's Mac» | КОНФЛИКТ ИНВАРИАНТА. Варианты: (а) resolve-RPC к ноде-владельцу токена (новое сообщение resolve_probe, нода отвечает avail/download_info без выдачи токена); (б) как в драфте — Яндекс-токен в env координатора (ослабление инварианта, только Яндекс, только для resolve/avail). [РЕШЕНИЕ заказчика в драфте = (б); АРХ-рекомендация: подтвердить осознанно на ревью, зафиксировать в Invariants goal v3] |
| протокол v1 | 26 golden, contract-тесты Go+Swift, счётчик в Swift-тесте | §7: аддитивная эволюция, uri остаётся = spotify-ref (старые ноды живут), новые поля optional; golden в том же коммите |

Модули-владельцы (куда ложится код): координатор: internal/resolver (новый), internal/spotify (расширяется ISRC-поиском), internal/yandex (новый, resolve/avail), store (tracks/track_refs/availability), session (ctid в Element/Playlist), bot (/provider /resolve, пометки [S|Y]); нода: NodeCore/MediaCache (бывш. VoiceCache), NodeCore/YandexEngine (новый: download_info+fetch+r128), PlayerCore.load ветвится по provider.

## 1. Цель и рамки

Добавить понятие провайдера так, чтобы:
1. Каждый дом (слот) имел активного провайдера: spotify или yandex. Орбит может быть однородным или смешанным.
2. Периастрон работал в смешанном орбите: общий эфир, каждый дом тянет тот же трек из своего сервиса под своим аккаунтом.
3. Апоастрон работал в смешанном орбите: каждый слушает своё, inject в обе стороны.
4. Недоступные части участников треки обрабатывались по явной политике.

Не меняется: топология (Pulsar-ноды + Barycenter, wss), голосовые вставки, синхронизация времени, инвариант «аудио между домами не ходит».

## 2. Модель: канонический трек

Элемент очереди перестаёт нести uri Spotify и ссылается на ctid:

```json
{
  "ctid": "ct_01J...",
  "title": "...", "artists": ["..."], "duration_ms": 214000,
  "isrc": "GBAYE0601498",
  "origin":  { "provider": "spotify", "ref": "spotify:track:...", "by_slot": "a" },
  "refs": {
    "spotify": { "ref": "spotify:track:...", "duration_ms": 214000 },
    "yandex":  { "ref": "10994777:1193829", "duration_ms": 213800 }
  },
  "resolution": { "method": "odesli | metadata | same | manual", "score": 0.97, "at": 1751 }
}
```

- ctid: ULID. [АРХ] session.Element получает CTID; Element.URI остаётся денормализованной копией origin.ref (журнал elements обратно совместим).
- refs: Spotify `spotify:track:<id>`; Yandex `<track_id>:<album_id>`.
- isrc: только если источник отдал (Spotify отдаёт external_ids.isrc; Яндекс — нет, перепроверка spike-2/S1).
- [АРХ] Доступность НЕ внутри refs, а отдельной таблицей (зависит от аккаунта дома): `availability(orbit_id, slot, provider, ref, ok, checked_at)`, ok NULL = не проверялось, TTL 7 дней.

Хранение [АРХ]: `tracks(ctid, title, artists_json, duration_ms, isrc, origin_json, resolution_json)` и `track_refs(ctid, provider, ref, duration_ms)` — ГЛОБАЛЬНЫЕ (кэш резолва общий для всех орбитов: маппинг ID-ID не приватен); availability — per-orbit. elements.ctid добавляется; Playlist.Tracks мигрирует []uri -> []ctid (JSON-снапшоты сессий: миграционный шим при LoadSession).

## 3. Разрешение (resolution)

Вход: ссылка/ref любого провайдера. Выход: refs для всех провайдеров, активных у слотов орбита. Разрешает координатор при постановке/inject.

[ВАРИАНТЫ]
| Вариант | Механика | Плюсы | Минусы |
|---|---|---|---|
| A. ISRC | ISRC источника -> поиск у цели | Точность | Только в сторону Spotify (`isrc:<код>` в Web API; Яндекс ISRC не отдаёт и не ищет) |
| B. Odesli | GET api.song.link/v1-alpha.1/links?url=… -> linksByPlatform | Обе стороны, один вызов | Третья сторона; rate limit без ключа (ключ: developers@song.link); покрытие проверить (spike-2/S2) |
| C. Метаданные | Поиск "artist title" у цели, скоринг | Без внешних зависимостей | Ложные совпадения |
| D. Гибрид | same-provider -> A (если цель Spotify и есть ISRC) -> B -> C -> unresolved | Максимум точности | Больше кода |

[РЕШЕНИЕ] D, каскад в указанном порядке. Кэш в track_refs навсегда (method+score в resolution); ручная починка /resolve пишет method=manual.

Скоринг C (обязательные правила): нормализация (lower, вырезать «(feat…)», «- remastered…», «(deluxe…)»); совпадение: пересечение артистов, похожесть названия >= 0.9 (Jaro-Winkler/триграммы), |Δduration| <= 2000 мс; штраф до нуля: cover/karaoke/tribute/live у кандидата при отсутствии в оригинале; ниже порога — unresolved, никаких «почти то» молча.

Правило длительности (для всех методов, включая B): |Δduration| > 2000 мс = другая редакция; для периастрона unresolved; для апоастрон-инжекта — [флаг, по умолчанию тоже unresolved].

Парсер ссылок [АРХ: internal/links.ParseRef расширяется]: + music.yandex.ru|com/album/<A>/track/<T> и /track/<T> (album добирается через API трека).

## 4. Доступность и политика пропусков

### 4.1 Два эшелона
1. Проактивный (best effort, в availability-кэш): Spotify GET /tracks/{id}?market=from_token (нужен OAuth участника; до его настройки — пропускать), Yandex Track.available/available_for_premium_users (+косвенно download_info). [АРХ] выполняется тем, у кого токен: см. §0 конфликт инварианта — вариант (а) шлёт resolve_probe ноде.
2. Реактивный при load (авторитетный, УЖЕ в ядре): track_unavailable -> скип с уведомлением. [АРХ] наши R0-фиксы остаются: readiness-гейт демона, 30с таймаут, локальный ретрай — «недоступен» теперь честный.

### 4.2 Периастрон: трек есть не у всех
«Есть у слота» = ref его провайдера разрешён И availability.ok != false.

[ВАРИАНТЫ] P1 строгий эфир (скип с «„<title>“: нет у Pulsar B (Яндекс), пропускаю»); P2 замена версией (схлопывается в P1); P3 частичный эфир (нода без трека получает wait той же длительности — механика личных голосовых, уже реализована; [АРХ] в peer-set FSM это target-подмножество, EffWait остальным — готово с M2).

[РЕШЕНИЕ] P1 по умолчанию (формулировка заказчика). P3 — флаг сессии на будущее, в ранней версии выключен.

### 4.3 Апоастрон: inject недоступного
[РЕШЕНИЕ] Отказ с указанием дома и провайдера: «у <дом> (<провайдер>) этого трека нет». [Вариант на потом: инлайн-кнопка «заинжектить похожее?» при под-пороговом кандидате C.]

## 5. Слой воспроизведения Yandex на ноде

Spotify-путь не меняется (наш форк -> FIFO -> ринг -> Source node). Яндекс: прямые файлы без DRM (mp3/aac 64..320) — демон не нужен.

[ВАРИАНТЫ] Y1 файл->AVAudioPlayerNode ([РЕШЕНИЕ]; [АРХ]: буквально тракт голосовых вставок: MediaCache.fetch(ctid, provider) -> AVAudioFile -> scheduleSegment(from: startFrame) -> play(at: AVAudioTime(T_local)) — тайминговый путь offset_test-кликов, калибровка offset переживает смену провайдера, т.к. тракт после микшера общий); Y2 Music Assistant (референс, не встраиваем); Y3 свой pipe-демон (симметрия ради симметрии — нет).

Требования Y1:
- Клиент: прямые HTTP-вызовы из NodeCore/YandexEngine (референс протокола MarshalX/yandex-music-api; эндпоинты track, download_info, search — полный список в spike-2). Без сайдкаров.
- load(yandex): скачать целиком в MediaCache (ссылки download_info короткоживущие), открыть AVAudioFile (валидация декода) -> ready. Позиция: scheduleSegment.
- Громкость: требование |ΔLU| между домами <= 2. [РЕШЕНИЕ] (a) гейн из Track.r128 к эквиваленту -14 LUFS как основной путь; (b) loudnorm-фолбэк — [АРХ-ВНИМАНИЕ] ffmpeg в Pulsar.app не бандлится: фолбэк либо на координаторе (файл гоняется через него — ломает «аудио не ходит между домами»? Нет: координатор и так раздаёт голосовые; но объёмы!), либо бандлим ffmpeg (+50МБ). Решение после spike-2/S5.
- Кодеки: mp3/aac 320..192; FLAC вне ранней версии.
- Кэш: LRU, общий с вставками (MediaCache), лимит конфиг (дефолт 2 ГБ), имена ctid+provider.
- Credentials: Яндекс OAuth-токен — в Keychain ноды (рядом с pairing-кредами, CredentialsStore расширяется), получение Device Flow (владелец подтверждает код с телефона). Копия у координатора — см. §0 конфликт инварианта.

## 6. Режимы

### 6.1 Периастрон в смешанном орбите
1. Координатор для каждого слота выбирает ref его активного провайдера.
2. load провайдер-специфичен; ready-семантика своя (librespot-путь против «файл скачан и открыт»). [АРХ] peer-set ready-барьер (M2) не меняется.
3. resume_at общий; T_local-механика без изменений.
4. Границы: duration элемента = max(refs); правило ended «все, либо один + laggard в 1с от конца» (M2, N-wise) уже покрывает Δ<=2с.
5. Голосовые: без изменений.

Инварианты: каждый дом стримит из своего сервиса под своим аккаунтом; между домами — только управление и файлы вставок. Правовой профиль: + второй неофициальный клиентский слой того же класса риска.

### 6.2 Апоастрон в смешанном орбите
- Spotify-дом: как сейчас (Connect, add_to_queue).
- Yandex-дом: [ВАРИАНТЫ] S1 бот-очередь (NodeApp ведёт локальную очередь ctid); S2 Моя волна (rotor-API, like/skip из бота); S3 Ynison-receiver (нода видна в приложении Яндекса; референс — MA-плагин, passive-player model; самый большой кусок). [РЕШЕНИЕ] Ранняя версия: S1+S2; S3 — отдельный этап (оценка spike-2/S7).
- Inject: Spotify-дому через add_to_queue; Яндекс-дому — в голову S1-очереди (при Моей волне — вклинивается следующим). Недоступность: 4.3.

### 6.3 Переключение провайдера
/provider <слот> <spotify|yandex>: пишет slots.provider, пушится ноде сообщением set_provider. На лету только вне PLAYING. Оба провайдера могут быть сконфигурированы одновременно; активный определяет ref при load и адрес inject.

## 7. Изменения протокола (v1 -> v1.1) [АРХ: аддитивно, 26 golden -> 28+]

| Сообщение | Было | Стало |
|---|---|---|
| load | { element_id, uri, position_ms } | + provider?, ref?, duration_ms? (uri сохраняется = spotify-ref; отсутствие provider = spotify: старые ноды живут) |
| solo_inject | { uri } | + provider?, ref?, ctid? |
| state | { …, speakers } | + provider (активный у ноды) |
| set_provider (новое) | | { provider } |
| error | коды спеки | + decode_failed, media_fetch_failed |

Резолв целиком на координаторе (кроме варианта (а) из §0: тогда + resolve_probe/resolve_result — решение на ревью). Golden + оба кодека + contract-тесты в том же коммите (инвариант goal).

## 8. Координатор, данные, бот
- Таблицы §2; slots.provider; секреты провайдеров — env/Keychain по §0.
- internal/resolver: каскад §3, кэш, rate-limit Odesli.
- Бот: ссылки обоих сервисов; фидбек «в очередь: <artist — title> [S+Y]» или причина отказа; /queue помечает [S|Y|SY]; /provider; /resolve <номер> <ссылка> (method=manual); ответы о недоступности всегда называют дом и провайдер.

## 9. Spike-2 (обязателен до имплементации)

| # | Проверка | Влияет на |
|---|---|---|
| S1 | Yandex Track: ISRC (ожидаем нет), живость available, r128 на реальных треках | §3, §5 |
| S2 | Odesli: покрытие Яндекса на 50 треках обеих сторон, лимиты, ключ | §3.B |
| S3 | download_info: TTL ссылок, стабильность 320, форма ошибки недоступного | Y1, 4.1 |
| S4 | AVAudioFile: декод mp3/aac, scheduleSegment с позиции, точность play(at:) [АРХ: против нашего T_local; замер как в offset_test] | Y1, 6.1 |
| S5 | r128 против loudnorm на 10 треках; решить (a)/(b) и где живёт фолбэк | §5 |
| S6 | Матрица доступности 20 треков × (2 Spotify-аккаунта, 2 стороны Яндекса) | §4, частота P1-пропусков |
| S7 | Ynison: объём receiver по референсам | 6.2/S3 |
| S8 [АРХ] | Два движка на одной ноде: FIFO-ринг (Spotify) и AVAudioPlayerNode (Яндекс) в одном AVAudioEngine-графе — переключение без рестарта движка, поведение при configuration change | Y1 |
| S9 [АРХ] | Протокол-совместимость: старая нода против v1.1-координатора (аддитивные поля), строгие декодеры обеих сторон | §7 |
| S10 [АРХ] | Резолв-инвариант: прототип resolve_probe против env-токена на координаторе; решение конфликта §0 | §0, §4.1 |

## 10. Ссылки
Ядро: docs/spec.md v1.3; docs/v2-multitenant-design.md; docs/goal-v2.md.
Spotify: наш форк https://github.com/relux-works/go-librespot; Web API track (external_ids.isrc, is_playable): developer.spotify.com/documentation/web-api/reference/get-track; поиск isrc: …/reference/search.
Yandex (неофициальное): MarshalX/yandex-music-api (+docs yandex-music.readthedocs.io, модели ym.marshal.dev); acherkashin/yandex-music-open-api; Music Assistant провайдер и Ynison-плагин (music-assistant.io); DECE2183/yamusic-tui; K1llMan/Yandex.Music.Api.
Матчинг: Odesli api.song.link/v1-alpha.1/links (ключ: developers@song.link); MattrAus/odesli.js (формат linksByPlatform).

## 11. Вне рамок ранней версии
Ynison-receiver (после ранней версии, по S7); FLAC/lossless; плейлисты/альбомы Яндекса как источник; миграция плейлистов между сервисами; третий провайдер; изменения голосового пайплайна.

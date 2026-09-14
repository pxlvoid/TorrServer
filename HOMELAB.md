# TorrServer — сборка homelab

Форк [YouROK/TorrServer](https://github.com/YouROK/TorrServer) с **постоянным дисковым кэшем**.
Всё остальное — как в upstream; обновления upstream вливаются автоматически, наши доработки при этом не затираются.

## Зачем

В upstream кэш торрента на диске ограничен `CacheSize` и чистится «кольцом» вокруг места, где сейчас читает
плеер. Сезон сериала — один торрент: пока автоплей крутит серии дальше, начало сезона вытесняется. Вернулся
туда, где уснул, — кэша нет, всё качается заново и заодно вытесняет поздние серии. Плюс при старте upstream
удаляет кэш торрентов, которых нет в его списке.

## Что умеет форк

Режим **«Хранить просмотренное на диске»** (боковое меню → «Кэш на диске»; нужен включённый «Использовать диск»
и путь к кэшу в настройках):

- Во время просмотра ничего не вытесняется. `CacheSize` задаёт только окно упреждающей загрузки.
- Порядок наводит уборщик (раз в минуту): держит весь кэш в пределах **лимита, ГБ**, выбрасывая то, к чему
  дольше всего не обращались (по всем торрентам сразу), и удаляет куски, которые не открывали **N дней**.
  На диске всегда остаётся ≥ 5 ГБ свободного места. То, что сейчас смотрят, и **закреплённые** (★) торренты
  уборщик не трогает.
- Кэш переживает перезапуск, drop торрента и удаление торрента из списка (`RemTorrent`) — удаляют его только
  уборщик и кнопки в диалоге.
- Куски, прошедшие проверку хэша, запоминаются в `<кэш>/<hash>/.homelab.json` и после перезапуска считаются
  готовыми (включая короткий последний кусок — в upstream он никогда не считался готовым). Файл куска без такой
  отметки перепроверяется движком, а не принимается по размеру: куски качаются вразнобой, и файл полного
  размера может быть с дырами.
- Диалог «Кэш на диске»: занято/лимит/свободно, настройки, список закэшированного (постер и название из списка
  TorrServer, размер, доля торрента, когда смотрели), закрепить, удалить, «Очистить незакреплённое».

Режим выключен — поведение в точности как в upstream (это проверяет тест).

API (с той же авторизацией, что и остальной API): `GET/POST /homelab/settings`
(`{"persistentCache":true,"limitGB":150,"keepDays":14}`), `POST /homelab/cache`
(`{"action":"list"}`, `{"action":"remove","hash":"…"}`, `{"action":"pin","hash":"…","pinned":true}`,
`{"action":"clear"}`).

## Как устроен форк: чтобы обновления upstream не затирали доработки

**Ветки.** `master` — чистое зеркало upstream, в него не коммитим. `homelab` — ветка по умолчанию:
upstream + наши изменения. Релизы upstream вливаются в неё **merge-коммитами** (история не переписывается,
force-push не нужен).

**Наш код — в своих файлах**, upstream их никогда не тронет:

| Что | Где |
|---|---|
| Постоянный кэш, обёртка кусков, отметки проверки | `server/torr/storage/torrstor/homelab.go` |
| Уборщик (лимит, срок, свободное место) | `server/torr/storage/torrstor/homelab_janitor.go`, `homelab_statfs*.go` |
| Список / удаление / закрепление | `server/torr/storage/torrstor/homelab_api.go` |
| Тесты | `server/torr/storage/torrstor/homelab_test.go` |
| Настройки (хранятся отдельно от `BTSets`) | `server/settings/homelab.go`, `homelab_upstream.txt` (на каком релизе стоим) |
| HTTP API | `server/web/api/homelab.go` |
| Диалог и переводы (без правки `locales/*.json`) | `web/src/components/Homelab/` |
| Автоматика | `scripts/homelab-sync.sh`, `.github/workflows/homelab-*.yml` |

**В файлах upstream — только однострочные крючки** с пометкой `homelab`:

| Файл | Крючков | Что делает |
|---|---|---|
| `server/torr/storage/torrstor/cache.go` | 5 | `Init` → `hlOnInit`; `Piece` → `hlPieceImpl`; `Close` → `hlOnClose` и `&& !hlKeep` у `RemoveCacheOnDrop`; `cleanPieces` → `hlOwnsEviction` |
| `server/server.go` | 1 | `cleanCache` при старте не трогает постоянный кэш |
| `server/torr/apihelper.go` | 2 | `RemTorrent` не удаляет постоянный кэш |
| `server/web/api/route.go` | 1 | `homelabRoutes(authorized)` |
| `web/src/components/App/Sidebar.jsx` | 2 | импорт и пункт меню «Кэш на диске» |
| `web/src/components/App/PWAFooter/index.jsx` | 2 | импорт и кнопка «Кэш» в нижней панели приложения на телефоне (PWA) |
| `web/src/components/App/PWAFooter/style.js` | 1 | сетка панели на 6 кнопок вместо 5 |

Число крючков сверяет `scripts/homelab-sync.sh` (список `HOOKS`) — поменял крючки, поправь и его, и эту таблицу.
Посмотреть всё наше: `git diff $(cat server/settings/homelab_upstream.txt) homelab`, крючки: `git grep -n homelab -- server/server.go server/torr web/src/components/App`.

**Правила:**
- В файлах upstream — ничего, кроме крючков. Логика — в `homelab*` файлах.
- Не коммитить `server/web/pages/template/` — это собранный веб, его генерирует Docker-сборка
  (`gen_web.go`). В ветке он остаётся upstream-версией, поэтому при слиянии не конфликтует.
- Строки интерфейса — в `web/src/components/Homelab/i18n.js`, не в `locales/*.json`.
- Коммиты и тексты — на русском.

## Автоматическое обновление из upstream

`.github/workflows/homelab-upstream.yml`, каждый день в 06:17 МСК (и вручную: Actions → Upstream (homelab) →
Run workflow, можно указать тег):

1. Берёт последний релиз `MatriX.*` upstream. Уже влит — ничего не делает.
2. Вливает его merge-коммитом, записывает тег в `server/settings/homelab_upstream.txt`.
3. Проверяет: все крючки на месте, `go vet` + наши тесты + сборка сервера, линт диалога и сборка веба.
4. Всё хорошо → push в `homelab` (deploy-ключом) → `homelab-deploy.yml` выкатывает на сервер; `master`
   подтягивается до upstream. В ntfy (топик `updates`) — «TorrServer: влит MatriX.X».
5. Конфликт или проверки не прошли → **ничего не пушит**, прод не тронут: issue в репозитории
   и громкое уведомление в ntfy.

Выкатка на сервере (общий деплой `pxlvoid/deploy`): сборка образа из этого репозитория (Dockerfile upstream,
`VERSION=homelab` → в интерфейсе `MatriX.X-homelab`), подъём с проверкой `/echo`, не поднялось — откат на прежний
образ.

## Если автоматика не справилась

```sh
git clone git@github.com:pxlvoid/TorrServer.git && cd TorrServer   # ветка homelab
scripts/homelab-sync.sh            # или с тегом: scripts/homelab-sync.sh MatriX.146
```

- **Конфликт** — скрипт прерывает слияние и пишет файлы. Повтори руками: `git merge MatriX.X`, в конфликтных
  файлах возьми версию upstream и заново поставь наш крючок в нужное место (таблица выше), `git add`,
  `git commit`, затем `scripts/homelab-sync.sh --check` и `git push`.
- **Не прошли тесты** — upstream поменял то, на что опираются крючки (например, переименовал поле `Piece`
  или порядок в `Cache.Close`). Поправь `homelab*.go` под новый код, `--check`, push.
- Веб проверяется с `--web` (нужны node 16 и yarn — как в Dockerfile upstream).

## Настройки репозитория (не в файлах)

- Ветка по умолчанию — `homelab`.
- Workflow upstream (`docker_image.yml`, `ts_release.yml`, `ts_build.yml`, `test-install-script.yml`)
  выключены в Actions — им здесь нечего делать (нет их секретов), а файлы остаются как в upstream, без конфликтов.
- Секреты: `DEPLOY_SSH_KEY`, `DEPLOY_SERVER` — выкатка; `SYNC_DEPLOY_KEY` — приватная часть deploy-ключа
  «homelab-upstream-sync» с правом записи (push слияний); `NTFY_URL`, `NTFY_TOKEN` — уведомления.

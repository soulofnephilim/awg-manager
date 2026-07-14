# FreeTurn — что уже готово и что нужно подключить руками

## Что в архиве

```
internal/freeturn/          — новый пакет: конфиг, процесс-менеджер, сервис
  types.go                  — Config/Status структуры (под реальные флаги freeturn)
  store.go                  — JSON-хранилище freeturn.json (своя миграционная цепочка не нужна)
  process.go                — PID-based управление процессом (по образцу internal/singbox/process.go)
  service.go                — публичный facade + сборка CLI-аргументов из конфига

internal/api/freeturn.go    — HTTP-хендлеры (GET/PUT config, GET status, POST start/stop)

frontend/src/routes/freeturn/+page.svelte   — страница Клиент/Сервер
frontend/src/lib/types.ts                   — добавлены FreeTurn* типы (в конец файла)
frontend/src/lib/api/client.ts              — добавлены методы api.getFreeTurnConfig() и т.д.
frontend/src/lib/types/usageLevel.ts        — добавлена секция 'freeturn'
frontend/src/lib/components/layout/AppHeader.svelte — добавлен пункт меню FREETURN
```

Всё это уже проверено:
- `internal/freeturn` — собирается и проходит `go vet` как самостоятельный пакет (только stdlib).
- `internal/api/freeturn.go` — собирается и проходит `go vet` против реальных `internal/freeturn` и `internal/response`.
- Фронтенд — `svelte-check` 0 ошибок по всему проекту, `vite build` проходит, страница `/freeturn` попадает в продакшен-бандл.

Чего я **не** проверял: сборку всего `internal/api` целиком и всего `cmd/awg-manager` — для этого нужен Go ≥1.25 (как у тебя локально), а в моей песочнице доступен только до 1.23 (зависимость `golang.org/x/net` в vendor требует 1.25). Так что финальный `go build ./...` у себя на машине всё равно стоит прогнать.

## Шаг 1 — скопировать файлы

Распакуй архив поверх своего форка (`vanomilah/awg-manager`), пути совпадают один в один.

## Шаг 2 — путь к бинарникам freeturn

Узнай, как называются собранные бинарники (`task build` в free-turn-proxy кладёт их в `dist/`) — обычно что-то вроде `client`/`server` или `freeturn-client`/`freeturn-server`. Скопируй их на роутер, например в `/opt/bin/freeturn-client` и `/opt/bin/freeturn-server`.

## Шаг 3 — `cmd/awg-manager/main.go`

Рядом с тем местом, где собирается `pingCheckService` (там это происходит около `pingCheckService := pingcheck.NewService(...)`), добавь:

```go
import (
    // ...
    "github.com/hoaxisr/awg-manager/internal/freeturn"
)

// ...

freeturnService := freeturn.NewService(
    *dataDir,
    filepath.Join(*dataDir, "run"),
    "/opt/bin/freeturn-client",
    "/opt/bin/freeturn-server",
)
srv.AddShutdownHook(freeturnService.Stop)
```

И добавь `FreeTurnService: freeturnService,` в структуру `Dependencies{}`, которая передаётся в `server.NewServer(deps)` (там же, где сейчас стоит `PingCheckService: pingCheckFacade,`).

## Шаг 4 — `internal/server/server.go`

1. Добавь поле в `Server` struct и в `Dependencies` struct (по аналогии с `pingCheckService api.PingCheckService` / `PingCheckService api.PingCheckService`):

```go
freeturnService api.FreeTurnService   // в Server
FreeTurnService  api.FreeTurnService  // в Dependencies
```

2. В конструкторе `NewServer` сохрани его в `s.freeturnService = deps.FreeTurnService` (там же, где `pingCheckService: deps.PingCheckService`).

3. Создай хендлер рядом с `pingCheckHandler := api.NewPingCheckHandler(...)`:

```go
freeturnHandler := api.NewFreeTurnHandler(s.freeturnService)
```

4. Зарегистрируй роуты рядом с `/api/pingcheck/...` (используй ту же переменную `guarded`):

```go
mux.HandleFunc("/api/freeturn/config", guarded(freeturnHandler.GetConfig))
mux.HandleFunc("/api/freeturn/client/config", guarded(freeturnHandler.UpdateClientConfig))
mux.HandleFunc("/api/freeturn/server/config", guarded(freeturnHandler.UpdateServerConfig))
mux.HandleFunc("/api/freeturn/status", guarded(freeturnHandler.GetStatus))
mux.HandleFunc("/api/freeturn/client/start", guarded(freeturnHandler.StartClient))
mux.HandleFunc("/api/freeturn/client/stop", guarded(freeturnHandler.StopClient))
mux.HandleFunc("/api/freeturn/server/start", guarded(freeturnHandler.StartServer))
mux.HandleFunc("/api/freeturn/server/stop", guarded(freeturnHandler.StopServer))
```

## Шаг 5 — собрать и проверить

```bash
go build ./...
cd frontend && npm run build
```

Если что-то не совпадёт по именам полей `Dependencies`/`Server` (структура могла слегка измениться с момента клонирования) — ошибки компилятора укажут ровно на нужные места, это чисто механическая правка по образцу pingcheck.

## Что осознанно упрощено

- `-link` (ссылка VK Calls) сейчас просто текстовое поле — без автоматизации получения ссылки из VK Calls API.
- Капча (`-manual-captcha`) — чекбокс есть, но сам процесс ручного прохождения капчи в браузере страница не показывает (в логах процесса будет видно, что просит).
- `-sub` (подписка) и `-client-id` есть в конфиге, но в форме на странице сейчас не отображаются — добавить как ещё одну пару полей, если понадобится.

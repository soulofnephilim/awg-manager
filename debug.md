# feat/tproxy-ipset-selective-bypass — задачи

## Уже сделано
- [x] Поддержка диапазонов портов в исключениях (`5000-5500 UDP`)
  - Go: `PortRange` тип, `parseExtraPorts`, iptables `--dport N:M`
  - Frontend: `ports.ts` (from/to), `PortChipsInput.svelte`, подсказка в StatusDrawer

---

## Бэкенд

### 1–5. `internal/singbox/router/selective/`
- [x] `deps_check.go` — `IsIPSetAvailable`, `IsXtSetAvailable`, `InstallIPSet`, `EnsureXtSetModule`
- [x] `ipset.go` — `CreateSet`, `DestroySet`, `SetExists`, `EntryCount`, `SetName` (+ staging: `ChunkedAddStaging`/`ChunkedAddLive`/`SwapWithStaging`)
- [x] `collector.go`/`collector_stream.go` — `StreamCollectFromRules`, inline/dat rule-set JSON парсинг
- [x] `resolver.go`/`resolver_pipeline.go` — `ResolveDomainQueriesStream`, `BuildDNSServers` (приоритет singbox→NDMS→system)
- [x] `builder.go` — `Builder.Rebuild`, `Progress`, `EventPublisher`, SSE публикация
- [x] `errors.go` — `ErrOpkgNotFound`, `ErrIPSetNotAvailable`, `ErrXtSetNotAvailable`

### 6. `internal/storage/types.go`
- [x] `SelectiveBypass bool` в `SingboxRouterSettings`

### 7. `internal/singbox/router/iptables.go`
- [x] `SelectiveIPSet bool` в `RestoreInputSpec`
- [x] guard-правила `-m set ! --match-set AWGM-SELECTIVE dst -j RETURN` в обеих цепочках (после DNS, перед catch-all)
- [x] `selectiveSetName` константа

### 8. `internal/singbox/router/iptables_test.go`
- [x] `TestBuildRestoreInput_SelectiveIPSet_AddsGuardRules`
- [x] `TestBuildRestoreInput_SelectiveIPSet_Disabled_NoGuardRules`
- [x] `TestBuildRestoreInput_SelectiveIPSet_GuardAfterDNS`

### 9. `internal/singbox/router/service.go` + `service_selective.go`
- [x] `SelectiveBuilder`, `SelectiveDNSSource` интерфейсы в Deps
- [x] `currentSelectiveBypass`, `currentRulesHash` tracking-поля в ServiceImpl
- [x] `selectiveChanged` детект в `reconcileInstalled`
- [x] `SelectiveIPSet: sr.SelectiveBypass` в `RestoreInputSpec`
- [x] `destroySelectiveSet` при отключении
- [x] `triggerSelectiveRebuild` (async) при включении или изменении правил
- [x] `rulesHash` для детекта изменений правил

### 10. `internal/events/types.go`
- [x] `SelectiveProgressEvent`
- [x] `SelectiveStatusEvent`

### 11. `internal/api/singbox_selective.go` + `internal/server/server.go`
- [x] `GET  /api/singbox/router/selective/status`
- [x] `POST /api/singbox/router/selective/install-deps`
- [x] `POST /api/singbox/router/selective/rebuild`
- [x] `selectiveHandler` поле + сеттер + регистрация маршрутов в server.go

---

## Фронтенд

### 12. `frontend/src/lib/types.ts`
- [x] `SelectiveStatus` interface
- [x] `SelectiveProgress` interface
- [x] `selectiveBypass?: boolean` в `SingboxRouterSettings`

### 13. `frontend/src/lib/api/client.ts`
- [x] `singboxRouterSelectiveStatus()`
- [x] `singboxRouterSelectiveInstallDeps()`
- [x] `singboxRouterSelectiveRebuild()`

### 14. `frontend/src/lib/api/events.ts`
- [x] `onSingboxRouterSelectiveProgress`
- [x] `onSingboxRouterSelectiveStatus`
- [x] зарегистрированы в `connectSSE`

### 15. `frontend/src/lib/stores/selectiveBypass.ts`
- [x] store с `status`, `progress`, `applyStatus`, `applyProgress`

### 16. `frontend/src/routes/+layout.svelte`
- [x] SSE wiring: `onSingboxRouterSelectiveProgress` → `selectiveBypass.applyProgress`
- [x] SSE wiring: `onSingboxRouterSelectiveStatus` → `selectiveBypass.applyStatus`

### 17. `frontend/src/lib/components/sb-router/SelectiveRebuildModal.svelte`
- [x] Модалка с 3 шагами (collecting / resolving / populating)
- [x] Итог: done (зелёный) / error (красный)
- [x] Кнопка «Закрыть» после завершения

### 18. `frontend/src/lib/components/sb-router/StatusDrawer.svelte`
- [x] Секция «Селективный перехват» в expert-режиме
- [x] Toggle disabled когда `!available`
- [x] Если `!available`: текст + кнопка «Установить ipset»
- [x] Если `available && enabled`: счётчик записей + дата обновления + кнопка «Пересобрать»
- [x] SelectiveRebuildModal подключён

---

## Осталось

Всё выполнено ✅

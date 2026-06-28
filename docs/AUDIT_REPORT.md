# Комплексный аудит WDTT — 28.06.2026

## Сводка

| Метрика | Значение | Статус |
|---------|----------|--------|
| server.go | 2138 строк, 84 функции, 11 типов | — |
| server_test.go | 848 строк, 38 тестов | ✅ |
| log.Fatalf вне main() | 1 (line 374) | 🔴 |
| goto | 1 (line 1231) | 🔴 |
| interface{} | 25 использований | 🟡 |
| defer recover() в горутинах | **0** | 🔴 |
| Игнорирование ошибок (json.Marshal) | 4 | 🟡 |
| Игнорирование ошибок (exec.Command) | 3 | 🟡 |
| Открытые Write() без проверки | 8 | 🟡 |
| iptables cleanup при shutdown | **нет** | 🔴 |
| YAML warnings | 2 (document-start, truthy) | ⚪ |
| ShellCheck warnings | 2 | ⚪ |

## 🔴 Критические (7)

| # | Проблема | Файл:строка | Фикс |
|---|---------|-------------|------|
| C-01 | Нет recover() в 5 горутинах | server.go:1493,1521-1523,1656-1657 | Обёртка с defer recover() |
| C-02 | Главный пароль в /list и wdtt:// | server.go:641,940 | maskPassword() |
| C-03 | Нет очистки iptables при shutdown | server.go:1270-1340 | cleanupNAT() + defer |
| C-04 | authClient возвращает срез локального buf | server.go:1673 | make + copy |
| C-05 | generatePassword детерминирован при ошибке | server.go:152-157 | log.Fatalf вместо fallback |
| C-06 | log.Fatalf в refreshWrapKeysFromDBLocked | server.go:374 | Возвращать error |
| C-07 | Нет replay protection в obfsUnwrapPacket | server.go:1979-2017 | sync.Map для lastSeq |

## 🟡 Высокие (8)

| # | Проблема | Файл:строка | Фикс |
|---|---------|-------------|------|
| H-01 | Контекст не проброшен в botLoop | server.go:1523 | http.NewRequestWithContext |
| H-02 | Игнорирование ошибок Write() | server.go:1597,1699,1705,1736,1738,1744,1747,1767 | Проверка + лог |
| H-03 | saveDB() без tmp+rename | server.go:378-381 | Write tmp → Rename |
| H-04 | Игнорирование json.Marshal | server.go:379,990,1011,1122 | Проверка ошибок |
| H-05 | Игнорирование exec.Command.Run | server.go:1274,1314,1315 | Логирование ошибок |
| H-06 | interface{} → any (Go 1.25) | 25 мест | sed-замена |
| H-07 | goto generate | server.go:1231 | Выделить в функцию |
| H-08 | botLoop без ctx висит 65s | server.go:417 | См. H-01 |

## 🟡 Средние (7)

| # | Проблема | Файл:строка | Фикс |
|---|---------|-------------|------|
| M-01 | 8 make([]byte) в hot path | server.go:77,150,214,1019,1665,1927,1958,2073 | sync.Pool / [N]byte |
| M-02 | Единичный http.Client | server.go:171,436,1001 | var httpClient |
| M-03 | crypto/rand для jitter | server.go:99 | math/rand/v2 |
| M-04 | resp.Body.Close без проверки | server.go:431,471 | _ = resp.Body.Close() |
| M-05 | Нет очистки aeadCache | server.go:1865 | Range + Delete |
| M-06 | deviceID без валидации | server.go:1684-1685 | sanitizeDeviceID |
| M-07 | ctx.Done не прерывает | server.go:1560,1604 | return из handleConn |

## ⚪ Низкие (5)

| # | Проблема | Файл:строка | Фикс |
|---|---------|-------------|------|
| L-01 | YAML: нет ---, line too long | .github/workflows/ci.yml | Форматирование |
| L-02 | ShellCheck | config/wdtt-start.sh:28, scripts/deploy.sh:37 | Кавычки, stat |
| L-03 | Нет проверки TUN | server.go в startUserspaceWG() | os.Stat(/dev/net/tun) |
| L-04 | config.json.example неполный | config/config.json.example | +config_dir |
| L-05 | Нет тестов handleConn/authClient | server_test.go | Mock-интерфейсы |

## Результаты инструментов

| Инструмент | Результат |
|-----------|-----------|
| `go build` | 🔴 Нет Go на локальной машине |
| `go vet` | 🔴 Нет Go |
| `go test` | 🔴 Нет Go |
| `golangci-lint` | 🔴 Не установлен |
| `yamllint` | ✅ 2 warnings |
| `shellcheck` | ✅ 2 warnings |
| `Makefile syntax` | ✅ Валиден |
| `.gitignore duplicates` | ✅ Нет |
| `Trailing whitespace` | ✅ Нет |
| `Tab/space mix` | ✅ Нет |

**Всего: 27 проблем** (7 critical, 8 high, 7 medium, 5 low)

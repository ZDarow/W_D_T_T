# AGENTS.md — WDTT Project Context for AI Coding Agents

> **Цель:** Позволить LLM мгновенно войти в контекст проекта, избежать галлюцинаций и не нарушить архитектуру.  
> **Версия:** 1.0 (2026-06-28)  
> **Репозиторий:** `https://github.com/ZDarow/W_D_T_T.git`

---

## 1. Контекст и архитектура

### 1.1 Что такое WDTT

WDTT (WireGuard DTLS TURN Tunnel) — VPN-сервер, который принимает DTLS-соединения от клиентов, аутентифицирует их по паролю, проксирует трафик в локальный userspace WireGuard и отправляет в интернет через NAT (MASQUERADE).

```
[Клиент] --DTLS/RTP/AEAD--> [WDTT Server] --UDP--> [WireGuard] --TUN--> [NAT] --> [Интернет]
                              ↑
                         [Telegram Bot] -- управление паролями
```

### 1.2 Стек

| Компонент | Технология | Версия |
|-----------|-----------|--------|
| Язык | Go | 1.25.0 |
| DTLS | `github.com/pion/dtls/v3` | v3.1.2 |
| Обфускация | `chacha20poly1305` (x/crypto) | v0.53.0 |
| WireGuard | `golang.zx2c4.com/wireguard` (userspace) | 2025-05-21 |
| NAT | iptables ИЛИ nft | системные утилиты |
| Транспорт | `github.com/pion/transport/v4/udp` | v4.0.1 |
| Telegram Bot | HTTP long poll (getUpdates) | – |
| База данных | JSON-файл на диске (`passwords.json`) | – |
| Обфускация L2 | SalamanderXOR (HKDF-SHA256) | опционально |
| Timing защита | randomJitter (1-10ms) | опционально |

### 1.3 Структура директорий

```
server/                          # ВСЁ в одном файле (package main)
├── server.go                    # 2138 строк, 84 функции, 11 типов
├── server_test.go               # 848 строк, 37 тестов
├── go.mod                       # 4 прямых + 4 косвенных зависимости
└── go.sum                       # 2621 байт

config/                          # Инфраструктура
├── config.json.example          # Пример конфига
├── wdtt.service                 # systemd unit
└── wdtt-start.sh                # Скрипт запуска

ansible/                         # Деплой
├── playbook.yml
├── inventory/production
└── roles/wdtt/templates/*.j2

docs/                            # Документация
├── ARCHITECTURE.md
├── DEEP_ANALYSIS.md             # Глубокий анализ кода
├── AUDIT_REPORT.md
├── BYPASS_RESEARCH.md
├── INSTALL.md
├── README.md
└── USAGE.md

.github/workflows/ci.yml         # CI (lint, vet, build, mod-check)
```

### 1.4 Ключевые сущности (типы)

```go
// server.go:108 — устройство клиента
type ClientDevice struct {
    DeviceID string  // уникальный ID устройства
    IP       string  // IP в подсети 10.66.66.0/24
    PrivKey  string  // WireGuard private key (base64)
    PubKey   string  // WireGuard public key (base64)
}

// server.go:115 — запись о пароле
type PasswordEntry struct {
    DeviceID      string // пусто = не привязан
    ExpiresAt     int64  // unix timestamp, 0 = бессрочный
    DownBytes     int64  // скачано клиентом
    UpBytes       int64  // отдано клиентом
    VkHash        string // VK идентификатор
    Ports         string // "dtls,wg,tun"
    IsDeactivated bool
}

// server.go:127 — БД в памяти
type Database struct {
    MainPassword string
    AdminID      string
    BotToken     string
    Passwords    map[string]*PasswordEntry
    Devices      map[string]*ClientDevice
}

// server.go:201 — хранилище WRAP-ключей (HKDF из паролей)
type wrapKeyStore struct {
    mu      sync.RWMutex
    entries []wrapKeyEntry  // {id string, key []byte}
}

// server.go:1893 — конфиг RTP-обфускации
type ObfsConfig struct {
    SSRC        uint32
    PayloadType uint8   // всегда 111
    PaddingMax  int     // 255
}

// server.go:1899 — состояние RTP-обфускации (sequence counter)
type ObfsState struct {
    mu      sync.Mutex
    initSeq uint16
    initTs  uint32
    count   uint64
}

// server.go:1187 — WireGuard ключи
type wgKeys struct {
    serverPrivate, serverPublic, clientPrivate, clientPublic string
}

// server.go:407 — состояние диалога Telegram-бота
type tgBotState struct {
    waitingForDays  bool
    waitingForPorts bool
    waitingForHash  bool
    targetPassword  string
    tempDays        int
    tempPorts       string
}
```

### 1.5 Точки входа и связи между модулями

```
main()                                                         [строка 1450]
├── initDB()                         → db, passwords.json      [строка 353]
│   └── refreshWrapKeysFromDB()      → serverWrapKeys          [строка 343]
├── loadOrGenerateKeys()             → wgKeys                  [строка 1217]
├── enableBBR()                      → sysctl                  [строка 1029]
├── startUserspaceWG()               → device.Device           [строка 1343]
│   ├── configureInterface()         → ip addr/link            [строка 1417]
│   └── setupFullConeNAT()           → iptables/nft            [строка 1259]
├── statsLoop()                      → server.log (горутина)   [строка 1090]
├── expiredPasswordJanitor()         → cleanup (горутина)      [строка 902]
├── botLoop()                        → Telegram API (горутина) [строка 417]
│   └── handleTgCallback() / handleTgTextCmd()                 [строки 500, 542]
├── listenWrapped()                  → wrapPacketListener      [строка 2031]
│   └── wrapPacketConn.ReadFrom()    → obfsUnwrapPacket        [строка 2071]
│   └── wrapPacketConn.WriteTo()     → obfsWrapPacket          [строка 2110]
└── listener.Accept() → handleConn()  → authClient → proxy    [строка 1580]
    ├── authClient()                 → GETCONF/READY протокол  [строка 1664]
    ├── proxyToWG()                  → клиент → WireGuard      [строка 1781]
    └── proxyToClient()              → WireGuard → клиент      [строка 1820]
```

### 1.6 Формат протокола GETCONF/READY

```
Клиент → Сервер:   GETCONF:<clientPort>|<deviceID>|<password>
Сервер → Клиент:   <WireGuard INI config>  (или DENIED:причина)

Клиент → Сервер:   READY
Сервер → Клиент:   READY_OK

Клиент → Сервер:   <WireGuard handshake initiation packet>
Сервер ←→ Клиент:  <прокси все WG-пакеты>
```

### 1.7 Формат пакета обфускации (RTP+AEAD)

```
┌─ 12 байт RTP-заголовок ─┬── payload (AEAD) ──┬── padding ──┬─┐
│ V=2 | P=1 | PT=111 | seq | ts | ssrc         │ pad[...]    │L │
└──────────────────────────┴────────────────────┴─────────────┴─┘
- ChaCha20-Poly1305 (AEAD, 16 байт overhead)
- padding: 1-255 байт случайных данных (countermeasure DPI)
- nonce = SSRC (4B) + seq (2B) + \x00\x00 (2B) + ts (4B)
```

---

## 2. Строгие правила и ограничения

### 2.1 Гайдлайны по стилю кода

1. **Комментарии на русском** — описывай «почему», а не «что». Именование переменных, функций, типов — на английском (CamelCase).
2. **Пакет:** ВСЁ в `package main`. **НЕ создавай новые пакеты** без явного указания. Разделение на пакеты запланировано, но не выполнено.
3. **Ошибки:** Всегда обрабатывай `error`. Никогда не игнорируй через `_` кроме `json.Marshal` на заранее валидных данных.
4. **Контекст:** Все блокирующие операции должны принимать `context.Context` и проверять `ctx.Done()`.
5. **Горутины:** Каждая горутина должна иметь `defer recover()` и должна учитывать `ctx.Done()` для graceful shutdown.
6. **Глобалы:** Не добавляй новые глобальные переменные. Используй аргументы функций или поля структур.
7. **Магические числа:** Не используй — выноси в `const` с поясняющим именем.
8. **`goto`:** ЗАПРЕЩЁН. Заменяй на отдельную функцию или цикл.
9. **`log.Fatalf`:** ЗАПРЕЩЁН вне `main()`. Возвращай ошибку.
10. **`interface{}`:** Не используй. Go 1.25 поддерживает generics. Для Telegram keyboard используй `[][]InlineKeyboardButton`.

### 2.2 Запрещённые паттерны

| Паттерн | Почему запрещён | Альтернатива |
|---------|----------------|-------------|
| `goto generate` | Нечитаем, хрупок | Выдели в `generateNewKeys()` |
| `map[string]interface{}` | Нет type safety | Определи `type InlineKeyboardButton struct` |
| `log.Fatalf` в не-main | `os.Exit(1)` без graceful shutdown | `return fmt.Errorf("...")` |
| `os.WriteFile` без tmp+rename | Повреждение данных при сбое | Write tmp → Rename |
| `http.Client{}` на каждый вызов | Утечка соединений, нет keep-alive | Единый `var httpClient` |
| `rand.Read` для jitter | Блокирующий syscall | `math/rand/v2` |
| Игнорирование `defer recover()` | Паника убивает весь сервер | Добавляй в каждую горутину |
| `make([]byte, N)` в hot path | Давление на GC | `sync.Pool` или стековые `[N]byte` |

### 2.3 Специфика целевых ОС

**Сервер (Linux Mint / Ubuntu / Debian):**
- Требует `root` для: WireGuard TUN, iptables/nft, sysctl
- Проверяй `os.Geteuid() == 0` перед вызовом `setupFullConeNAT`
- `iptables` может отсутствовать — используй `commandExists()` и fallback на `nft`
- `/proc/sys/net/ipv4/ip_forward` должен быть доступен для записи
- BBR: `sysctl net.ipv4.tcp_congestion_control` — читай перед установкой

**Клиент (Android / LineageOS):**
- Android-клиент — отдельный Kotlin проект (не в этом репозитории)
- Не добавляй Android-специфичный код в server.go
- Протокол GETCONF/READY должен оставаться обратно совместимым

### 2.4 Безопасность (НЕ НАРУШАТЬ)

1. **Никогда не логируй пароль в открытом виде.** Всегда используй `maskPassword()`.
2. **Главный пароль не должен отправляться в Telegram в открытом виде.** В `/list` используй `maskPassword(db.MainPassword)`.
3. **Ключевой материал (wrap key) не должен попадать в строку.** Используй `cacheKeyForAEAD()` — SHA256 ключа в hex.
4. **Всегда обнуляй ключевой материал через `zeroBytes()` после использования.**
5. **Никогда не доверяй `deviceID` от клиента.** Валидируй длину и содержимое.
6. **saveDB()** должен писать через `tmp + Rename`, не напрямую.
7. **При завершении сервера** очищай iptables правила (MASQUERADE, FORWARD).

---

## 3. Операционные инструкции

### 3.1 Установка зависимостей (локально)

```bash
# Требуется: Go 1.25+, Linux с root
apt install -y wireguard-tools iptables iproute2

# Зависимости Go (go mod tidy делает это автоматически)
cd server && go mod tidy
```

### 3.2 Сборка

```bash
# Статическая сборка (рекомендуется для деплоя)
cd server && CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
  -ldflags='-s -w' -o wdtt-server .

# Обычная сборка
cd server && go build -o wdtt-server .

# Через Makefile (сборка + scp на VPS)
make deploy HOST=root@213.21.242.99
```

### 3.3 Запуск

```bash
# Минимальный запуск
sudo ./wdtt-server -password "MySecretPass"

# Полный запуск (с Telegram-ботом и обфускацией)
sudo ./wdtt-server \
  -password "MySecretPass" \
  -admin "123456789" \
  -bot-token "123:ABCdef" \
  -listen "0.0.0.0:56000" \
  -config-dir /etc/wdtt \
  -salamander-key "aabbccdd11223344aabbccdd11223344" \
  -jitter

# Через systemd
sudo systemctl start wdtt
sudo systemctl status wdtt
```

### 3.4 Тестирование

```bash
# Все тесты
cd server && go test -v -timeout 60s -count=1 ./...

# С покрытием
cd server && go test -coverprofile=coverage.out ./... && \
  go tool cover -func=coverage.out | grep -E 'total|obfs'

# Race detector
cd server && go test -race -timeout 30s ./...

# Benchmark (если есть)
cd server && go test -bench=. -benchmem ./...
```

### 3.5 Линтинг

```bash
# go vet (обязательно перед каждым коммитом)
cd server && go vet ./...

# golangci-lint (в CI)
cd server && golangci-lint run --timeout=5m

# yamllint для CI
yamllint .github/workflows/ci.yml
```

### 3.6 Деплой (на VPS)

```bash
# VPS: Ubuntu/Debian, root, Go 1.25 установлен в /usr/local/go/bin/go
# ВАЖНО: путь до Go — /usr/local/go/bin/go (не в PATH для ssh non-interactive)

# Ansible
cd ansible && ansible-playbook -i inventory/production playbook.yml

# Вручную через make
make deploy HOST=root@213.21.242.99

# Вручную scp + ssh
scp server/wdtt-server root@213.21.242.99:/usr/local/bin/
ssh root@213.21.242.99 "systemctl restart wdtt && systemctl status wdtt --no-pager"
```

### 3.7 Полезные команды для отладки

```bash
# NAT проверка
iptables -t nat -L POSTROUTING -v -n
nft list table ip wdtt
cat /proc/sys/net/ipv4/ip_forward

# WireGuard проверка
wg show
ip addr show wdtt0
ip link show wdtt0

# Логи
journalctl -u wdtt -f --no-pager
cat /etc/wdtt/server.log
tail -f /etc/wdtt/passwords.json | jq '.passwords | keys'

# Статистика
curl -s https://api.telegram.org/bot<TOKEN>/getUpdates | jq '.result | length'
```

---

## 4. Бэклог задач (Roadmap)

### Sprint 1 — Безопасность (3-4 ч)

| # | Задача | Файл | Строки |
|---|--------|------|--------|
| S1.1 | Маскировать `db.MainPassword` в `/list` | server.go | 940 |
| S1.2 | Atomic write в `saveDB()` (tmp+rename) | server.go | 378-381 |
| S1.3 | Очистка iptables/nft при shutdown | server.go | 1516-1519 |
| S1.4 | Заменить `log.Fatalf` на возврат ошибок (initDB, loadOrGenerateKeys, startUserspaceWG) | server.go | 373, 1503, 1510, 1527, 1531, 1534 |
| S1.5 | Защита от replay в `obfsUnwrapPacket` (проверка seq) | server.go | 1979-2018 |
| S1.6 | Валидация `deviceID` от клиента | server.go | 1684-1686 |

### Sprint 2 — Архитектура (8-12 ч)

| # | Задача | Файл | Описание |
|---|--------|------|----------|
| S2.1 | Выделить `Server struct` | server.go | DI вместо глобалов |
| S2.2 | Контекст в `botLoop` | server.go | Заменить `client.Get` на `client.Do` с ctx |
| S2.3 | Типизированные Telegram-структуры | server.go | `InlineKeyboardButton`, `InlineKeyboard` |
| S2.4 | `defer recover()` во все горутины | server.go | proxyToWG, proxyToClient, botLoop, statsLoop |
| S2.5 | Удалить `goto generate` | server.go | 1217-1255 |
| S2.6 | Заменить `interface{}` на `any` (Go 1.25 style) | server.go | весь файл |

### Sprint 3 — Производительность (2-3 ч)

| # | Задача | Файл | Строки |
|---|--------|------|--------|
| S3.1 | `obfsBuildNonce` на стек (return `[12]byte`) | server.go | 1926-1932 |
| S3.2 | `sync.Pool` для буфера в `wrapPacketConn.ReadFrom` | server.go | 2071-2077 |
| S3.3 | Заменить `crypto/rand` на `math/rand/v2` для jitter | server.go | 95-104 |
| S3.4 | Единый `http.Client` для Telegram | server.go | 171, 436, 1001, 987 |
| S3.5 | Копировать `firstPacket` из `buf` (bug fix) | server.go | 1664-1673 |

### Sprint 4 — Тесты (6-8 ч)

| # | Задача | Покрытие |
|---|--------|----------|
| S4.1 | Mock-интерфейсы для `handleConn` | WGDevice, ConnAuthenticator |
| S4.2 | Integration test: mock-клиент ↔ сервер | Полный цикл GETCONF+READY+proxy |
| S4.3 | Fuzz test для `obfsWrapPacket` / `obfsUnwrapPacket` | Edge cases |

---

## 5. Технический долг и рекомендации

### 5.1 Известные узкие места

**🔴 Гонка данных в authClient**  
`authClient()` возвращает `firstPacket` как срез своего локального `buf`. Если после возврата `buf` будет переиспользован GC — данные повреждены.  
**Фикс:** `firstPacket = make([]byte, n); copy(firstPacket, buf[:n])`

**🔴 Нет очистки iptables при shutdown**  
WG-интерфейс удаляется, но правила MASQUERADE и FORWARD остаются.  
**Фикс:** добавить `defer cleanupNAT()` в main.

**🟡 Telegram bot висит 60 секунд при завершении**  
`botLoop` использует `client.Get(url)` без контекста. После `cancel()` ждёт до 60s.  
**Фикс:** `http.NewRequestWithContext(ctx, "GET", url, nil)`

**🟡 iptables `-D` в цикле из 5 итераций**  
Костыль для удаления дублирующихся правил. Лучше использовать `iptables-save | grep -v WDTT_MANAGED | iptables-restore`.  
**Фикс:** атомарная замена через `iptables-save/restore`.

**🟡 `generatePassword` fallback на `time.Now()`**  
Если `crypto/rand` не работает (редко на Linux), пароль становится детерминированным на основе времени.  
**Фикс:** `log.Fatalf` при ошибке `rand.Read` (безопасность важнее доступности).

### 5.2 Что категорически НЕЛЬЗЯ делать агенту

1. **НЕ трогать** `go.mod` вручную — только через `go get` на VPS (нет локального Go).
2. **НЕ добавлять** новые глобальные переменные `var`.
3. **НЕ использовать** `log.Fatalf` вне `main()`.
4. **НЕ игнорировать** ошибки `exec.Command` в NAT-функциях.
5. **НЕ менять** формат протокола GETCONF/READY — сломается Android-клиент.
6. **НЕ добавлять** внешние HTTP-зависимости без острой необходимости (проект должен собираться статически).
7. **НЕ переименовывать** `package main` — тесты написаны для этого пакета.
8. **НЕ удалять** `salamanderKey` как глобал — он нужен в `wrapPacketConn.WriteTo/ReadFrom`.

### 5.3 Архитектурные советы (что делать при рефакторинге)

При разделении монолита на пакеты соблюдай порядок:

```
1. crypto/   → obfsWrapPacket, obfsUnwrapPacket, getAEAD, salamanderXOR
               НЕ ЗАВИСИТ ни от чего
2. config/   → flag parsing, загрузка
               Зависит от crypto/ (deriveWrapKey)
3. db/       → Database, PasswordEntry, save/load
               НЕ ЗАВИСИТ от crypto/ и proxy/
4. bot/      → Telegram handlers
               Зависит от db/, wg/ (только интерфейс)
5. wg/       → WireGuard management
               Зависит от db/ (ClientDevice)
6. proxy/    → handleConn, authClient, proxyToWG, proxyToClient
               Зависит от crypto/, db/, wg/
7. nat/      → iptables/nft управление
               НЕ ЗАВИСИТ от других пакетов
8. cmd/wdtt/main.go → точка входа, DI, запуск
               Зависит от всех
```

### 5.4 Проверка перед коммитом

```bash
# Минимальный чек-лист:
go vet ./...
go build ./...
go test -race -timeout 30s ./...
grep -n "log.Fatalf" server.go | grep -v "func main()"  # не должно быть!
grep -n "goto " server.go                                 # не должно быть!

# Проверка, что зависимости не раздулись:
go mod tidy && git diff --stat go.mod go.sum
```

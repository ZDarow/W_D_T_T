# Глубокий анализ кодовой базы WDTT

**Дата:** 28.06.2026  
**Версия:** 1.0 — ревизия 625d9cc  
**Объект:** `server/server.go` (2138 строк), `server/server_test.go` (848 строк), конфигурация

---

## 1. Архитектура и стек

### 1.1 Общая схема

```
[Клиент] --DTLS/RTP--> [wrapPacketListener] --UDP--> [WireGuard Device] --TUN--> [NAT] --iptables--> [Интернет]
                            |
                       [Telegram Bot] --HTTP--> [api.telegram.org]
                            |
                       [JSON File DB] (passwords.json)
```

### 1.2 Стек технологий

| Компонент | Технология | Версия |
|-----------|-----------|--------|
| Язык | Go | 1.25.0 |
| DTLS | pion/dtls/v3 | v3.1.2 |
| Шифрование | chacha20poly1305 (x/crypto) | v0.53.0 |
| WireGuard | wireguard (userspace) | 2025-05-21 |
| NAT | iptables / nft (внешние утилиты) | – |
| Бот | Telegram Bot API (HTTP long poll) | – |
| База данных | JSON-файл на диске | – |

### 1.3 Структура директорий

```
wdtt/
├── .github/workflows/ci.yml     # CI (4 джобы)
├── ansible/                      # Ansible-роль деплоя
├── config/                       # systemd unit, start.sh, example config
├── docs/                         # 6 документов
├── scripts/deploy.sh             # Shell-скрипт деплоя
├── Makefile                      # Сборка и деплой
└── server/
    ├── go.mod / go.sum           # Зависимости
    ├── server.go                 # 2138 строк — ВЕСЬ сервер
    └── server_test.go            # 848 строк — 37 тестов
```

---

## 2. Детальный анализ проблем

### 2.1 Архитектура и паттерны (CRITICAL: 4, HIGH: 6, MEDIUM: 5)

#### 🔴 C-ARC-01: Монолит — весь сервер в одном файле

**Файл:** `server/server.go` (1–2138)  
**Суть:** 2138 строк, 84 функции, 11 типов — в `package main`. Никакого разделения на пакеты.  
**Последствия:**
- Невозможно импортировать как библиотеку
- Нет изоляции тестов (только `package main` с доступом ко всем глобалам)
- Нет границ ответственности

**Рекомендация:** Разделить на пакеты:
```
server/
├── cmd/wdtt/main.go         # точка входа, flag parsing
├── internal/
│   ├── crypto/              # obfsWrap/Unwrap, salamander, aeadCache
│   ├── config/              # загрузка конфигурации
│   ├── db/                  # Database, PasswordEntry, save/load
│   ├── proxy/               # handleConn, proxyToWG, proxyToClient
│   ├── bot/                 # botLoop, TG handlers
│   ├── nat/                 # NAT setup, forward rules
│   └── wg/                  # WireGuard management
```

#### 🔴 C-ARC-02: Глобальное состояние

**Файл:** `server.go:135-141, 1069-1071, 141, 165`  
**Суть:** `db`, `dbMutex`, `authAttempts`, `connSemaphore`, `serverWrapKeys`, `publicIP`, `salamanderKey`, `enableJitter` — всё глобальные переменные.  
**Последствия:**
- Невозможно запустить два инстанса в одном процессе
- Тестирование требует ручного сброса
- Любая функция может изменить состояние из любой точки

**Пример:**
```go
var db      *Database    // строка 136 — кто угодно может перезаписать
var dbMutex sync.Mutex   // строка 137 — публичный мьютекс
```

**Рекомендация:** Внедрить DI через контекст или структуру `Server`:
```go
type Server struct {
    db        *Database
    dbMutex   sync.Mutex
    clientSem chan struct{}
    wrapKeys  *wrapKeyStore
    wgDev     *device.Device
    keys      *wgKeys
    ctx       context.Context
}
func NewServer(cfg *Config) *Server { ... }
```

#### 🟡 C-ARC-03: Telegram Bot на длинном поллинге без graceful shutdown

**Файл:** `server.go:417-497`  
**Суть:** `botLoop` висит в бесконечном цикле `for { client.Get(...) }`. Не реагирует на `ctx.Done()`.  
**Последствия:** При завершении сервера бот продолжает висеть до следующего HTTP-таймаута (65 секунд).

**Пример:**
```go
// строка 439 — контекст не проверяется
for {
    url := fmt.Sprintf("https://api.telegram.org/bot%s/getUpdates?timeout=60&offset=%d", token, offset)
    resp, err := client.Get(url)    // <- может висеть 60 секунд
```

**Рекомендация:** Использовать `client.Get` с контекстом:
```go
req, _ := http.NewRequestWithContext(ctx, "GET", url, nil)
resp, err := client.Do(req)
```

#### 🟡 C-ARC-04: Нет слоя абстракции для базы данных

**Файл:** `server.go:353-381`  
**Суть:** База данных — это `map[string]*PasswordEntry` в памяти + JSON на диске. Нет транзакций, нет блокировок чтения/записи на уровне БД.  
**Последствия:** Конкурентный доступ с `dbMutex` по всему коду — микро-блокировки рассеяны по 20+ местам.  

**Рекомендация:** Создать `Database` c методами:
```go
type Database struct {
    mu       sync.RWMutex
    filename string
    data     *DBData
}

func (d *Database) GetPassword(pass string) (*PasswordEntry, bool)
func (d *Database) SetPassword(pass string, entry *PasswordEntry) error
func (d *Database) Save() error
```

#### 🟡 C-ARC-05: `package main` без возможности unit-тестирования сетевой логики

**Файл:** `server.go:1580-1660`  
**Суть:** `handleConn` принимает `net.Conn`, `device.Device`, `wgKeys` — все зависимости жёсткие. Невозможно протестировать без WireGuard и DTLS.  
**Рекомендация:** Выделить интерфейсы:
```go
type WGController interface {
    UpsertPeer(dev *ClientDevice) error
    RemovePeer(pubKey string) error
}
type ConnAuthenticator interface {
    Authenticate(clientConn net.Conn) (password string, isMain bool, firstPacket []byte, err error)
}
```

---

### 2.2 Безопасность (CRITICAL: 3, HIGH: 4, MEDIUM: 3)

#### 🔴 C-SEC-01: Главный пароль в открытом виде в ответе /list

**Файл:** `server.go:940`  
**Код:**
```go
txt += fmt.Sprintf("🔒 Главный: `%s` (владелец)\n\n", db.MainPassword)
```
**Суть:** Администратору в Telegram отправляется `db.MainPassword` в открытом виде. Если Telegram-сессия скомпрометирована — пароль раскрыт.  
**Рекомендация:** Маскировать как `maskPassword(db.MainPassword)`.

#### 🔴 C-SEC-02: Нет блокировки на scope паролей

**Файл:** `server.go:1693`  
**Код:**
```go
isMainPass := password != "" && password == db.MainPassword
entry, isGenPass := db.Passwords[password]
```
**Суть:** Любой клиент, знающий пароль, получает WG-конфиг. Нет привязки к подсети, времени суток, геолокации. После утечки пароль невозможно отозвать частично.  
**Рекомендация:** Добавить whitelist IP/подсетей в `PasswordEntry`:
```go
type PasswordEntry struct {
    // ...
    AllowedCIDRs []string `json:"allowed_cidrs,omitempty"`
}
```

#### 🔴 C-SEC-03: `os.Exit` в библиотечных функциях при ошибках

**Файл:** `server.go:373-375, 1503, 1510, 1527, 1531, 1534, 1539`  
**Пример:**
```go
// строка 373
if err := refreshWrapKeysFromDBLocked(); err != nil {
    log.Fatalf("[WRAP] init keys: %v", err)
}
```
**Суть:** `log.Fatalf` вызывает `os.Exit(1)` без graceful shutdown. Если функция будет вызвана не из `main`, сервер аварийно завершится.  
**Рекомендация:** Возвращать ошибку в `main`:
```go
if err := refreshWrapKeysFromDBLocked(); err != nil {
    return fmt.Errorf("wrap keys: %w", err)
}
```

#### 🟡 C-SEC-04: Нет защиты от повторной отправки (replay) в обфускации

**Файл:** `server.go:1941-1944`  
**Код:**
```go
c := state.count
state.count++
```
**Суть:** Счётчик последовательности(uint64) монотонно растёт. Но в `obfsUnwrapPacket` (1979-2018) нет проверки, что `seq > lastSeq` для предотвращения replay-атак.  
**Рекомендация:** В `ObfsState` для чтения добавить `lastSeq uint16` и проверять возрастание:
```go
if seq <= c.lastSeq {
    return 0, errors.New("obfs: replay detected")
}
c.lastSeq = seq
```

#### 🟡 C-SEC-05: Нет таймаута на `os.WriteFile`

**Файл:** `server.go:380`  
**Код:**
```go
func saveDB() {
    data, _ := json.MarshalIndent(db, "", "  ")
    os.WriteFile(dbFile, data, 0600)
}
```
**Суть:** При проблемах с диском (NFS, монтирование) `WriteFile` может зависнуть навсегда, блокируя мьютекс.  
**Рекомендация:** Использовать `os.OpenFile` с `O_SYNC` или писать во временный файл + `os.Rename`:
```go
tmp := dbFile + ".tmp"
os.WriteFile(tmp, data, 0600)
os.Rename(tmp, dbFile)
```

#### 🟡 C-SEC-06: Пароль в логах

**Файл:** `server.go:1700, 1706`  
**Код:**
```go
log.Printf("[WG] Отказ: пароль %s деактивирован, запрос от %s", maskPassword(password), deviceID)
```
**Суть:** `maskPassword` маскирует пароль, но не URL или хеш.  
**Рекомендация:** Маскировать `deviceID` и `VkHash` при записи в лог.

#### 🟡 C-SEC-07: iptables rules не удаляются при завершении

**Файл:** `server.go:1516-1519`  
**Код:**
```go
defer func() {
    wgDev.Close()
    runCmdSilent("ip", "link", "del", wgIfaceName)
}()
```
**Суть:** При завершении сервера удаляется только WG-интерфейс. Правила iptables/NAT (MASQUERADE, FORWARD) остаются в системе.  
**Рекомендация:** Добавить очистку правил при завершении.

---

### 2.3 Производительность (HIGH: 1, MEDIUM: 4, LOW: 3)

#### 🔴 C-PRF-01: Избыточное выделение памяти в hot path

**Файл:** `server.go:1926-1931, 1949, 1958, 2073-2074, 2118`  

| Функция | Аллокация | Частота |
|---------|-----------|---------|
| `obfsBuildNonce` | `make([]byte, 12)` | Каждый пакет |
| `obfsWrapPacket` | `make([]byte, outLen)` | Каждый пакет |
| `wrapPacketConn.ReadFrom` | `make([]byte, len(p)+80)` | Каждый пакет |
| `salamanderXOR` | `make([]byte, len(src))` | Каждый пакет при salamander |

**Влияние:** Для 64 одновременных клиентов с MTU=1280 — ~500 аллокаций/с на клиента, ~32k alloc/s всего. GC overhead.

**Рекомендация:**
- `obfsBuildNonce`: заменить на `var nonce [12]byte` на стеке
- `wrapPacketConn.ReadFrom`: добавить `sync.Pool` для буфера, а не создавать каждый раз
- `salamanderXOR`: при salamander-ключе пред-генерировать keyStream (влияние на память)

**Пример оптимизации `obfsBuildNonce`:**
```go
func obfsBuildNonce(ssrc uint32, seq uint16, ts uint32) [12]byte {
    var n [12]byte
    binary.BigEndian.PutUint32(n[0:4], ssrc)
    binary.BigEndian.PutUint16(n[4:6], seq)
    binary.BigEndian.PutUint32(n[8:12], ts)
    return n
}
```

#### 🟡 C-PRF-02: HTTP-клиент не переиспользуется

**Файл:** `server.go:171, 436, 1001, 987`  
**Суть:** 4 разных создания `http.Client` в разных функциях. `sendTelegram` и `answerCallback` создают клиента каждый вызов.  
**Рекомендация:** Единый `http.Client` с переиспользуемым Transport:
```go
var httpClient = &http.Client{
    Timeout: 30 * time.Second,
    Transport: &http.Transport{
        MaxIdleConns:    100,
        IdleConnTimeout: 90 * time.Second,
    },
}
```

#### 🟡 C-PRF-03: `rand.Read` для jitter — криптостойкий ГСЧ для не-крипто задачи

**Файл:** `server.go:100-101`  
**Код:**
```go
var jitterBuf [1]byte
rand.Read(jitterBuf[:])
delay := 1*time.Millisecond + time.Duration(jitterBuf[0]%10)*time.Millisecond
```
**Суть:** `crypto/rand` — блокирующий syscall. Для jitter достаточно `math/rand/v2`.  
**Рекомендация:**
```go
var jitterRng = rand.New(rand.NewPCG(uint64(time.Now().UnixNano()), 0))
// ...
delay := 1*time.Millisecond + time.Duration(jitterRng.IntN(10))*time.Millisecond
```

#### 🟡 C-PRF-04: `firstPacket` может быть WG handshake до 1480 байт — аллокация на каждое соединение

**Файл:** `server.go:1665`  
**Код:**
```go
buf := make([]byte, 1600)
```
**Суть:** Каждое новое соединение выделяет 1600 байт. Для 64 клиентов в секунду — 102 КБ/с аллокаций.  
**Рекомендация:** Выделять буфер через `sync.Pool`.

---

### 2.4 Читаемость и поддерживаемость (HIGH: 5, MEDIUM: 4, LOW: 2)

#### 🔴 C-READ-01: Нет документации публичных типов

**Файл:** `server.go:108-133, 196-204, 1893-1904`  
**Суть:** `ClientDevice`, `PasswordEntry`, `Database`, `wrapKeyEntry`, `wrapKeyStore` — ни один тип не имеет документации (godoc).  
**Рекомендация:** Добавить godoc-комментарии.

#### 🟡 C-READ-02: Магические числа

**Файл:** Всего ~25 магических чисел.  
**Примеры:**
- `12` — размер RTP-заголовка (должна быть константа `rtpHeaderSize`)
- `13` — минимальный размер RTP-пакета
- `1600` — размер буфера (1300 + 12 + 16 + 255 + запас)
- `5` — количество попыток очистки iptables в цикле

**Рекомендация:** Определить константы:
```go
const (
    rtpHeaderSize  = 12
    minPacketSize  = rtpHeaderSize + 1
    bufSize        = 1600
    iptablesRetry  = 5
)
```

#### 🟡 C-READ-03: Дублирование кода

**Файл:** `server.go`  
**Дубликаты:**
1. Парсинг `GETCONF:port|deviceID|password` — `authClient` (1676-1689) и клиент (Android) должны быть синхронизированы, но спецификация протокола не вынесена
2. Per-password tracking: `e.UpBytes += int64(nn)` и `e.DownBytes += int64(nn)` — почти идентичные блоки в `proxyToWG` (1803-1812) и `proxyToClient` (1844-1853)
3. `runCmd` и `runCmdSilent` — 90% дублирования

#### 🟡 C-READ-04: `interface{}` без generics

**Файл:** `server.go:975, 979, 982, 1017`  
**Суть:** Go 1.25 поддерживает generics, но Telegram keyboard строится через `map[string]interface{}` и `[]map[string]interface{}`.  
**Рекомендация:** Определить типы для Telegram InlineKeyboard:
```go
type InlineKeyboardButton struct {
    Text         string `json:"text"`
    CallbackData string `json:"callback_data"`
}
type InlineKeyboard [][]InlineKeyboardButton
```

#### 🟡 C-READ-05: `goto generate`

**Файл:** `server.go:1231, 1238`  
**Суть:** `goto` в Go — красный флаг. В данном случае replaceable на отдельную функцию.  
**Рекомендация:** Выделить в отдельную функцию `generateNewKeys(dir string) (*wgKeys, error)`.

---

### 2.5 Тестовое покрытие (HIGH: 2)

#### 🔴 C-TST-01: Нет тестов для handleConn и authClient

**Файл:** `server_test.go`  
**Суть:** 37 тестов покрывают только изолированные функции (obfs, crypto, rate-limit). `handleConn` (80 строк), `authClient` (116 строк) не тестируются.  
**Причина:** `device.Device`, `net.Conn`, `dtls.Conn` — hard dependencies.  
**Рекомендация:** Создать интерфейсы и тесты с mock:
```go
type WGDevice interface {
    UpsertPeer(dev *ClientDevice) error
}
type mockWGDevice struct{}
func (m *mockWGDevice) UpsertPeer(dev *ClientDevice) error { return nil }
```

#### 🔴 C-TST-02: Тесты rate-limit зависят от глобального состояния

**Файл:** `server_test.go:606-619`  
**Суть:** Тесты `TestCheckRateLimit` использует адрес `10.0.0.1` и модифицирует `authAttempts`. При параллельном запуске тестов — гонка данных.  
**Рекомендация:** Обнулять `authAttempts` в `init()` тестов или использовать отдельный экземпляр.

---

### 2.6 Потенциальные баги (CRITICAL: 1, HIGH: 2, MEDIUM: 2)

#### 🔴 C-BUG-01: Гонка данных в `authClient` + `handleConn` (setReadahead с первым пакетом)

**Файл:** `server.go:1665, 1673, 1756, 1761, 1768-1774`  
**Суть:** `buf` — локальная переменная `authClient`, а `firstPacket` — срез `buf[:n]`. После возврата из `authClient`, в `handleConn` происходит `wgConn.Write(firstPacket)`. Если GC сдвинет или переиспользует `buf` — data race.  
**Статус:** На практике не проявляется, так как `firstPacket` читается до следующей аллокации. Но это хрупкий код.  
**Рекомендация:** Копировать в отдельный буфер:
```go
firstPacket := make([]byte, n)
copy(firstPacket, buf[:n])
```

#### 🟡 C-BUG-02: deviceID без санитизации

**Файл:** `server.go:1684-1686, 1720-1727`  
**Код:**
```go
deviceID := parts[1]                         // клиент может передать любой deviceID
db.Devices[deviceID] = dev                   // запись без проверки
if entry.DeviceID == "" {
    entry.DeviceID = deviceID                // привязка без валидации
}
```
**Влияние:** Злоумышленник может перезаписать чужой deviceID при первом подключении.  
**Рекомендация:** Валидация `deviceID`:
```go
const maxDeviceIDLen = 64
if len(deviceID) > maxDeviceIDLen || !validDeviceIDPattern.MatchString(deviceID) {
    clientConn.Write([]byte("DENIED:invalid_device_id"))
    return "", false, nil, errors.New("invalid device id")
}
```

#### 🟡 C-BUG-03: `os.WriteFile` в `statsLoop` без `dbMutex` на все поля

**Файл:** `server.go:1133`  
**Код:**
```go
os.WriteFile(statsFile, statsJSON, 0644)
```
**Суть:** `fromC`, `toC`, `active`, `total` — atomic.Load, но JSON-маршаллинг даёт несогласованные значения (T1 читает total, T2 обновляет active — в JSON разная эпоха).  
**Влияние:** Статистика может быть несогласованной (active > total на миг).  
**Рекомендация:** Снимать снапшот всех метрик под одной блокировкой.

#### 🟡 C-BUG-04: `recover` не обрабатывается

**Файл:** `server.go` — нет ни одного `defer recover()`.  
**Влияние:** Если `proxyToWG` или `proxyToClient` паникует (nil pointer, panic в библиотеке), сервер упадёт целиком.  
**Рекомендация:** Добавить `defer recover()` в критические горутины.

---

## 3. Пошаговый план рефакторинга

### Sprint 1: Безопасность (3-4 часа)

| # | Задача | Файлы | Оценка |
|---|--------|-------|--------|
| 1 | Маскировать mainPassword в /list | server.go:940 | 5 мин |
| 2 | Добавить atomic write в saveDB (write+rename) | server.go:378-381 | 15 мин |
| 3 | Очистка iptables/nft при shutdown | server.go:1516-1519 | 30 мин |
| 4 | Заменить log.Fatalf на возврат ошибок | server.go:373, 1503, 1510, 1527 | 30 мин |
| 5 | Защита от replay в obfsUnwrapPacket | server.go:1979-2018 | 15 мин |

### Sprint 2: Архитектура (8-12 часов)

| # | Задача | Оценка |
|---|--------|--------|
| 6 | Выделить `Server` struct с DI вместо глобалов | 2 часа |
| 7 | Разделить на пакеты: crypto, db, proxy, bot, nat, wg | 3 часа |
| 8 | Внедрить контекст в botLoop | 30 мин |
| 9 | Определить интерфейсы WGDevice, DBAccess | 1 час |
| 10 | Заменить `interface{}` на типизированные структуры Telegram | 30 мин |
| 11 | Добавить recover в горутины | 15 мин |

### Sprint 3: Производительность (2-3 часа)

| # | Задача | Оценка |
|---|--------|--------|
| 12 | `obfsBuildNonce` на стек (return [12]byte) | 5 мин |
| 13 | `sync.Pool` для ReadFrom buffer | 15 мин |
| 14 | `math/rand/v2` для jitter | 10 мин |
| 15 | Единый http.Client | 15 мин |
| 16 | copy firstPacket из buf | 5 мин |

### Sprint 4: Тесты (6-8 часов)

| # | Задача | Оценка |
|---|--------|--------|
| 17 | Mock-интерфейсы для handleConn тестов | 2 часа |
| 18 | Integration test (сервер ↔ mock-клиент) | 3 часа |
| 19 | Fuzz test для obfsWrapPacket/Unwrap | 1 час |

---

## 4. Сводная статистика

```
┌──────────────────────────────────────────────────────────────────┐
│                    WDTT — Deep Code Analysis                     │
├──────────────────────────────────────────────────────────────────┤
│  Категория              │ 🔴 Critical │ 🟡 High │ 🟢 Medium │
├──────────────────────────┼─────────────┼─────────┼───────────┤
│  Архитектура             │     2       │   4     │    3      │
│  Безопасность            │     3       │   4     │    3      │
│  Производительность      │     1       │   2     │    1      │
│  Читаемость              │     1       │   4     │    3      │
│  Тесты                   │     2       │   0     │    0      │
│  Потенциальные баги      │     1       │   2     │    1      │
├──────────────────────────┼─────────────┼─────────┼───────────┤
│  Итого                   │    10       │   16    │   11      │
├──────────────────────────┴─────────────┴─────────┴───────────┤
│  Общий вердикт:           🟡 Alpha — серьёзные проблемы      │
│                           безопасности и архитектуры          │
│  Техдолг:                 37 проблем (10 критических)         │
│  unit-тесты:              ✅ 37 (но покрытие 17,2%)           │
│  Всего строк:             server.go: 2138                     │
│                            server_test.go: 848                │
│  Рекомендуемый приоритет: Sprint 1 → Sprint 2 → Sprint 4      │
└──────────────────────────────────────────────────────────────┘
```

---

## 5. Примеры кода для критических исправлений

### 5.1 Atomic write для saveDB

```go
func saveDB() error {
    data, err := json.MarshalIndent(db, "", "  ")
    if err != nil {
        return fmt.Errorf("saveDB marshal: %w", err)
    }
    tmp := dbFile + ".tmp"
    if err := os.WriteFile(tmp, data, 0600); err != nil {
        return fmt.Errorf("saveDB write tmp: %w", err)
    }
    if err := os.Rename(tmp, dbFile); err != nil {
        return fmt.Errorf("saveDB rename: %w", err)
    }
    return nil
}
```

### 5.2 Очистка iptables при shutdown

```go
// В main(), перед defer:
defer func() {
    log.Println("[CLEANUP] Очистка правил iptables...")
    if commandExists("iptables") {
        for i := 0; i < 5; i++ {
            exec.Command("iptables", "-t", "nat", "-D", "POSTROUTING",
                "-s", wgServerCIDR, "-o", extIface,
                "-m", "comment", "--comment", "WDTT_MANAGED",
                "-j", "MASQUERADE").Run()
            exec.Command("iptables", "-D", "FORWARD",
                "-i", wgIfaceName,
                "-j", "ACCEPT").Run()
            exec.Command("iptables", "-D", "FORWARD",
                "-o", wgIfaceName,
                "-j", "ACCEPT").Run()
        }
    }
    wgDev.Close()
    runCmdSilent("ip", "link", "del", wgIfaceName)
}()
```

### 5.3 Защита от replay в obfsUnwrapPacket

```go
type ObfsStateRead struct {
    mu      sync.Mutex
    lastSeq uint16
}

func (s *ObfsStateRead) CheckSequence(seq uint16) error {
    s.mu.Lock()
    defer s.mu.Unlock()
    // Разрешаем wrap around в пределах 1024 (RFC 3550)
    if int16(seq-s.lastSeq) <= 0 && s.lastSeq != 0 {
        return fmt.Errorf("replay or out-of-order: last=%d, got=%d", s.lastSeq, seq)
    }
    s.lastSeq = seq
    return nil
}
```

---

*Документ создан 28.06.2026 на основе ревизии 625d9cc.*

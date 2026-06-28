# WDTT — WireGuard over TURN Tunnel

> Версия: 2.0 (Multi-User)
> Лицензия: MIT
> Репозиторий: [github.com/ZDarow/proxy-turn-vk-android](https://github.com/ZDarow/proxy-turn-vk-android)

## Кратко

WDTT — это VPN-туннель, который прокладывает WireGuard поверх DTLS/TURN. Использует технику **RTP-обфускации** (маскировка VPN-трафика под аудио/видео поток WebRTC) и опциональную **Salamander-обфускацию** для защиты от Deep Packet Inspection (DPI).

### Схема работы

```
┌─────────────────┐     RTP/DTLS      ┌──────────────────┐     UDP      ┌────────────┐
│  Android Client │ ←───────────────── │   WDTT Server    │ ←────────── │ WireGuard  │
│  (Go + Java)    │    порт 56000      │   (server.go)    │   порт 56001 │ Userspace  │
└─────────────────┘                    └──────────────────┘              └────────────┘
        │                                       │                              │
        ├── WRAP: RTP + ChaCha20-Poly1305 AEAD   │                              │
        ├── Salamander: XOR + HKDF-SHA256 (опц.)  │                              │
        └── Jitter: 1-10ms рандомизация таймингов │                              │
```

### Особенности

- **RTP-обфускация** — каждый пакет выглядит как RTP-пакет (аудио/видео поток): RTP-заголовок + AEAD-шифрованный payload + случайный padding
- **Многопользовательский режим** — Telegram-бот для управления паролями и устройствами
- **WireGuard userspace** — не требует ядерного модуля WireGuard, работает через TUN-интерфейс
- **Автоматический NAT** — поддержка iptables/nftables MASQUERADE
- **DDoS-защита** — каждый пароль ограничен по трафику и времени жизни
- **Опциональная Salamander-обфускация** — XOR с HKDF-SHA256 для дополнительной защиты
- **Jitter** — рандомизация таймингов отправки пакетов (1-10 мс)

---

## Быстрый старт

### 1. Установка на сервер

```bash
# Клонирование
git clone https://github.com/ZDarow/proxy-turn-vk-android.git
cd proxy-turn-vk-android

# Сборка
CGO_ENABLED=0 go build -ldflags='-s -w' -o wdtt-server .

# Установка
mkdir -p /etc/wdtt
cp wdtt-server /usr/local/bin/
cp config/wdtt-start.sh /usr/local/bin/ && chmod +x /usr/local/bin/wdtt-start.sh
cp config/config.json.example /etc/wdtt/config.json
# Отредактируйте /etc/wdtt/config.json
cp config/wdtt.service /etc/systemd/system/
systemctl daemon-reload && systemctl enable --now wdtt
```

### 2. Настройка конфига

```json
{
  "password": "мой_секретный_пароль",
  "admin": "123456789",
  "bot_token": "123:ABC-DEF1234ghIkl-zyx57W2v1u123ew11",
  "listen": "0.0.0.0:56000",
  "wg_port": 56001
}
```

### 3. Открыть порты в firewall

```bash
ufw allow 56000/udp  # DTLS
ufw allow 56001/udp  # WireGuard
ufw allow 9000/udp   # TUN (опционально)
```

### 4. Использование

Подключитесь к серверу через Android-клиент, используя мастер-пароль или сгенерированный через Telegram-бота.

---

## Структура проекта

```
wdtt/
├── server/                  # Исходный код сервера на Go
│   ├── server.go            # Основной файл (~1900 строк)
│   └── go.mod               # Go-модуль
├── config/                  # Конфигурационные файлы
│   ├── config.json.example  # Пример конфигурации
│   ├── wdtt.service         # Systemd-юнит
│   └── wdtt-start.sh        # Стартовый скрипт
├── scripts/                 # Скрипты
│   └── deploy.sh            # Автоматический деплой на сервер
├── ansible/                 # Ansible-роль для установки
│   ├── playbook.yml         # Основной плейбук
│   ├── inventory/           # Инвентарь
│   └── roles/wdtt/          # Роль WDTT
├── docs/                    # Документация
│   ├── README.md            # Этот файл
│   ├── INSTALL.md           # Полное руководство по установке
│   ├── USAGE.md             # Руководство по использованию
│   ├── ARCHITECTURE.md      # Архитектура и протокол
│   └── BYPASS_RESEARCH.md   # Исследование методов обхода DPI
└── Makefile                 # Сборка и деплой
```

---

## Документация

| Файл | Описание |
|------|----------|
| [INSTALL.md](INSTALL.md) | Полное руководство по установке с нуля |
| [USAGE.md](USAGE.md) | Управление сервером, Telegram-бот, мониторинг |
| [ARCHITECTURE.md](ARCHITECTURE.md) | Архитектура, протоколы, слои обфускации |
| [BYPASS_RESEARCH.md](BYPASS_RESEARCH.md) | Исследование методов обхода DPI |

---

## Флаги сервера

| Флаг | По умолчанию | Описание |
|------|-------------|----------|
| `-listen` | `0.0.0.0:56000` | Адрес DTLS-сервера |
| `-wg-port` | `56001` | Внутренний порт WireGuard |
| `-password` | `""` | Мастер-пароль владельца |
| `-admin` | `""` | Telegram Admin ID |
| `-bot-token` | `""` | Telegram Bot Token |
| `-dns` | `1.1.1.1` | DNS для клиентов |
| `-salamander-key` | `""` | Ключ Salamander-обфускации (hex) |
| `-jitter` | `false` | Рандомизация таймингов отправки |
| `-config-dir` | `/etc/wdtt` | Директория конфигурации |

---

## Обфускация трафика

WDTT использует многослойную обфускацию для защиты от DPI:

1. **WRAP (RTP + AEAD)** — каждый пакет оборачивается в RTP-подобный формат с ChaCha20-Poly1305 шифрованием и случайным padding (1-255 байт). Трафик неотличим от реального RTP-потока WebRTC.

2. **Salamander** (опционально) — дополнительный слой XOR на ключе HKDF-SHA256. Каждый пакет XOR-ится с криптостойким потоком ключей, сгенерированным из мастер-ключа и RTP-nonce. Включение: флаг `-salamander-key=<hex>`.

3. **Jitter** (опционально) — случайная задержка 1-10мс перед отправкой каждого пакета, нарушающая анализ таймингов DPI. Включение: флаг `-jitter`.

Подробнее: [ARCHITECTURE.md](ARCHITECTURE.md) и [BYPASS_RESEARCH.md](BYPASS_RESEARCH.md).

---

## Быстрые команды

```bash
# Статус сервиса
systemctl status wdtt

# Логи в реальном времени
journalctl -u wdtt -f

# Статистика сервера
cat /etc/wdtt/server.log

# Сборка
make build-static

# Деплой
make deploy HOST=root@213.21.242.99

# Ansible
ansible-playbook -i ansible/inventory/production ansible/playbook.yml \
  -e wdtt_main_password=мой_пароль
```

---

## Требования

- **Сервер:** Linux x86_64, Go >= 1.21, systemd, iptables/nftables, TUN-устройство
- **Клиент:** Android 8+ (репозиторий: [proxy-turn-vk-android](https://github.com/ZDarow/proxy-turn-vk-android))
- **Опционально:** Telegram Bot Token для управления

---

## Лицензия

MIT

<p align="center">
  <img src="https://img.shields.io/badge/Go-1.25+-00ADD8?logo=go" alt="Go">
  <img src="https://img.shields.io/badge/license-MIT-green" alt="License">
  <img src="https://img.shields.io/badge/platform-linux%20x86__64-lightgrey" alt="Platform">
  <img src="https://img.shields.io/badge/coverage-17%25-yellow" alt="Coverage">
  <img src="https://img.shields.io/badge/status-active-brightgreen" alt="Status">
  <img src="https://img.shields.io/github/issues/ZDarow/W_D_T_T" alt="Issues">
</p>

<h1 align="center">WDTT — WireGuard over DTLS/TURN Tunnel</h1>

<p align="center">
  <b>VPN-туннель с RTP-обфускацией трафика</b><br>
  Маскировка WireGuard под WebRTC-аудио/видео поток<br>
  Защита от Deep Packet Inspection (DPI)
</p>

<p align="center">
  <a href="#быстрый-старт">Быстрый старт</a> •
  <a href="#особенности">Особенности</a> •
  <a href="docs/INSTALL.md">Установка</a> •
  <a href="docs/USAGE.md">Использование</a> •
  <a href="docs/ARCHITECTURE.md">Архитектура</a> •
  <a href="CONTRIBUTING.md">Участие</a>
</p>

---

## Схема работы

```
┌─────────────────┐     RTP/DTLS      ┌──────────────────┐     UDP      ┌────────────┐
│  Android Client │ ←───────────────── │   WDTT Server    │ ←────────── │ WireGuard  │
│  (Kotlin)       │    порт 56000      │   (Go, server)   │   порт 56001 │ Userspace  │
└─────────────────┘                    └──────────────────┘              └────────────┘
        │                                       │                              │
        ├── WRAP: RTP + ChaCha20-Poly1305 AEAD   │                              │
        ├── Salamander: XOR + HKDF-SHA256 (опц.)  │                              │
        └── Jitter: 1-10ms рандомизация таймингов │                              │
```

---

## Особенности

- **RTP-обфускация** — каждый пакет выглядит как RTP-поток WebRTC (RTP-заголовок + AEAD + случайный padding до 255 байт)
- **Salamander** (опц.) — второй слой XOR на HKDF-SHA256 против DPI
- **Jitter** (опц.) — рандомизация таймингов 1-10ms против анализа трафика
- **Telegram Bot** — управление паролями, устройствами, статистикой
- **Multi-user** — временные пароли, привязка к устройствам, лимиты трафика
- **WireGuard userspace** — без ядерного модуля
- **Full-cone NAT** — iptables/nftables MASQUERADE
- **DDoS-защита** — rate-limit (5 попыток/10с), лимит соединений (64 клиента)

---

## Быстрый старт

### 1. Сборка

```bash
git clone https://github.com/ZDarow/W_D_T_T.git
cd W_D_T_T/server
CGO_ENABLED=0 go build -ldflags='-s -w' -o wdtt-server .
```

### 2. Запуск

```bash
sudo ./wdtt-server \
  -password "МойСекретныйПароль" \
  -admin "123456789" \
  -bot-token "123:ABC-DEF1234ghIkl-zyx57W2v1u123ew11" \
  -listen "0.0.0.0:56000"
```

### 3. Подключение

Используйте Android-клиент из [proxy-turn-vk-android](https://github.com/ZDarow/proxy-turn-vk-android).

> Подробнее: [INSTALL.md](docs/INSTALL.md), [USAGE.md](docs/USAGE.md)

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
| `-jitter` | `false` | Рандомизация таймингов |
| `-config-dir` | `/etc/wdtt` | Директория конфигурации |

---

## Структура проекта

```
wdtt/
├── server/                   # Исходный код сервера на Go
│   ├── server.go             # Основной файл (2138 строк, 84 функции)
│   ├── server_test.go        # 37 unit-тестов (17% покрытие)
│   └── go.mod
├── config/                   # Конфигурация и systemd
│   ├── config.json.example
│   ├── wdtt.service
│   └── wdtt-start.sh
├── scripts/                  # Утилиты
│   └── deploy.sh
├── ansible/                  # Ansible-роль для деплоя
│   ├── playbook.yml
│   └── roles/wdtt/
├── docs/                     # Документация
│   ├── ARCHITECTURE.md       # Архитектура и протокол
│   ├── INSTALL.md            # Полное руководство по установке
│   ├── USAGE.md              # Управление сервером
│   ├── AUDIT_REPORT.md       # Результаты аудита
│   ├── BYPASS_RESEARCH.md    # Исследование методов обхода DPI
│   └── DEEP_ANALYSIS.md      # Глубокий анализ кода (37 проблем)
├── .github/                  # GitHub-интеграция
│   ├── workflows/ci.yml
│   ├── ISSUE_TEMPLATE/
│   ├── PULL_REQUEST_TEMPLATE.md
│   ├── CODEOWNERS
│   └── FUNDING.yml
├── README.md                 # Этот файл
├── CHANGELOG.md              # История изменений
├── SECURITY.md               # Политика безопасности
├── CONTRIBUTING.md           # Руководство для контрибьюторов
├── LICENSE (MIT)
├── Makefile
└── .editorconfig
```

---

## Обфускация трафика

WDTT применяет многослойную обфускацию:

1. **WRAP (RTP + AEAD)** — базовая обфускация: ChaCha20-Poly1305 + RTP-заголовок + padding 1-255 байт
2. **Salamander** — поверх WRAP. XOR на HKDF-SHA256. Включение: `-salamander-key=<hex>`
3. **Jitter** — случайная задержка 1-10ms перед отправкой. Включение: `-jitter`

> Подробнее: [ARCHITECTURE.md](docs/ARCHITECTURE.md), [BYPASS_RESEARCH.md](docs/BYPASS_RESEARCH.md)

---

## Документация

| Файл | Описание |
|------|----------|
| [INSTALL.md](docs/INSTALL.md) | Полное руководство по установке с нуля |
| [USAGE.md](docs/USAGE.md) | Управление сервером, Telegram-бот, мониторинг |
| [ARCHITECTURE.md](docs/ARCHITECTURE.md) | Архитектура, протоколы, слои обфускации |
| [AUDIT_REPORT.md](docs/AUDIT_REPORT.md) | Результаты аудита и статус исправлений |
| [DEEP_ANALYSIS.md](docs/DEEP_ANALYSIS.md) | 37 проблем с планом рефакторинга |
| [BYPASS_RESEARCH.md](docs/BYPASS_RESEARCH.md) | Исследование методов обхода DPI |

---

## Быстрые команды

```bash
# Статус сервиса
systemctl status wdtt

# Логи
journalctl -u wdtt -f

# Сборка статического бинарника
make build-static

# Тесты
make test

# Деплой на VPS
make deploy HOST=root@213.21.242.99

# Ansible-деплой
make ansible-deploy PASSWORD=мой_пароль
```

---

## Требования

| Компонент | Требование |
|-----------|-----------|
| Серверная ОС | Linux x86_64 (Ubuntu/Debian/Mint) |
| Go | 1.25+ |
| systemd | для сервиса |
| iptables или nftables | для NAT |
| TUN-устройство | для WireGuard |
| Android | 8+ (клиент) |
| Telegram Bot Token | опционально |

---

## Лицензия

MIT License. См. [LICENSE](LICENSE).

---

<p align="center">
  <sub>Сделано с ❤️ для обхода DPI и защиты приватности</sub>
</p>

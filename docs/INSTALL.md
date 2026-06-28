# Руководство по установке WDTT

## Содержание

1. [Подготовка сервера](#1-подготовка-сервера)
2. [Установка Go](#2-установка-go)
3. [Сборка WDTT-сервера](#3-сборка-wdtt-сервера)
4. [Конфигурация](#4-конфигурация)
5. [Systemd-юнит](#5-systemd-юнит)
6. [Firewall](#6-firewall)
7. [Ansible (автоматизированная установка)](#7-ansible-автоматизированная-установка)
8. [Деплой-скрипт](#8-деплой-скрипт)
9. [Проверка установки](#9-проверка-установки)

---

## 1. Подготовка сервера

**Минимальные требования:**

- Linux x86_64 (Ubuntu 22.04+, Debian 12+)
- 512 MB RAM
- 5 GB дискового пространства
- Открытые UDP-порты: 56000, 56001 (и опционально 9000)
- TUN-устройство (включено по умолчанию в большинстве VPS)
- Root-доступ

**Рекомендуемые настройки (выполняются один раз):**

```bash
# Обновление системы
apt update && apt upgrade -y

# Установка базовых пакетов
apt install -y curl git ufw

# Включение IP forwarding (для NAT)
sysctl -w net.ipv4.ip_forward=1
echo "net.ipv4.ip_forward = 1" >> /etc/sysctl.conf

# Оптимизация сети: BBR
sysctl -w net.core.default_qdisc=fq
sysctl -w net.ipv4.tcp_congestion_control=bbr
echo "net.core.default_qdisc=fq" >> /etc/sysctl.conf
echo "net.ipv4.tcp_congestion_control=bbr" >> /etc/sysctl.conf

# Увеличение буферов сокетов
sysctl -w net.core.rmem_max=25165824
sysctl -w net.core.wmem_max=25165824
echo "net.core.rmem_max=25165824" >> /etc/sysctl.conf
echo "net.core.wmem_max=25165824" >> /etc/sysctl.conf

# Включение TUN (проверка)
ls -la /dev/net/tun || (mkdir -p /dev/net && mknod /dev/net/tun c 10 200 && chmod 666 /dev/net/tun)
```

---

## 2. Установка Go

```bash
# Определение последней версии
GO_VERSION=$(curl -s https://go.dev/dl/?mode=json | grep -oP '"version": "\K[^"]+' | head -1)
echo "Установка Go ${GO_VERSION:-1.23.0}..."

# Скачивание и установка
wget -q "https://go.dev/dl/${GO_VERSION:-1.23.0}.linux-amd64.tar.gz" -O /tmp/go.tar.gz
rm -rf /usr/local/go && tar -C /usr/local -xzf /tmp/go.tar.gz
echo 'export PATH=$PATH:/usr/local/go/bin' >> /etc/profile
source /etc/profile

# Проверка
go version
```

---

## 3. Сборка WDTT-сервера

```bash
# Клонирование репозитория
git clone https://github.com/ZDarow/proxy-turn-vk-android.git
cd proxy-turn-vk-android

# Статическая сборка (рекомендуется)
CGO_ENABLED=0 go build -ldflags='-s -w' -o wdtt-server .

# Проверка
./wdtt-server -h
```

---

## 4. Конфигурация

```bash
# Создание директории
mkdir -p /etc/wdtt

# Создание конфига
cat > /etc/wdtt/config.json << 'EOF'
{
  "password": "мой_супер_секретный_пароль",
  "admin": "",
  "bot_token": "",
  "dns": "1.1.1.1",
  "listen": "0.0.0.0:56000",
  "wg_port": 56001,
  "salamander_key": "",
  "jitter": false
}
EOF

# Защита конфига
chmod 0600 /etc/wdtt/config.json
```

**Поля конфига:**

| Поле | Тип | Описание |
|------|-----|----------|
| `password` | string | **Обязательно.** Мастер-пароль владельца |
| `admin` | string | Telegram Admin ID (число, для управления через бота) |
| `bot_token` | string | Telegram Bot Token (получить у @BotFather) |
| `dns` | string | DNS-серверы для клиентов (через запятую) |
| `listen` | string | Адрес DTLS-сервера |
| `wg_port` | int | Внутренний порт WireGuard |
| `salamander_key` | string | Ключ Salamander-обфускации в hex (минимум 32 hex-символа = 16 байт) |
| `jitter` | bool | Включить рандомизацию таймингов отправки |

---

## 5. Systemd-юнит

```bash
# Копирование юнита
cat > /etc/systemd/system/wdtt.service << 'EOF'
[Unit]
Description=WDTT — WireGuard over TURN Tunnel Server
After=network.target
Wants=network.target

[Service]
Type=simple
User=root
ExecStart=/usr/local/bin/wdtt-start.sh
Restart=always
RestartSec=5
LimitNOFILE=65536

[Install]
WantedBy=multi-user.target
EOF

# Стартовый скрипт
cat > /usr/local/bin/wdtt-start.sh << 'SCRIPT'
#!/bin/bash
CONFIG=/etc/wdtt/config.json

if [ -f "$CONFIG" ]; then
    PASSWORD=$(     grep -oP '"password"\s*:\s*"\K[^"]+' $CONFIG)
    ADMIN=$(        grep -oP '"admin"\s*:\s*"\K[^"]*' $CONFIG)
    BOT=$(          grep -oP '"bot_token"\s*:\s*"\K[^"]*' $CONFIG)
    DNS=$(          grep -oP '"dns"\s*:\s*"\K[^"]+' $CONFIG)
    LISTEN=$(       grep -oP '"listen"\s*:\s*"\K[^"]+' $CONFIG)
    WG_PORT=$(      grep -oP '"wg_port"\s*:\s*\K[0-9]+' $CONFIG)
    SALAMANDER_KEY=$(grep -oP '"salamander_key"\s*:\s*"\K[^"]*' $CONFIG)
    JITTER=$(       grep -oP '"jitter"\s*:\s*\K(?:true|false)' $CONFIG)

    ARGS="-listen $LISTEN -wg-port $WG_PORT -dns $DNS"
    [ -n "$PASSWORD" ]       && ARGS="$ARGS -password $PASSWORD"
    [ -n "$ADMIN" ]          && ARGS="$ARGS -admin $ADMIN"
    [ -n "$BOT" ]            && ARGS="$ARGS -bot-token $BOT"
    [ -n "$SALAMANDER_KEY" ] && ARGS="$ARGS -salamander-key $SALAMANDER_KEY"
    [ "$JITTER" = "true" ]   && ARGS="$ARGS -jitter"

    exec /usr/local/bin/wdtt-server $ARGS
else
    exec /usr/local/bin/wdtt-server -password changeme
fi
SCRIPT

chmod +x /usr/local/bin/wdtt-start.sh

# Активация
systemctl daemon-reload
systemctl enable --now wdtt
systemctl status wdtt --no-pager
```

---

## 6. Firewall

```bash
# UFW
ufw allow 56000/udp comment 'WDTT DTLS'
ufw allow 56001/udp comment 'WDTT WireGuard'
ufw allow 9000/udp  comment 'WDTT TUN (опционально)'

# iptables (если без UFW)
iptables -A INPUT -p udp --dport 56000 -j ACCEPT
iptables -A INPUT -p udp --dport 56001 -j ACCEPT
```

---

## 7. Ansible (автоматизированная установка)

Если у вас есть Ansible, можно установить WDTT полностью автоматически:

```bash
# Настройка инвентаря
cat > /etc/ansible/hosts << 'EOF'
[wdtt]
ваш-сервер ansible_host=213.21.242.99 ansible_user=root
EOF

# Запуск плейбука
ansible-playbook ansible/playbook.yml \
  -i ansible/inventory/production \
  -e wdtt_main_password=мой_пароль \
  -e wdtt_admin_id=123456789 \
  -e wdtt_bot_token=123:ABC-DEF1234ghIkl-zyx57W2v1u123ew11 \
  -e wdtt_jitter=true
```

**Переменные Ansible:**

| Переменная | По умолч. | Описание |
|-----------|-----------|----------|
| `wdtt_main_password` | (обяз.) | Мастер-пароль |
| `wdtt_admin_id` | `""` | Telegram Admin ID |
| `wdtt_bot_token` | `""` | Telegram Bot Token |
| `wdtt_dns` | `1.1.1.1` | DNS для клиентов |
| `wdtt_listen` | `0.0.0.0:56000` | DTLS-адрес |
| `wdtt_dtls_port` | `56000` | DTLS-порт для UFW |
| `wdtt_wg_port` | `56001` | WG-порт |
| `wdtt_tun_port` | `9000` | TUN-порт для UFW |
| `wdtt_salamander_key` | `""` | Ключ Salamander |
| `wdtt_jitter` | `false` | Jitter |
| `wdtt_memory_max` | `512M` | Лимит памяти в systemd |
| `wdtt_cpu_quota` | `80%` | Квота CPU |

---

## 8. Деплой-скрипт

Для быстрого деплоя после изменений:

```bash
./scripts/deploy.sh root@213.21.242.99
```

Или через Makefile:

```bash
make deploy HOST=root@213.21.242.99
```

Скрипт автоматически:
1. Собирает бинарник (статическая сборка)
2. Копирует его на сервер
3. Копирует стартовый скрипт и systemd-юнит
4. Перезапускает сервис

---

## 9. Проверка установки

```bash
# Проверка, что сервис запущен
systemctl status wdtt

# Проверка логов
journalctl -u wdtt --no-pager | tail -20

# Проверка портов
ss -ulpn | grep -E '56000|56001'

# Проверка TUN-интерфейса
ip addr show wdtt0

# Проверка WireGuard
wg show

# Проверка NAT
iptables -t nat -L POSTROUTING -v -n | grep WDTT

# Быстрый тест (если сервер запущен с -salamander-key и -jitter)
# Логи должны содержать строки:
#   [SALAMANDER] Включён
#   [JITTER] Включён
journalctl -u wdtt --no-pager | grep -E 'SALAMANDER|JITTER'
```

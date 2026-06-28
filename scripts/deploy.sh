#!/bin/bash
# ==================== WDTT — Deploy скрипт ====================
# Использование: ./deploy.sh <host>
# Пример: ./deploy.sh root@213.21.242.99
#
# Делает:
#   1. Компилирует wdtt-server статически (CGO_ENABLED=0)
#   2. Копирует бинарник, скрипты и конфиг на сервер
#   3. Устанавливает systemd-юнит
#   4. Запускает/перезапускает сервис
# ==============================================================

set -euo pipefail

HOST="${1:-}"
if [ -z "$HOST" ]; then
    echo "❌ Использование: $0 <user@host>"
    echo "   Пример: $0 root@213.21.242.99"
    exit 1
fi

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PROJECT_DIR="$(dirname "$SCRIPT_DIR")"
SERVER_DIR="$PROJECT_DIR/server"
CONFIG_DIR="$PROJECT_DIR/config"
BUILD_DIR="/tmp/wdtt-build"

echo "═══════════════════════════════════════════"
echo " WDTT Deploy → $HOST"
echo "═══════════════════════════════════════════"

# 1. Сборка
echo "🔧 Сборка wdtt-server..."
cd "$SERVER_DIR"
go mod tidy
CGO_ENABLED=0 go build -ldflags='-s -w' -o "$BUILD_DIR/wdtt-server" .
echo "   ✅ Бинарник: $(ls -lh "$BUILD_DIR/wdtt-server" | awk '{print $5}')"

# 2. Копирование на сервер
echo "📤 Копирование на сервер..."
scp "$BUILD_DIR/wdtt-server" "$HOST:/usr/local/bin/"
scp "$CONFIG_DIR/wdtt-start.sh" "$HOST:/usr/local/bin/"
scp "$CONFIG_DIR/wdtt.service" "$HOST:/etc/systemd/system/"

# 3. Установка прав
ssh "$HOST" bash -c "'
    chmod +x /usr/local/bin/wdtt-server
    chmod +x /usr/local/bin/wdtt-start.sh
    systemctl daemon-reload
'"

# 4. Создание конфига (если нет)
ssh "$HOST" bash -c "'
    if [ ! -f /etc/wdtt/config.json ]; then
        mkdir -p /etc/wdtt
        cp /usr/local/bin/wdtt-start.sh /etc/wdtt/config.json.example 2>/dev/null || true
        echo \"⚠️ Конфиг /etc/wdtt/config.json не найден!\"
        echo \"   Создайте его вручную или скопируйте пример:\"
        echo \"   cp /etc/wdtt/config.json.example /etc/wdtt/config.json\"
    fi
'"

# 5. Запуск
echo "🚀 Запуск сервиса..."
ssh "$HOST" "systemctl enable wdtt && systemctl restart wdtt && systemctl status wdtt --no-pager | head -15"

echo "═══════════════════════════════════════════"
echo "✅ Deploy завершён!"
echo "   Сервер: $HOST"
echo "   Статус: systemctl status wdtt"
echo "   Логи: journalctl -u wdtt -f"
echo "═══════════════════════════════════════════"

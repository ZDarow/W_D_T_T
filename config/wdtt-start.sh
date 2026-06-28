#!/bin/bash
# ==================== WDTT — Стартовый скрипт ====================
# Читает конфиг /etc/wdtt/config.json и запускает сервер с нужными флагами.
# Используется как ExecStart в systemd-юните wdtt.service.
# ================================================================

CONFIG=/etc/wdtt/config.json

if [ -f "$CONFIG" ]; then
    # Чтение всех полей из JSON одной командой через grep/sed
    PASSWORD=$(     grep -oP '"password"\s*:\s*"\K[^"]+' $CONFIG)
    ADMIN=$(        grep -oP '"admin"\s*:\s*"\K[^"]*' $CONFIG)
    BOT=$(          grep -oP '"bot_token"\s*:\s*"\K[^"]*' $CONFIG)
    DNS=$(          grep -oP '"dns"\s*:\s*"\K[^"]+' $CONFIG)
    LISTEN=$(       grep -oP '"listen"\s*:\s*"\K[^"]+' $CONFIG)
    WG_PORT=$(      grep -oP '"wg_port"\s*:\s*\K[0-9]+' $CONFIG)
    SALAMANDER_KEY=$(grep -oP '"salamander_key"\s*:\s*"\K[^"]*' $CONFIG)
    JITTER=$(       grep -oP '"jitter"\s*:\s*\K(?:true|false)' $CONFIG)

    # Сборка аргументов
    ARGS="-listen $LISTEN -wg-port $WG_PORT -dns $DNS"
    [ -n "$PASSWORD" ]       && ARGS="$ARGS -password $PASSWORD"
    [ -n "$ADMIN" ]          && ARGS="$ARGS -admin $ADMIN"
    [ -n "$BOT" ]            && ARGS="$ARGS -bot-token $BOT"
    [ -n "$SALAMANDER_KEY" ] && ARGS="$ARGS -salamander-key $SALAMANDER_KEY"
    [ "$JITTER" = "true" ]   && ARGS="$ARGS -jitter"

    exec /usr/local/bin/wdtt-server $ARGS
else
    # Fallback: минимальный запуск без конфига
    exec /usr/local/bin/wdtt-server -password changeme
fi

# ==================== Makefile для WDTT ====================
# Основные команды:
#   make build          — сборка сервера
#   make build-static   — статическая сборка (для Linux)
#   make deploy HOST=root@ip — деплой на сервер
#   make clean          — очистка артефактов
# ===========================================================

SERVER_DIR = server
BUILD_DIR  = /tmp/wdtt-build
BINARY     = wdtt-server
HOST      ?= root@213.21.242.99

.PHONY: all build build-static deploy clean

all: build-static

build:
	@echo "🔧 Сборка $(BINARY)..."
	cd $(SERVER_DIR) && go build -ldflags='-s -w' -o $(BUILD_DIR)/$(BINARY) .
	@echo "✅ Готово: $(BUILD_DIR)/$(BINARY)"

build-static:
	@echo "🔧 Статическая сборка $(BINARY)..."
	cd $(SERVER_DIR) && CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags='-s -w' -o $(BUILD_DIR)/$(BINARY) .
	@echo "✅ Готово: $(BUILD_DIR)/$(BINARY) (статический ELF)"

deploy: build-static
	@echo "📤 Деплой на $(HOST)..."
	scp $(BUILD_DIR)/$(BINARY) $(HOST):/usr/local/bin/
	scp config/wdtt-start.sh $(HOST):/usr/local/bin/
	scp config/wdtt.service $(HOST):/etc/systemd/system/
	ssh $(HOST) "chmod +x /usr/local/bin/$(BINARY) /usr/local/bin/wdtt-start.sh && systemctl daemon-reload && systemctl restart wdtt"
	ssh $(HOST) "systemctl status wdtt --no-pager | head -10"
	@echo "✅ Деплой завершён!"

clean:
	@echo "🧹 Очистка..."
	rm -rf $(BUILD_DIR)
	@echo "✅ Готово"

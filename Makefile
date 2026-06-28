# ==================== Makefile для WDTT ====================
# Основные команды:
#   make build            — сборка сервера
#   make build-static     — статическая сборка (для Linux)
#   make test             — запуск всех тестов
#   make lint             — go vet + статические анализаторы
#   make deploy HOST=...  — деплой на сервер
#   make ansible-deploy   — деплой через Ansible
#   make clean            — очистка артефактов
# ============================================================

SERVER_DIR = server
BUILD_DIR  = /tmp/wdtt-build
BINARY     = wdtt-server
HOST      ?= root@213.21.242.99
PASSWORD  ?=

.PHONY: all build build-static test test-race lint deploy ansible-deploy clean clean-all

all: build-static

build:
	@echo "==> Сборка $(BINARY)..."
	cd $(SERVER_DIR) && go build -ldflags='-s -w' -o $(BUILD_DIR)/$(BINARY) .

build-static:
	@echo "==> Статическая сборка $(BINARY)..."
	@mkdir -p $(BUILD_DIR)
	cd $(SERVER_DIR) && CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags='-s -w' -o $(BUILD_DIR)/$(BINARY) .
	@echo "    Готово: $(BUILD_DIR)/$(BINARY) (статический ELF)"
	ls -lh $(BUILD_DIR)/$(BINARY)

test:
	@echo "==> Запуск тестов..."
	cd $(SERVER_DIR) && go test -v -timeout 60s -count=1 ./...

test-race:
	@echo "==> Запуск тестов с race detector..."
	cd $(SERVER_DIR) && go test -race -timeout 60s -count=1 ./...

test-cover:
	@echo "==> Запуск тестов с покрытием..."
	cd $(SERVER_DIR) && go test -coverprofile=coverage.out -timeout 60s -count=1 ./... && \
		go tool cover -func=coverage.out | grep -E 'total|obfs|auth|wrap'

lint:
	@echo "==> Линтинг..."
	cd $(SERVER_DIR) && go vet ./...
	@echo "    Проверка log.Fatalf вне main..."
	@! grep -n "log.Fatalf" $(SERVER_DIR)/server.go | grep -v "func main()" || \
		(echo "    [ОШИБКА] log.Fatalf вне main()!"; false)
	@echo "    Проверка goto..."
	@! grep -n "goto " $(SERVER_DIR)/server.go || \
		(echo "    [ОШИБКА] goto найден!"; false)
	@echo "    Линтинг YAML..."
	yamllint .github/workflows/ci.yml 2>/dev/null || true

deploy: build-static
	@echo "==> Деплой на $(HOST)..."
	scp $(BUILD_DIR)/$(BINARY) $(HOST):/usr/local/bin/
	scp config/wdtt-start.sh $(HOST):/usr/local/bin/
	scp config/wdtt.service $(HOST):/etc/systemd/system/
	ssh $(HOST) "chmod +x /usr/local/bin/$(BINARY) /usr/local/bin/wdtt-start.sh && systemctl daemon-reload && systemctl restart wdtt"
	ssh $(HOST) "systemctl status wdtt --no-pager | head -10"
	@echo "==> Деплой завершён!"

ansible-deploy:
	@echo "==> Деплой через Ansible..."
	cd ansible && ansible-playbook -i inventory/production playbook.yml \
		$(if $(PASSWORD),-e wdtt_main_password=$(PASSWORD),)

clean:
	@echo "==> Очистка артефактов сборки..."
	rm -rf $(BUILD_DIR)
	cd $(SERVER_DIR) && rm -f coverage.out race*.out *.test

clean-all: clean
	@echo "==> Полная очистка..."
	rm -rf $(SERVER_DIR)/vendor/
	go clean -cache

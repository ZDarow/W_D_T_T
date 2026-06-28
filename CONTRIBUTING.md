# Как внести вклад в WDTT

Спасибо, что хотите помочь проекту! 🎉

## Сообщение об ошибке

Создайте [Issue с шаблоном](https://github.com/ZDarow/W_D_T_T/issues/new/choose).
Убедитесь, что:
- Проверили существующие Issues
- Приложили логи `journalctl -u wdtt -n 50`
- Указали флаги запуска и версию сервера

## Предложение функциональности

Создайте [Feature Request](https://github.com/ZDarow/W_D_T_T/issues/new?template=feature_request.md).

## Pull Request

### Подготовка

1. Форкните репозиторий
2. Создайте ветку от `main`:
   ```bash
   git checkout -b feature/your-feature
   # или
   git checkout -b fix/your-fix
   ```
3. Внесите изменения

### Требования к коду

- Комментарии на русском (объясняйте «почему», а не «что»)
- Имена переменных/функций/типов — на английском (CamelCase)
- Код Go — соответствует `gofmt` и `go vet`
- **Запрещено:** `log.Fatalf` вне `main()`, `goto`, игнорирование ошибок
- **Обратная совместимость** — не ломать протокол GETCONF/READY

### Проверка перед коммитом

```bash
go vet ./...
go build ./...
go test -race -timeout 30s ./...
grep -n "log.Fatalf" server/server.go | grep -v "func main()"  # 0 результатов
grep -n "goto " server/server.go                                # 0 результатов
```

### Коммит

- Сообщение на русском, в повелительном наклонении
- Формат: `тип: краткое описание`
- Типы: `feat`, `fix`, `refactor`, `docs`, `test`, `chore`, `style`, `perf`, `ci`

### PR

1. Заполните [шаблон PR](.github/PULL_REQUEST_TEMPLATE.md)
2. Убедитесь, что CI проходит (lint, vet, build, mod-check)
3. Дождитесь ревью

## Стиль кода

Следуйте [AGENTS.md](AGENTS.md) — это основной документ с гайдлайнами.

### Что НЕ приветствуется

- Косметические изменения в коде без функциональной необходимости
- Добавление новых зависимостей без острой нужды
- Изменение формата протокола без обсуждения
- `gofmt`-только коммиты

# Changelog

## [2.0.0] — 2026-06-28

### Добавлено
- Salamander-обфускация (XOR + HKDF-SHA256, флаг `-salamander-key`)
- Jitter — рандомизация таймингов 1-10ms (флаг `-jitter`)
- Rate-limiting на аутентификацию (5 попыток за 10 секунд)
- Лимит параллельных соединений (макс. 64 клиента)
- Graceful shutdown (ожидание активных соединений при SIGTERM)
- Telegram Bot: управление паролями, устройствами, статистика
- Telegram Bot: inline-кнопки с просмотром, деактивацией, удалением паролей
- Multi-user: генерация временных паролей, привязка к устройствам
- Full-cone NAT: поддержка iptables и nftables
- 37 unit-тестов (криптоядро, key store, rate-limit, обфускация)
- CI/CD: GitHub Actions (lint, vet, build, mod-check)
- Ansible-роль для деплоя
- Документация: README, INSTALL, USAGE, ARCHITECTURE, AUDIT, BYPASS_RESEARCH

### Исправлено
- TD-1: обработка ошибок `ResolveUDPAddr` и `GenerateSelfSigned`
- TD-2: `setupFullConeNAT` возвращает реальные ошибки
- TD-3: graceful shutdown вместо `os.Exit(0)`
- TD-4/5: утечка ключей через `string(key)` в aeadCache
- TD-6/7: игнорирование ошибок `os.WriteFile` и `exec.Command` в NAT
- TD-9: `botLoop` разделён с 410 строк на 12 функций
- TD-11: удалены неиспользуемые зависимости (`turn/v5`, `connutil`, `uuid`)
- SEC-1: rate-limiting на аутентификацию
- SEC-2: лимит параллельных соединений

### Техдолг (открыто)
- 0% тестового покрытия для `handleConn` и `authClient`
- Монолит — весь код в одном файле (2138 строк, 84 функции)
- Глобальные переменные (`db`, `authAttempts`, `connSemaphore`)
- Нет очистки iptables при завершении
- `botLoop` без graceful shutdown (висит до 60s)

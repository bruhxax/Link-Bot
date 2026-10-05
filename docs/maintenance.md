# Обслуживание Link-Bot

[← README](../README.md) · [Настройки](configuration.md)

Все команды выполняются на сервере из `/opt/Link-Bot`.

## Обновление

```bash
cd /opt/Link-Bot
bash ./update.sh
```

Скрипт скачивает изменения, добавляет недостающие параметры в `.env` и пересоздаёт контейнеры. Заданные значения `.env`, база и настройки админки сохраняются. Для подробного вывода: `LINK_BOT_UPDATE_VERBOSE=1 bash ./update.sh`.

При смене домена измените **оба** значения: `PUBLIC_HOST` и `PUBLIC_BASE_URL`. Скрипт обновляет встроенный Caddy; для общего `link-bot-caddy` нужен доступный Caddyfile, примонтированный с хоста.

## Резервная копия

```bash
docker compose exec -T db sh -c 'pg_dump -U "$POSTGRES_USER" "$POSTGRES_DB"' > link-bot-backup.sql
```

Сохраните также `.env` и пользовательские медиа: загруженные файлы находятся в Docker-томе uploads, баннеры Telegram — в `assets/telegram/`.

**Восстановление SQL-копии:**

```bash
cat link-bot-backup.sql | docker compose exec -T db sh -c 'psql -U "$POSTGRES_USER" "$POSTGRES_DB"'
```

## Откат

Посмотреть выбранную версию:

```bash
bash ./rollback.sh --dry-run
```

Запустить предыдущий релиз:

```bash
bash ./rollback.sh
```

Для конкретного более раннего релиза: `bash ./rollback.sh --to vX.Y.Z` — замените тег существующим. Скрипт делает бэкап БД и заменяет только контейнер бота. Исходники остаются на `main`; возврат к текущей версии — через `bash ./update.sh`.

База автоматически назад не откатывается. Если старый код несовместим с данными, верните работавшую версию; путь к бэкапу указан в выводе скрипта.

## Перенос из Bedolaga

Переносятся пользователи, баланс, реферальные связи и действующие подписки. Link-Bot должен использовать **ту же панель Remnawave**, а база Bedolaga — быть доступна с его сервера.

1. Остановите старый бот, соберите Link-Bot и сделайте бэкап:

```bash
docker compose up -d --build bot
docker compose exec -T db sh -c 'pg_dump -U "$POSTGRES_USER" "$POSTGRES_DB"' > link-bot-pre-bedolaga.sql
```

2. Укажите подключение к базе Bedolaga и запустите проверку без изменений:

```bash
export BEDOLAGA_DATABASE_URL='postgres://USER:PASSWORD@BEDOLAGA_HOST:5432/DBNAME?sslmode=require'
docker compose --profile tools run --rm migrate-bedolaga
```

3. Проверьте числа в отчёте и примените перенос:

```bash
docker compose --profile tools run --rm migrate-bedolaga --apply
unset BEDOLAGA_DATABASE_URL
```

Повторный импорт не зачисляет баланс дважды. Не переносятся история платежей, платёжные реквизиты, настройки старого бота и отключённые, ограниченные или ожидающие подписки. При ошибке импорт отменяется целиком.

## Контейнеры

| Действие | Команда |
| :--- | :--- |
| Логи HTTPS-прокси | `docker compose --profile standalone logs -f --tail=200 caddy` |
| Остановить | `docker compose --profile standalone stop` |
| Запустить | `docker compose --profile standalone start` |
| Удалить контейнеры, сохранив тома | `docker compose --profile standalone down` |

> `docker compose --profile standalone down -v` удаляет тома с базой и загруженными медиа. Для восстановления нужна резервная копия.

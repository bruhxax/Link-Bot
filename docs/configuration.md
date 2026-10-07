# Настройки Link-Bot

[← README](../README.md) · [Платежи](payment-providers.md) · [Обслуживание](maintenance.md)

Тарифы, платёжные ключи, промокоды, награды за отзывы и оформление задаются в **Админке Mini App**. Параметры сервера — в `.env`; после их изменения выполните `bash ./update.sh` из `/opt/Link-Bot`.

## Telegram и вход в кабинет

1. Отправьте боту `/start` и откройте Mini App с аккаунта из `ADMIN_TELEGRAM_ID`.
2. В [@BotFather](https://t.me/BotFather?startapp) выберите бота и задайте адрес **Main App**: `https://bot.example.com/mini-app/`.
3. В **Login Widget → Allowed URLs** добавьте `https://bot.example.com` и `https://bot.example.com/mini-app/`.
4. В **Login Widget → Advanced** оставьте алгоритм `RS256`.

Замените домен своим. Для входа через Google задайте `GOOGLE_CLIENT_ID` и разрешите HTTPS-адрес кабинета в настройках OAuth. Для входа по почте настройте SMTP ниже.

## Домены и HTTPS

| Параметр | Пример |
| :--- | :--- |
| `PUBLIC_HOST` | `bot.example.com` — без протокола |
| `PUBLIC_BASE_URL` | `https://bot.example.com` — полный HTTPS-адрес |
| `CABINET_SUBDOMAIN` | `my` — кабинет на `my.bot.example.com`; пусто — основной домен |

Для поддомена создайте `A`-запись на тот же IP и разрешите его в Telegram Login Widget и Google OAuth. Лендинг остаётся на основном домене; кнопка **Кабинет** открывает авторизацию.

Встроенный Caddy запускается профилем `standalone`. С внешним HTTPS-прокси запускайте `docker compose up -d --build bot`; прокси должен иметь доступ к `bot:8080` в Docker-сети.

## Подключение Remnawave

Для обычной панели достаточно `REMNAWAVE_URL`, `REMNAWAVE_TOKEN` и `REMNAWAVE_MODE=remote`.

Если панель защищена дополнительной авторизацией:

| Защита | Параметр `.env` |
| :--- | :--- |
| Caddy Security / MFA | `CADDY_AUTH_API_TOKEN=ключ_API` |
| eGames TinyAuth | `CADDY_AUTH_API_TOKEN=Basic base64(логин:пароль)` |
| eGames Nginx cookie | `EGAMES_COOKIE=ИМЯ=ЗНАЧЕНИЕ` |
| Дополнительные заголовки | `REMNAWAVE_HEADERS=Header-One:value;Header-Two:value` |

Cookie возьмите из `map $http_cookie $auth_cookie` в `/opt/remnawave/nginx.conf`: только пару `ИМЯ=ЗНАЧЕНИЕ`, без URL, `Cookie:` или `Path=/`.

При подключении к той же Docker-сети можно использовать `REMNAWAVE_URL=http://remnawave:3000` и `REMNAWAVE_MODE=local` без cookie.

### Уведомления о входе в панель

В `.env` Link-Bot задайте `REMNAWAVE_WEBHOOK_SECRET` длиной от 32 символов. В `.env` Remnawave:

```dotenv
WEBHOOK_ENABLED=true
WEBHOOK_URL=https://bot.example.com/api/remnawave/webhook
WEBHOOK_SECRET_HEADER=тот_же_секрет
```

Включите события `service.login_attempt_failed` и `service.login_attempt_success` в настройках уведомлений панели и перезапустите оба приложения.

## Почта

Укажите данные SMTP-провайдера в `.env`:

```dotenv
SMTP_HOST=smtp.example.com
SMTP_PORT=587
SMTP_USER=mail@example.com
SMTP_PASSWORD=пароль_приложения
SMTP_FROM=mail@example.com
```

Порт `587` использует STARTTLS, `465` — TLS. Если хостинг блокирует SMTP, задайте `SMTP_PROXY_URL`: HTTP(S)-прокси должен разрешать CONNECT к SMTP-хосту и порту.

## ИИ в поддержке

В **Админка → ИИ**:

1. Укажите URL OpenAI-совместимого API и ключ своего провайдера.
2. Нажмите **Проверить и загрузить модели**, выберите модель.
3. Настройте промпт, сохраните и включите переключатель сверху.

К URL без пути добавляется `/v1`. API должен поддерживать `GET /models` и `POST /chat/completions`. При смене URL введите ключ заново.

Помощник учитывает подписки и историю пользователя, FAQ и обезличенные решения похожих тикетов. Отвечает только в поддержке; по просьбе об операторе или при затруднении зовёт администратора. Баланс и подписки сам не изменяет. Имя и стиль ответов задаются промптом; ключ хранится зашифрованным.

## Аналитика

### Google Analytics 4

1. Создайте ресурс GA4 и веб-поток. Укажите в `.env` `GA4_MEASUREMENT_ID=G-...` и числовой `GA4_PROPERTY_ID`.
2. В Google Cloud включите Google Analytics Data API, создайте сервисный аккаунт и его JSON-ключ. Добавьте email аккаунта к ресурсу GA4 с ролью **Читатель**.
3. Сохраните JSON на сервере в `/opt/Link-Bot/secrets/ga4-service-account.json`:

```bash
chown 1000:1000 /opt/Link-Bot/secrets/ga4-service-account.json
chmod 600 /opt/Link-Bot/secrets/ga4-service-account.json
```

В `.env` оставьте `GA4_SERVICE_ACCOUNT_FILE=/app/secrets/ga4-service-account.json`. `GOOGLE_CLIENT_ID` для входа не заменяет данные GA4.

### Яндекс Метрика

1. Создайте счётчик для своего домена и задайте `YANDEX_METRIKA_COUNTER_ID` в `.env`.
2. Получите OAuth-токен с разрешением `metrika:read` от пользователя с доступом к счётчику и задайте `YANDEX_METRIKA_OAUTH_TOKEN`.

ID включает сбор посещений; OAuth-токен нужен для отчётов в **Админка → Аналитика**. GA4 и Метрика подключаются независимо. Выручка и история оплат находятся в **Финансах**.

## Оформление

| Что изменить | Где |
| :--- | :--- |
| Логотип | **Админка → Контент → Главное меню**: PNG, JPG или WebP до 2 МБ |
| Баннер Mini App | **Конструктор UI → Добавить элемент → Баннер**: PNG, GIF или MP4 до 50 МБ |
| Свои фоны | **Админка → Оформление → Свой фон**: файл или прямая HTTP/HTTPS-ссылка, до 50 МБ |
| Баннеры Telegram | [Папки и пути](../assets/telegram/README.md) |
| Язык и шрифт | **Админка → Язык и шрифт** |

Загрузите медиа через интерфейс и сохраните изменения. Логотип, баннеры и свои фоны сохраняются в постоянном Docker-томе. Фон по ссылке копируется на сервер.

Можно сохранить до **20 фонов**: PNG, JPG, GIF, WebP, AVIF, BMP, SVG; видео MP4, WebM, MOV, OGV. Видео должно поддерживаться браузером; для совместимости используйте MP4 с H.264 или WebM.

Для GIF действует ограничение на сложность анимации: до 4 мегапикселей, 1000 кадров и 150 миллионов пикселей суммарно по кадрам. Для длинных анимаций лучше использовать видео.

Для каждого фона отдельно сохраняются **масштаб 50–300%**, положение, режим «Заполнить экран / Показать целиком», **затемнение 0–90%** и **скорость GIF/видео 10–200%**. Двигайте фон в предпросмотре пальцем или мышью; точное положение доступно под ползунками. Нажмите галочку сохранения, чтобы применить оформление всем пользователям. Добавление фонов доступно администраторам с правом **«Оформление»**.

<div align="center">

<p>
  <b>Русский</b> · <a href="README_EN.md">English</a>
</p>

<img src="docs/Link-Bot.png" width="100%" alt="Интерфейс Link-Bot">

# Link-Bot

**Telegram-бот и Mini App для продажи и управления VPN-подписками Remnawave**

<p>
  <a href="https://t.me/BruhvpnBot">
    <img src="https://img.shields.io/badge/Telegram-Try%20Bot-2AABEE?style=for-the-badge&logo=telegram&logoColor=white" alt="Попробовать Link-Bot">
  </a>
  <a href="https://t.me/REMNALinkBot">
    <img src="https://img.shields.io/badge/Telegram-Community-229ED9?style=for-the-badge&logo=telegram&logoColor=white" alt="Сообщество Link-Bot">
  </a>
</p>

<p>
  <img src="https://img.shields.io/badge/Go-1.25-00ADD8?style=flat-square&logo=go&logoColor=white" alt="Go 1.25">
  <img src="https://img.shields.io/badge/PostgreSQL-17-4169E1?style=flat-square&logo=postgresql&logoColor=white" alt="PostgreSQL 17">
  <img src="https://img.shields.io/badge/Docker-Compose-2496ED?style=flat-square&logo=docker&logoColor=white" alt="Docker Compose">
</p>

[Попробовать бота](https://t.me/BruhvpnBot) · [Сообщество в Telegram](https://t.me/REMNALinkBot) · [Remnawave](https://github.com/remnawave/panel)

</div>

---

## 🧩 Что такое Link-Bot?

**Link-Bot** — Telegram-бот и Mini App для продажи и управления VPN-подписками Remnawave.

Он объединяет личный кабинет пользователя, оплату, управление подписками, поддержку и администрирование в одном интерфейсе.

> 🤖 **Демонстрация:** [открыть Link-Bot в Telegram](https://t.me/BruhvpnBot)  
> 💬 **Сообщество:** [новости, вопросы и обсуждения](https://t.me/REMNALinkBot)

---

## ✨ Возможности

| 📦 Подписки и тарифы | 💳 Платежи |
|---|---|
| • Личный кабинет в Telegram Mini App и браузере<br>• Создание и продление подписок Remnawave<br>• Покупка дополнительных устройств отдельно или вместе с тарифом<br>• Тарифы, триал и выбор внутренних/внешних сквадов<br>• Настраиваемый шаблон имени пользователя в панели<br>• Привязка и перенос подписок между Telegram-аккаунтами | • YooKassa<br>• Crypto Pay<br>• Telegram Stars<br>• Lava<br>• WATA<br>• Platega<br>• FreeKassa<br>• Heleket<br>• Pally<br>• RollyPay<br>• cisPay |

| 📣 Продвижение и уведомления | 🛠️ Администрирование |
|---|---|
| • Промокоды<br>• Реферальная система<br>• Рассылки с кнопками навигации по Mini App<br>• Уведомления об окончании подписки, устройствах, отзывах и ошибках | • Поддержка с тикетами и FAQ<br>• Русский, английский и фарси с RTL и шрифтом Vazir<br>• Режим технических работ<br>• Редактор тарифов и пакетов устройств<br>• Редактор контента, оформления и функций прямо в админке<br>• Анимированные фоны с настройкой цветов, затемнения и скорости |

---

## 🛠️ Админ-панель

Управляйте системой, интерфейсом, тарифами, интеграциями, рассылками, промокодами и другими функциями прямо из Mini App.

<div align="center">
  <img src="docs/admin-menu.png" width="100%" alt="Админ-панель Link-Bot">
</div>

---

## 📋 Требования

| Компонент | Требование |
|---|---|
| 🖥️ Сервер | VPS с Ubuntu 22.04/24.04 или Debian 12 |
| 🌐 Домен | Домен с `A`-записью на IP сервера |
| 🔌 Порты | Открытые порты `22`, `80` и `443` |
| 🌊 Remnawave | Установленная и доступная панель Remnawave 2.x или 3.x |
| 🤖 Telegram | Бот, созданный через [@BotFather](https://t.me/BotFather) |

---

## 🚀 Быстрая установка

### 1. Подготовьте домен

Создайте у DNS-провайдера запись:

```text
Тип: A
Имя: bot
Значение: IP_ВАШЕГО_VPS
```

В примерах ниже используется домен `bot.example.com`. Дождитесь обновления DNS перед первым запуском.

### 2. Установите Docker и Git

```bash
apt update && apt install -y git curl
curl -fsSL https://get.docker.com | sh
systemctl enable --now docker
```

### 3. Скачайте Link-Bot

```bash
cd /opt
git clone https://github.com/bruhxax/Link-Bot.git
cd Link-Bot
```

### 4. Создайте `.env`

```bash
cp .env.example .env
nano .env
```

Минимально заполните:

```dotenv
TELEGRAM_TOKEN=токен_бота_от_BotFather
ADMIN_TELEGRAM_ID=ваш_telegram_id

REMNAWAVE_URL=https://panel.example.com
REMNAWAVE_TOKEN=токен_remnawave
REMNAWAVE_MODE=remote
CADDY_AUTH_API_TOKEN=
REMNAWAVE_HEADERS=
EGAMES_COOKIE=

POSTGRES_USER=linkbot
POSTGRES_PASSWORD=сложный_пароль
POSTGRES_DB=linkbot

PUBLIC_HOST=bot.example.com
PUBLIC_BASE_URL=https://bot.example.com
# Оставьте пустым для кабинета на основном домене, или укажите my для my.bot.example.com
CABINET_SUBDOMAIN=

REFERRAL_DAYS=0

# Общий язык Telegram-бота и Mini App: ru, en или fa
DEFAULT_LANGUAGE=ru
# auto включает Vazir для фарси и Montserrat для остальных языков
DEFAULT_FONT=auto
```

Если API панели защищён Caddy Security, укажите токен в
`CADDY_AUTH_API_TOKEN`. Бот будет отправлять его в заголовке `X-Api-Key`.
Дополнительные заголовки можно задать через `REMNAWAVE_HEADERS` в формате
`Header-One:value;Header-Two:value`.

### Панель установлена скриптом eGames

Link-Bot поддерживает eGames Reverse-Proxy. Выберите только один вариант
защиты, который был выбран при установке панели:

| Защита панели eGames | Что указать в .env |
|---|---|
| Cookie-защита Nginx | EGAMES_COOKIE=ИМЯ=ЗНАЧЕНИЕ |
| Страница входа TinyAuth | CADDY_AUTH_API_TOKEN=Basic base64(логин:пароль) |
| Caddy с MFA | CADDY_AUTH_API_TOKEN=ключ из API Keys |

Для cookie-защиты откройте /opt/remnawave/nginx.conf, найдите строку
map $http_cookie $auth_cookie и скопируйте пару из кавычек после ~*, например
aEmFnBcC=WbYWpixX. Вставьте только эту пару в EGAMES_COOKIE:

~~~dotenv
REMNAWAVE_URL=https://panel.example.com
REMNAWAVE_MODE=remote
REMNAWAVE_TOKEN=токен_панели
EGAMES_COOKIE=aEmFnBcC=WbYWpixX
~~~

Не вставляйте URL входа, Cookie: или Path=/. Если Link-Bot подключён к
контейнеру remnawave в той же Docker-сети, вместо внешнего URL можно указать
REMNAWAVE_URL=http://remnawave:3000 и REMNAWAVE_MODE=local; cookie тогда не нужна.

Сгенерировать пароль PostgreSQL:

```bash
openssl rand -hex 24
```

> [!IMPORTANT]
> Не добавляйте `https://` в `PUBLIC_HOST`.  
> В `PUBLIC_BASE_URL`, наоборот, нужен полный HTTPS-адрес.

Если нужен отдельный адрес кабинета, установите `CABINET_SUBDOMAIN=my` и создайте DNS-запись `A` для `my` с тем же IP, что у `PUBLIC_HOST`. Лендинг останется на основном домене, а кнопки «Кабинет» и ссылки на тарифы и поддержку откроют `https://my.bot.example.com/mini-app/`. При пустом значении кабинет остаётся на основном домене. В режиме `standalone` Caddy сам добавит сайт и получит сертификат после перезапуска Compose; при общем внешнем прокси добавьте тот же хост в его конфигурацию. Для входа через Telegram в браузере разрешите новый домен у BotFather. Если включён вход через Google, добавьте новый HTTPS-адрес в разрешённые источники и адреса перенаправления OAuth. Заполненный `CABINET_SUBDOMAIN` имеет приоритет над `MINI_APP_URL`.

### 5. Запустите бота

```bash
docker compose --profile standalone up -d --build
```

Caddy автоматически получит TLS-сертификат. Проверка:

```bash
docker compose ps
curl https://bot.example.com/healthcheck
```

Если на сервере уже работает общий Caddy или другой HTTPS-прокси, запускайте
только бота и PostgreSQL: `docker compose up -d --build bot`. Сервис `caddy`
в Compose включается только профилем `standalone`.

### 6. Выполните первый запуск

1. Откройте бота и отправьте `/start`.
2. Откройте Mini App под аккаунтом из `ADMIN_TELEGRAM_ID`.
3. Перейдите в раздел **Админка**.
4. Настройте интеграции, тарифы, триал, сквады, контент и функции. Язык и шрифт можно повторно выбрать в разделе **Админка → Язык и шрифт**.
5. В Mini App [@BotFather](https://t.me/BotFather?startapp) выберите бота → **Login Widget** и добавьте в **Allowed URLs** адреса `https://bot.example.com` и `https://bot.example.com/mini-app/`.
6. В **Login Widget → Advanced** оставьте стандартный алгоритм подписи `RS256`.

Браузерная версия использует новый **Log In With Telegram (OIDC)**: Telegram открывается по ссылке, а пользователь подтверждает вход без ручного ввода номера и кода. `id_token` проверяется на сервере по официальным ключам Telegram; Client Secret в `.env` не требуется.

Главная страница сайта (`/`) показывает публичный лендинг. При обычном открытии `/mini-app/` в браузере посетитель также попадает на лендинг; в Telegram Mini App открывается сразу. Кнопка **Кабинет** запускает прежнюю авторизацию и затем открывает Mini App. Прямые ссылки на разделы кабинета работают как раньше. Лендинг автоматически берёт бренд, цвета, доступные тарифы и контакты из настроек Mini App, а статус нод — из Remnawave; список обновляется, пока страница открыта. Отдельных настроек лендинга нет.

> [!NOTE]
> Платёжные ключи, тарифы, триал, промокоды, ссылки, баннеры и оформление задаются через админку. Хранить их в `.env` не требуется.

### Платёжные провайдеры

Откройте **Админка → Интеграции**, заполните поля нужного провайдера, сохраните и включите его. Затем скопируйте показанный там **Webhook URL** в настройки кассы.

Для **RollyPay** укажите API key и Webhook signing secret; в настройках кассы задайте выданный Link-Bot webhook как `callback_url`. Для **cisPay** укажите Shop ID, API key и один способ оплаты: `CARD` или `SBP`; в кабинете cisPay установите этот же webhook URL. Секреты хранятся в зашифрованной конфигурации и не выводятся обратно в админке.

---

## 🪪 Логотип Mini App

Откройте **Админка → Контент → Главное меню**, нажмите **Загрузить файл**, выберите PNG, JPG или WebP до 2 МБ и сохраните настройки. Link-Bot сам сохранит изображение в постоянном Docker-томе и подставит рабочий адрес — вручную копировать файл на сервер не нужно.

Для логотипа лучше использовать квадратное изображение от 256×256 px с прозрачным фоном. Внешнюю HTTPS-ссылку всё ещё можно указать в дополнительном поле. Если ссылка перестанет работать, Mini App покажет стандартный логотип вместо пустого места.

---

## 🖼️ Собственные баннеры

В конструкторе UI выберите **Добавить элемент → Баннер** и загрузите PNG, GIF или MP4 до 50 МБ. Тяните углы рамки для ручной кадрировки, двигайте медиа внутри неё и меняйте масштаб кнопками, колёсиком или двумя пальцами. Стрелки позволяют точно двигать медиа или выбранный угол. Исходный файл сохраняется без сжатия и перекодирования; GIF и MP4 продолжают воспроизводиться.

Размер баннера в конструкторе меняется пропорционально, а выбранный кадр сохраняется. Закругление, слой, перемещение и удаление доступны в настройках элемента. Баннер может быть статичным, открывать ссылку или раздел Mini App. После применения сохраните изменения конструктора.

Баннеры сообщений Telegram настраиваются отдельно:

Готовые баннеры в репозиторий не включены. Загрузите свои файлы в нужную папку:

```text
assets/telegram/menu/
assets/telegram/verification/
assets/telegram/commerce/
assets/telegram/success/
```

После загрузки укажите путь в редакторе контента, например:

```text
/assets/telegram/menu/banner.png
```

> Пустое поле означает отправку сообщения без баннера.

---

## 🧰 Полезные команды

Все команды выполняются из `/opt/Link-Bot`.

<details>
<summary><b>📊 Статус контейнеров</b></summary>

```bash
docker compose ps
```

</details>

<details>
<summary><b>📜 Логи бота</b></summary>

```bash
docker compose logs -f --tail=200 bot
```

</details>

<details>
<summary><b>🔐 Логи HTTPS-прокси</b></summary>

```bash
docker compose logs -f --tail=200 caddy
```

</details>

<details>
<summary><b>🔄 Перезапуск бота</b></summary>

```bash
docker compose restart bot
```

</details>

<details>
<summary><b>♻️ Перезапуск всего проекта</b></summary>

```bash
docker compose restart
```

</details>

<details>
<summary><b>⏯️ Остановка и запуск</b></summary>

```bash
docker compose stop
docker compose start
```

</details>

<details>
<summary><b>⬆️ Обновление</b></summary>

```bash
cd /opt/Link-Bot
git pull --ff-only
docker compose up -d --build bot
```

Если этот проект сам управляет Caddy, используйте
`docker compose --profile standalone up -d --build`.
Обновление сохраняет базу данных и настройки из админки. Уже созданные тарифы, оформление и интеграции не сбрасываются на новые значения по умолчанию.

</details>

<details>
<summary><b>↩️ Откат на предыдущую версию</b></summary>

```bash
cd /opt/Link-Bot
bash ./rollback.sh --dry-run  # показать выбранную версию без изменения контейнеров и БД
bash ./rollback.sh            # запустить предыдущий релиз
```

Для отката на конкретный более ранний релиз: `bash ./rollback.sh --to v2.1.7` (подставьте нужный тег). Команда собирает старый образ отдельно, создаёт резервную копию БД и заменяет **только контейнер бота**. Если контейнер сразу не запустится, она вернёт прежний образ. Исходники остаются на `main`, поэтому для возвращения к новой версии снова выполните команду обновления выше.

База данных автоматически назад не откатывается: уже применённые миграции включаются в старый образ. Если старый код несовместим с новыми данными, восстановите работавшую версию и проверьте логи. Путь к резервной копии команда выводит после её создания.

</details>

<details>
<summary><b>💾 Резервная копия базы</b></summary>

```bash
docker compose exec -T db sh -c 'pg_dump -U "$POSTGRES_USER" "$POSTGRES_DB"' > link-bot-backup.sql
```

</details>

<details>
<summary><b>🔁 Переезд с Bedolaga Bot</b></summary>

Этот инструмент переносит базу
[`remnawave-bedolaga-telegram-bot`](https://github.com/BEDOLAGA-DEV/remnawave-bedolaga-telegram-bot)
в Link-Bot. Перенесутся пользователи, баланс, реферальные связи и действующие
подписки с их сроком, ссылкой и Remnawave ID.

**Перед началом:** новый Link-Bot должен быть подключён к той же панели
Remnawave, что и Bedolaga. База Bedolaga должна быть доступна с VPS, где
запущен Link-Bot.

1. Остановите старый Bedolaga Bot и обновите Link-Bot:

   ```bash
   cd /opt/Link-Bot
   docker compose up -d --build bot
   ```

2. Сделайте резервную копию базы Link-Bot:

   ```bash
   docker compose exec -T db sh -c 'pg_dump -U "$POSTGRES_USER" "$POSTGRES_DB"' > link-bot-pre-bedolaga.sql
   ```

3. Укажите доступ к базе Bedolaga и запустите проверку. Она **ничего не меняет**:

   ```bash
   export BEDOLAGA_DATABASE_URL='postgres://USER:PASSWORD@BEDOLAGA_HOST:5432/DBNAME?sslmode=require'
   docker compose --profile tools run --rm migrate-bedolaga
   ```

4. Если числа в выводе верные, запустите сам перенос:

   ```bash
   docker compose --profile tools run --rm migrate-bedolaga --apply
   unset BEDOLAGA_DATABASE_URL
   ```

Импорт можно безопасно запустить повторно: баланс не зачислится дважды.
Не переносятся платежные реквизиты и история платежей, настройки Bedolaga,
а также отключённые, ограниченные и ожидающие подписки. При ошибке или
конфликте ничего не будет перенесено частично.

</details>

<details>
<summary><b>📥 Восстановление базы</b></summary>

```bash
cat link-bot-backup.sql | docker compose exec -T db sh -c 'psql -U "$POSTGRES_USER" "$POSTGRES_DB"'
```

</details>

<details>
<summary><b>🗑️ Удаление контейнеров без удаления базы</b></summary>

```bash
docker compose down
```

> [!CAUTION]
> `docker compose down -v` удаляет базу данных и настройки без возможности восстановления.

</details>

---

## 🏗️ Структура проекта

| Путь | Назначение |
|---|---|
| `cmd/` | Запуск приложения |
| `db/migrations/` | Миграции PostgreSQL |
| `internal/` | Логика бота, Mini App и интеграций |
| `translations/` | Тексты Telegram-бота |
| `assets/telegram/` | Пользовательские баннеры |
| `docker-compose.yaml` | Bot, PostgreSQL и Caddy |
| `.env.example` | Параметры первого запуска |

---

## 🔒 Безопасность

| Рекомендация | Описание |
|---|---|
| 🔑 Секреты | Не публикуйте `.env`, токены и резервные копии |
| 🐘 PostgreSQL | Используйте отдельный сложный пароль |
| 🛡️ SSH | Ограничьте SSH-доступ и используйте ключи вместо пароля |
| 💾 Обновления | Перед обновлением создавайте резервную копию базы |

---

## 💬 Сообщество

<div align="center">

<a href="https://t.me/REMNALinkBot">
  <img src="https://img.shields.io/badge/Telegram-Сообщество-2AABEE?style=for-the-badge&logo=telegram&logoColor=white" alt="Сообщество Link-Bot">
</a>
<a href="https://t.me/BruhvpnBot">
  <img src="https://img.shields.io/badge/Telegram-Попробовать%20бота-229ED9?style=for-the-badge&logo=telegram&logoColor=white" alt="Попробовать Link-Bot">
</a>

**Вопросы и обсуждения:** [t.me/REMNALinkBot](https://t.me/REMNALinkBot)  
**Демонстрация бота:** [t.me/BruhvpnBot](https://t.me/BruhvpnBot)

</div>

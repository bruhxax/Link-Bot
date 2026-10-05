<div align="center">

<b>Русский</b> · <a href="README_EN.md">English</a>

# Link-Bot

Telegram-бот, Mini App и веб-кабинет для VPN-подписок Remnawave.

<p>
  <a href="https://t.me/BruhvpnBot"><img src="https://img.shields.io/badge/%D0%94%D0%B5%D0%BC%D0%BE-229ED9?style=for-the-badge&amp;logo=telegram&amp;logoColor=white" alt="Открыть демо"></a>
  <a href="#установка"><img src="https://img.shields.io/badge/%D0%A3%D1%81%D1%82%D0%B0%D0%BD%D0%BE%D0%B2%D0%B8%D1%82%D1%8C-22C55E?style=for-the-badge&amp;logo=docker&amp;logoColor=white" alt="Установка"></a>
  <a href="docs/configuration.md"><img src="https://img.shields.io/badge/%D0%9D%D0%B0%D1%81%D1%82%D1%80%D0%BE%D0%B9%D0%BA%D0%B8-6366F1?style=for-the-badge&amp;logo=readthedocs&amp;logoColor=white" alt="Документация"></a>
  <a href="https://t.me/REMNALinkBot"><img src="https://img.shields.io/badge/%D0%A1%D0%BE%D0%BE%D0%B1%D1%89%D0%B5%D1%81%D1%82%D0%B2%D0%BE-374151?style=for-the-badge&amp;logo=telegram&amp;logoColor=white" alt="Сообщество"></a>
</p>

<img src="docs/Link-Bot.png" width="100%" alt="Интерфейс Link-Bot">

</div>

## Возможности

| Кабинет | Продажи | Администрирование |
| :--- | :--- | :--- |
| Telegram и браузер | Тарифы, триал, устройства и трафик | Интеграции, оформление и контент |
| Подписки, баланс и история | Платёжные системы и Telegram Stars | Финансы, GA4 и Яндекс Метрика |
| Тикеты, FAQ и ИИ-помощник | Промокоды, рефералы и награды за отзывы | Рассылки и языки: RU / EN / FA |

## Установка

Нужны VPS с Ubuntu 22.04/24.04 или Debian 12, Docker Compose, доступная [панель Remnawave](https://github.com/remnawave/panel) и бот от [@BotFather](https://t.me/BotFather). Создайте `A`-запись домена на IP сервера; откройте порты `80` и `443`.

<details>
<summary>Установить Docker и Git</summary>

Команды для сервера, от имени root:

```bash
apt update && apt install -y git curl
curl -fsSL https://get.docker.com | sh
systemctl enable --now docker
```

</details>

**1. Скачайте проект**

```bash
cd /opt
git clone https://github.com/bruhxax/Link-Bot.git
cd Link-Bot
cp .env.example .env
nano .env
```

**2. Заполните основные параметры `.env`**

```dotenv
TELEGRAM_TOKEN=токен_бота
ADMIN_TELEGRAM_ID=ваш_telegram_id
REMNAWAVE_URL=https://panel.example.com
REMNAWAVE_TOKEN=токен_панели
POSTGRES_PASSWORD=замените_на_случайный_пароль
PUBLIC_HOST=bot.example.com
PUBLIC_BASE_URL=https://bot.example.com
```

Пароль БД можно получить командой `openssl rand -hex 24`. Остальные параметры — в [.env.example](.env.example).

**3. Запустите**

```bash
docker compose --profile standalone up -d --build
docker compose ps
curl https://bot.example.com/healthcheck
```

Замените `bot.example.com` своим доменом. Caddy автоматически настроит HTTPS. Если HTTPS-прокси уже настроен, используйте `docker compose up -d --build bot` и направьте его на сервис `bot:8080`.

Отправьте боту `/start`, откройте Mini App под аккаунтом администратора и настройте тарифы и платежи в **Админке**. [Подключение входа и дополнительные настройки →](docs/configuration.md)

## Команды

Выполняйте из `/opt/Link-Bot`.

| Действие | Команда |
| :--- | :--- |
| Статус | `docker compose ps` |
| Логи | `docker compose logs -f --tail=200 bot` |
| Перезапуск бота | `docker compose restart bot` |
| Проверка отката | `bash ./rollback.sh --dry-run` |

**Обновление**

```bash
cd /opt/Link-Bot
bash ./update.sh
```

Скрипт скачает обновление и применит `.env`, сохранив базу и настройки админки.

## Документация

| Раздел | Что внутри |
| :--- | :--- |
| [Настройки](docs/configuration.md) | Вход, домены, почта, ИИ и аналитика |
| [Платёжные системы](docs/payment-providers.md) | Ключи, вебхуки и API провайдеров |
| [Обслуживание](docs/maintenance.md) | Бэкапы, откат и перенос из Bedolaga |
| [Баннеры Telegram](assets/telegram/README.md) | Папки и пути к изображениям |

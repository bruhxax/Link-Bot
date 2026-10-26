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

В **Контент → Telegram → Главное меню → Закреплённое сообщение** можно написать сообщение в чате бота, проверить его и включить закреп при `/start`. Сначала пользователь получает закреплённое сообщение, затем обычное меню. Актуальная закреплённая копия не дублируется; откреплённая закрепляется снова, удалённая восстанавливается. Сохраняются форматирование, premium emoji и вложения. Массовой отправки нет. Выключение снимает только этот закреп.

В редакторе **Тарифы → Порядок тарифов и способов оплаты** карандаш рядом с платёжкой позволяет задать название и описание для выбора оплаты в кабинете. Пустые поля используют системные значения. Нажмите «Готово», затем сохраните изменения в тарифах.

В **Подарить → Способ оплаты → Баланс** подарок можно оплатить внутренним балансом с учётом промокода. Показаны доступная сумма и нехватка средств. Оплата доступна, если в **Рефералы и баланс** включены покупки с баланса; получателю, который ещё не заходил в бот, подарок будет выдан при входе.

В **Рассылки → Telegram** включите **Временная рассылка**, чтобы сообщение автоматически удалялось у каждого получателя. Выберите 1, 6, 12, 24 или 47 часов либо свой срок в минутах. Срок отсчитывается с отправки каждой копии, сохраняется в шаблоне и продолжает выполняться после перезапуска бота. Отключённый переключатель оставляет обычную бессрочную рассылку. Максимум 47 часов оставляет запас до ограничения Telegram в 48 часов; если бот был выключен дольше этого окна, Telegram уже не позволит удалить сообщение.

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

## Перенос из Bedolaga

Переносятся **пользователи, баланс, рефералы и активные/пробные подписки**. Оба бота должны использовать одну панель Remnawave; база PostgreSQL Bedolaga должна быть доступна контейнеру Link-Bot.

Сначала установите и запустите Link-Bot, остановите бот Bedolaga и сделайте резервные копии баз. Выполните из `/opt/Link-Bot`, подставив в ссылку данные базы Bedolaga:

```bash
export BEDOLAGA_DATABASE_URL='postgres://USER:PASSWORD@BEDOLAGA_HOST:5432/DBNAME'

# Проверка без изменения данных
docker compose --profile tools run --rm migrate-bedolaga

# Проверьте отчёт и примените перенос
docker compose --profile tools run --rm migrate-bedolaga --apply
unset BEDOLAGA_DATABASE_URL
```

Повторный импорт не зачисляет баланс дважды. История платежей, настройки и недействующие подписки не переносятся. [Подробности и команда бэкапа →](docs/maintenance.md#перенос-из-bedolaga)

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
| [Синхронизация подписок](docs/subscription-sync.md) | Ручные правки панели, покупки и пакеты устройств |
| [Баннеры Telegram](assets/telegram/README.md) | Папки и пути к изображениям |

<div align="center">

<a href="README.md">Русский</a> · <b>English</b>

# Link-Bot

Telegram bot, Mini App and web cabinet for Remnawave VPN subscriptions.

<p>
  <a href="https://t.me/BruhvpnBot"><img src="https://img.shields.io/badge/Demo-229ED9?style=for-the-badge&amp;logo=telegram&amp;logoColor=white" alt="Try the demo"></a>
  <a href="#installation"><img src="https://img.shields.io/badge/Install-22C55E?style=for-the-badge&amp;logo=docker&amp;logoColor=white" alt="Installation"></a>
  <a href="docs/configuration.md"><img src="https://img.shields.io/badge/Settings-6366F1?style=for-the-badge&amp;logo=readthedocs&amp;logoColor=white" alt="Documentation in Russian"></a>
  <a href="https://t.me/REMNALinkBot"><img src="https://img.shields.io/badge/Community-374151?style=for-the-badge&amp;logo=telegram&amp;logoColor=white" alt="Community"></a>
</p>

<img src="docs/Link-Bot.png" width="100%" alt="Link-Bot interface">

</div>

## Features

| Cabinet | Sales | Administration |
| :--- | :--- | :--- |
| Telegram and browser | Plans, trials, devices and traffic | Integrations, appearance and content |
| Subscriptions, balance and history | Payment providers and Telegram Stars | Finances, GA4 and Yandex Metrika |
| Tickets, FAQ and AI support | Promo codes, referrals and review rewards | Broadcasts and languages: RU / EN / FA |

## Installation

You need a VPS with Ubuntu 22.04/24.04 or Debian 12, Docker Compose, an accessible [Remnawave panel](https://github.com/remnawave/panel) and a bot from [@BotFather](https://t.me/BotFather). Point the domain's `A` record to your server and open ports `80` and `443`.

<details>
<summary>Install Docker and Git</summary>

Run on the server as root:

```bash
apt update && apt install -y git curl
curl -fsSL https://get.docker.com | sh
systemctl enable --now docker
```

</details>

**1. Download the project**

```bash
cd /opt
git clone https://github.com/bruhxax/Link-Bot.git
cd Link-Bot
cp .env.example .env
nano .env
```

**2. Set the main values in `.env`**

```dotenv
TELEGRAM_TOKEN=your_bot_token
ADMIN_TELEGRAM_ID=your_telegram_id
REMNAWAVE_URL=https://panel.example.com
REMNAWAVE_TOKEN=your_panel_token
POSTGRES_PASSWORD=replace_with_a_random_password
PUBLIC_HOST=bot.example.com
PUBLIC_BASE_URL=https://bot.example.com
```

Generate a database password with `openssl rand -hex 24`. Other settings are in [.env.example](.env.example).

**3. Start**

```bash
docker compose --profile standalone up -d --build
docker compose ps
curl https://bot.example.com/healthcheck
```

Replace `bot.example.com` with your domain. Caddy sets up HTTPS automatically. If you already have an HTTPS proxy configured, use `docker compose up -d --build bot` and route it to the `bot:8080` service.

Send `/start` to your bot, open the Mini App as the administrator, then configure plans and payments in **Admin**. [Login and additional settings →](docs/configuration.md) (Russian)

### Separate admin host

Set `ADMIN_SUBDOMAIN=admin` in `.env` to serve administration at `https://admin.<PUBLIC_HOST>/`. The value is a single DNS label, without a protocol, dots or a path, and must differ from `CABINET_SUBDOMAIN`. Leave `ADMIN_SUBDOMAIN=` empty to retain both the original `/mini-app/?page=admin` address and the original admin interface, including its existing plan and layout editors. The new interface is enabled only on the configured admin host; `/admin` is also available.

Create an A record for the admin hostname pointing to the Link-Bot server before running `bash update.sh` (or `bash update.sh --standalone`). Any AAAA record must reach the same server. The existing update workflow configures the managed or shared Caddy and verifies HTTPS. With a manually managed proxy, route the admin host to `bot:8080` and preserve the request Host.

The admin UI uses React, Mantine and the Remnawave table library, with top navigation, a mobile drawer and separate forms. Plans and the UI builder open inside administration. User cards expose panel contacts, tags, description, HWID, traffic limits/reset, expiry and squads; existing bot operations remain available. Frontend sources and license notices are in admin-ui/. Run npm ci and npm run build there after source changes; the generated assets are embedded in Go, so production does not require Node.js. The new interface preserves existing operations and role permissions. The admin host rejects customer accounts, legacy admin links redirect, and admin API routes require the configured host. Customer URLs and payment webhooks remain on their original host. Sign in again on the new origin; Telegram QR uses the existing bot. Add the new origin and redirect URL to existing Telegram OIDC / Google provider settings when those login methods are used. Enable browser Push again on the new origin.

## Migration from Bedolaga

Imports **users, balances, referrals and active/trial subscriptions**. Both bots must use the same Remnawave panel, and the Bedolaga PostgreSQL database must be reachable from the Link-Bot container.

First install and start Link-Bot, stop the Bedolaga bot and back up both databases. Run from `/opt/Link-Bot`, replacing the URL placeholders with your Bedolaga database connection details:

```bash
export BEDOLAGA_DATABASE_URL='postgres://USER:PASSWORD@BEDOLAGA_HOST:5432/DBNAME'

# Preview without changing data
docker compose --profile tools run --rm migrate-bedolaga

# Review the report, then apply the migration
docker compose --profile tools run --rm migrate-bedolaga --apply
unset BEDOLAGA_DATABASE_URL
```

Repeating the import does not credit balances twice. Payment history, settings and inactive subscriptions are not imported. [Details and backup command →](docs/maintenance.md#перенос-из-bedolaga) (Russian)

## Commands

Run from `/opt/Link-Bot`.

| Action | Command |
| :--- | :--- |
| Status | `docker compose ps` |
| Logs | `docker compose logs -f --tail=200 bot` |
| Restart bot | `docker compose restart bot` |
| Preview rollback | `bash ./rollback.sh --dry-run` |

**Update**

```bash
cd /opt/Link-Bot
bash ./update.sh
```

The script pulls updates and applies `.env`, preserving the database and admin settings.

## Documentation

Detailed guides are in Russian; the Telegram banner guide is in English.

| Guide | Contents |
| :--- | :--- |
| [Configuration](docs/configuration.md) | Login, domains, email, AI and analytics |
| [Payment providers](docs/payment-providers.md) | Keys, webhooks and provider APIs |
| [Maintenance](docs/maintenance.md) | Backups, rollback and Bedolaga migration |
| [Telegram banners](assets/telegram/README.md) | Directories and image paths |

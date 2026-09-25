<div align="center">

<p>
  <a href="README.md">Русский</a> · <b>English</b>
</p>

<img src="docs/Link-Bot.png" width="100%" alt="Link-Bot interface">

# Link-Bot

**Telegram bot and Mini App for selling and managing Remnawave VPN subscriptions**

<p>
  <a href="https://t.me/BruhvpnBot">
    <img src="https://img.shields.io/badge/Telegram-Try%20Bot-2AABEE?style=for-the-badge&logo=telegram&logoColor=white" alt="Try Link-Bot">
  </a>
  <a href="https://t.me/REMNALinkBot">
    <img src="https://img.shields.io/badge/Telegram-Community-229ED9?style=for-the-badge&logo=telegram&logoColor=white" alt="Link-Bot community">
  </a>
</p>

<p>
  <img src="https://img.shields.io/badge/Go-1.25-00ADD8?style=flat-square&logo=go&logoColor=white" alt="Go 1.25">
  <img src="https://img.shields.io/badge/PostgreSQL-17-4169E1?style=flat-square&logo=postgresql&logoColor=white" alt="PostgreSQL 17">
  <img src="https://img.shields.io/badge/Docker-Compose-2496ED?style=flat-square&logo=docker&logoColor=white" alt="Docker Compose">
</p>

[Try the bot](https://t.me/BruhvpnBot) · [Telegram community](https://t.me/REMNALinkBot) · [Remnawave](https://github.com/remnawave/panel)

</div>

---

## 🧩 What is Link-Bot?

**Link-Bot** is a Telegram bot and Mini App for selling and managing Remnawave VPN subscriptions.

It combines a user dashboard, payments, subscription management, support, and administration in one interface.

> 🤖 **Demo:** [open Link-Bot in Telegram](https://t.me/BruhvpnBot)  
> 💬 **Community:** [news, questions, and discussions](https://t.me/REMNALinkBot)

---

## ✨ Features

| 📦 Subscriptions and plans | 💳 Payments |
|---|---|
| • Personal dashboard in Telegram Mini App and browser<br>• Create and renew Remnawave subscriptions<br>• Plans, trials, and internal/external squad selection<br>• Link and transfer subscriptions between Telegram accounts | • YooKassa<br>• Crypto Pay<br>• Telegram Stars<br>• Lava<br>• WATA<br>• Platega<br>• FreeKassa<br>• Heleket<br>• Pally |

| 📣 Marketing and notifications | 🛠️ Administration |
|---|---|
| • Promo codes<br>• Referral system<br>• Broadcasts<br>• Subscription expiration and error notifications | • Support tickets and FAQ<br>• Russian, English, and Persian with RTL and the Vazir font<br>• Maintenance mode<br>• Content, appearance, and feature editor directly in the admin panel<br>• Animated backgrounds with color, dimming, and speed controls |

---

## 🛠️ Admin panel

Manage the system, interface, plans, integrations, broadcasts, promo codes, and other features directly from the Mini App.

<div align="center">
  <img src="docs/admin-menu.png" width="100%" alt="Link-Bot admin panel">
</div>

---

## 📋 Requirements

| Component | Requirement |
|---|---|
| 🖥️ Server | VPS running Ubuntu 22.04/24.04 or Debian 12 |
| 🌐 Domain | A domain with an `A` record pointing to the server IP |
| 🔌 Ports | Open ports `22`, `80`, and `443` |
| 🌊 Remnawave | Installed and accessible Remnawave 2.x or 3.x panel |
| 🤖 Telegram | A bot created via [@BotFather](https://t.me/BotFather) |

---

## 🚀 Quick installation

### 1. Prepare the domain

Create the following DNS record:

```text
Type: A
Name: bot
Value: YOUR_VPS_IP
```

The examples below use `bot.example.com`. Wait for DNS propagation before the first launch.

### 2. Install Docker and Git

```bash
apt update && apt install -y git curl
curl -fsSL https://get.docker.com | sh
systemctl enable --now docker
```

### 3. Download Link-Bot

```bash
cd /opt
git clone https://github.com/bruhxax/Link-Bot.git
cd Link-Bot
```

### 4. Create `.env`

```bash
cp .env.example .env
nano .env
```

Fill in at least these values:

```dotenv
TELEGRAM_TOKEN=bot_token_from_BotFather
ADMIN_TELEGRAM_ID=your_telegram_id

REMNAWAVE_URL=https://panel.example.com
REMNAWAVE_TOKEN=remnawave_token
REMNAWAVE_MODE=remote
CADDY_AUTH_API_TOKEN=
REMNAWAVE_HEADERS=

POSTGRES_USER=linkbot
POSTGRES_PASSWORD=strong_password
POSTGRES_DB=linkbot

PUBLIC_HOST=bot.example.com
PUBLIC_BASE_URL=https://bot.example.com

REFERRAL_DAYS=0

# Global Telegram bot and Mini App language: ru, en, or fa
DEFAULT_LANGUAGE=ru
# auto uses Vazir for Persian and Montserrat for other languages
DEFAULT_FONT=auto
```

If the panel API is protected by Caddy Security, set
`CADDY_AUTH_API_TOKEN`. Link-Bot sends it as the `X-Api-Key` header.
Additional panel headers can be configured through `REMNAWAVE_HEADERS` using
`Header-One:value;Header-Two:value`.

Generate a PostgreSQL password:

```bash
openssl rand -hex 24
```

> [!IMPORTANT]
> Do not include `https://` in `PUBLIC_HOST`.  
> `PUBLIC_BASE_URL`, on the other hand, must contain the full HTTPS URL.

To host the cabinet at `my.bot.example.com`, set `CABINET_SUBDOMAIN=my` and point a DNS A record for `my` to the same IP as `PUBLIC_HOST`. Leave it empty to keep the cabinet on the main domain. The update command below configures either the bundled Caddy or an existing `link-bot-caddy` container with a host-mounted Caddyfile. It validates and reloads the shared config while preserving its other sites.

### 5. Start the bot

```bash
docker compose --profile standalone up -d --build
```

Caddy will automatically obtain a TLS certificate. Check the deployment:

```bash
docker compose ps
curl https://bot.example.com/healthcheck
```

If the server already has a shared Caddy or another HTTPS proxy, start only
the bot and PostgreSQL: `docker compose up -d --build bot`. The Compose
`caddy` service is enabled only with the `standalone` profile.

### 6. Complete the first launch

1. Open the bot and send `/start`.
2. Open the Mini App using the account specified in `ADMIN_TELEGRAM_ID`.
3. Go to **Admin**.
4. Configure integrations, plans, trial access, squads, content, and features. Language and font can also be changed under **Admin → Language and font**.
5. Open the [@BotFather](https://t.me/BotFather?startapp) Mini App, select your bot → **Login Widget**, then add `https://bot.example.com` and `https://bot.example.com/mini-app/` to **Allowed URLs**.
6. Keep the default `RS256` signing algorithm under **Login Widget → Advanced**.

The browser dashboard uses the new **Log In With Telegram (OIDC)** flow: Telegram opens from a link and the user confirms sign-in without manually entering a phone number or code. The server validates the returned `id_token` against Telegram's official signing keys; no Client Secret is required in `.env`.

> [!NOTE]
> Payment keys, plans, trial settings, promo codes, links, banners, and visual appearance are configured through the admin panel. They do not need to be stored in `.env`.

---

## 🪪 Mini App logo

Open **Admin → Content → Main menu**, click **Upload file**, select a PNG, JPG, or WebP image up to 2 MB, and save the settings. Link-Bot stores the image in a persistent Docker volume and inserts the correct URL automatically, so there is no need to copy a file to the server manually.

A square image of at least 256×256 px with a transparent background works best. An external HTTPS URL is still available under the advanced option. If that URL stops working, the Mini App shows the default logo instead of an empty space.

---

## 🖼️ Custom banners

In the UI builder, choose **Add element → Banner** and upload a PNG, GIF, or MP4 up to 50 MB. Drag the frame corners to crop, move the media inside the frame, and zoom with the buttons, mouse wheel, or pinch gesture. Arrow keys provide precise adjustments for the media or focused corner. Original files are stored without compression or transcoding; GIF and MP4 playback is retained.

Resizing a banner in the builder keeps its proportions and saved crop. Element settings provide corner rounding, layers, positioning, and deletion. Banners can be static, open a link, or navigate to a Mini App section. Apply the banner, then save the builder changes.

Telegram message banners are configured separately:

Ready-made banners are not included in the repository. Upload your own files to the required directories:

```text
assets/telegram/menu/
assets/telegram/verification/
assets/telegram/commerce/
assets/telegram/success/
```

Then specify the path in the content editor, for example:

```text
/assets/telegram/menu/banner.png
```

> An empty field means the message will be sent without a banner.

---

## 🧰 Useful commands

Run all commands from `/opt/Link-Bot`.

<details>
<summary><b>📊 Container status</b></summary>

```bash
docker compose ps
```

</details>

<details>
<summary><b>📜 Bot logs</b></summary>

```bash
docker compose logs -f --tail=200 bot
```

</details>

<details>
<summary><b>🔐 HTTPS proxy logs</b></summary>

```bash
docker compose logs -f --tail=200 caddy
```

</details>

<details>
<summary><b>🔄 Restart the bot</b></summary>

```bash
docker compose restart bot
```

</details>

<details>
<summary><b>♻️ Restart the entire project</b></summary>

```bash
docker compose restart
```

</details>

<details>
<summary><b>⏯️ Stop and start</b></summary>

```bash
docker compose stop
docker compose start
```

</details>

<details>
<summary><b>⬆️ Update</b></summary>

```bash
cd /opt/Link-Bot
bash ./update.sh
```

When moving from `v2.1.13`, first run `cd /opt/Link-Bot && git pull --ff-only && bash ./update.sh` once. After that, the command above pulls updates and applies `.env` changes. When changing domains, set both `PUBLIC_HOST=new.example.com` and `PUBLIC_BASE_URL=https://new.example.com` in `.env`. The script recreates the bot and the existing bundled Caddy using Caddy's original Compose project, or starts bundled Caddy if none exists. It updates the cabinet host in an accessible external Caddyfile and checks both HTTPS endpoints. It also adds `CABINET_SUBDOMAIN=` to an existing `.env` if missing, without overwriting a configured value.
Updating preserves the database and admin panel settings. Existing plans, appearance settings, and integrations are not reset to new default values.

</details>

<details>
<summary><b>↩️ Roll back to the previous release</b></summary>

```bash
cd /opt/Link-Bot
bash ./rollback.sh --dry-run  # show the target without changing containers or DB
bash ./rollback.sh            # run the previous release
```

To select a specific older release, use `bash ./rollback.sh --to v2.1.7` with the desired tag. The command builds the older image separately, backs up PostgreSQL, and replaces **only the bot container**. If the container fails immediately after startup, it restores the previous image. The source checkout stays on `main`; run the update command above to return to the latest version.

The database is not downgraded automatically. Already-applied migrations are included in the older image. If older code cannot use newer data, restore the working version and inspect the logs. The command prints the backup path.

</details>

<details>
<summary><b>💾 Database backup</b></summary>

```bash
docker compose exec -T db sh -c 'pg_dump -U "$POSTGRES_USER" "$POSTGRES_DB"' > link-bot-backup.sql
```

</details>

<details>
<summary><b>📥 Restore the database</b></summary>

```bash
cat link-bot-backup.sql | docker compose exec -T db sh -c 'psql -U "$POSTGRES_USER" "$POSTGRES_DB"'
```

</details>

<details>
<summary><b>🗑️ Remove containers without deleting the database</b></summary>

```bash
docker compose down
```

> [!CAUTION]
> `docker compose down -v` permanently deletes the database and all settings.

</details>

---

## 🏗️ Project structure

| Path | Purpose |
|---|---|
| `cmd/` | Application startup |
| `db/migrations/` | PostgreSQL migrations |
| `internal/` | Bot, Mini App, and integration logic |
| `translations/` | Telegram bot texts |
| `assets/telegram/` | Custom banners |
| `docker-compose.yaml` | Bot, PostgreSQL, and Caddy |
| `.env.example` | Initial configuration parameters |

---

## 🔒 Security

| Recommendation | Description |
|---|---|
| 🔑 Secrets | Never publish `.env`, tokens, or backups |
| 🐘 PostgreSQL | Use a separate strong password |
| 🛡️ SSH | Restrict SSH access and use keys instead of passwords |
| 💾 Updates | Create a database backup before updating |

---

## 💬 Community

<div align="center">

<a href="https://t.me/REMNALinkBot">
  <img src="https://img.shields.io/badge/Telegram-Community-2AABEE?style=for-the-badge&logo=telegram&logoColor=white" alt="Link-Bot community">
</a>
<a href="https://t.me/BruhvpnBot">
  <img src="https://img.shields.io/badge/Telegram-Try%20the%20bot-229ED9?style=for-the-badge&logo=telegram&logoColor=white" alt="Try Link-Bot">
</a>

**Questions and discussions:** [t.me/REMNALinkBot](https://t.me/REMNALinkBot)  
**Bot demo:** [t.me/BruhvpnBot](https://t.me/BruhvpnBot)

</div>

# Telegram banners

Link-Bot does not include built-in banners. Add your own files to the matching
directory and select their path in the mini-app content editor:

- `menu/` - main bot menu
- `verification/` - channel verification
- `commerce/` - tariffs and payment screens
- `success/` - successful purchase

For example, a file at `assets/telegram/menu/banner.png` is available to the
bot as `/assets/telegram/menu/banner.png`. Leave the editor field empty to send
the Telegram screen without a banner.

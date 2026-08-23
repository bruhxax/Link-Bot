const label = (ru, en, fa) => Object.freeze({ ru, en, fa });
const link = (ru, en, fa, url) => Object.freeze({ label: label(ru, en, fa), url });

export const SETUP_PLATFORMS = Object.freeze([
  {
    id: "ios",
    name: label("iOS", "iOS", "iOS"),
    icon: "apple",
    apps: [
      {
        id: "happ",
        name: "Happ",
        featured: true,
        scheme: "happ://add/",
        links: [
          link("App Store (RU)", "App Store (RU)", "App Store (RU)", "https://apps.apple.com/ru/app/happ-proxy-utility-plus/id6746188973"),
          link("App Store (Global)", "App Store (Global)", "App Store (Global)", "https://apps.apple.com/us/app/happ-proxy-utility/id6504287215"),
        ],
      },
      {
        id: "stash",
        name: "Stash",
        scheme: "stash://install-config?url=",
        links: [link("Открыть в App Store", "Open in App Store", "باز کردن در App Store", "https://apps.apple.com/us/app/stash-rule-based-proxy/id1596063349")],
      },
      {
        id: "streisand",
        name: "Streisand",
        scheme: "streisand://import/",
        links: [link("Открыть в App Store", "Open in App Store", "باز کردن در App Store", "https://apps.apple.com/ru/app/streisand/id6450534064")],
      },
      {
        id: "shadowrocket",
        name: "Shadowrocket",
        scheme: "sub://",
        base64: true,
        links: [link("Открыть в App Store", "Open in App Store", "باز کردن در App Store", "https://apps.apple.com/ru/app/shadowrocket/id932747118")],
      },
      {
        id: "clash-mi",
        name: "Clash Mi",
        scheme: "clash://install-config?overwrite=no&name=Remnawave&url=",
        links: [link("Открыть в App Store", "Open in App Store", "باز کردن در App Store", "https://apps.apple.com/us/app/clash-mi/id6744321968")],
      },
    ],
  },
  {
    id: "android",
    name: label("Android", "Android", "Android"),
    icon: "android",
    apps: [
      {
        id: "flclashx",
        name: "FlClashX",
        featured: true,
        scheme: "flclashx://install-config?url=",
        links: [link("Скачать APK", "Download APK", "دانلود APK", "https://github.com/pluralplay/FlClashX/releases/download/v0.2.1/FlClashX-0.2.1-android-arm64-v8a.apk")],
      },
      {
        id: "clash-meta",
        name: "Clash Meta",
        featured: true,
        scheme: "clashmeta://install-config?name=Remnawave&url=",
        links: [
          link("Скачать APK", "Download APK", "دانلود APK", "https://github.com/MetaCubeX/ClashMetaForAndroid/releases/download/v2.11.20/cmfa-2.11.20-meta-universal-release.apk"),
          link("Открыть в F-Droid", "Open in F-Droid", "باز کردن در F-Droid", "https://f-droid.org/packages/com.github.metacubex.clash.meta/"),
        ],
      },
      {
        id: "happ",
        name: "Happ",
        featured: true,
        scheme: "happ://add/",
        links: [
          link("Открыть в Google Play", "Open in Google Play", "باز کردن در Google Play", "https://play.google.com/store/apps/details?id=com.happproxy"),
          link("Скачать APK", "Download APK", "دانلود APK", "https://github.com/Happ-proxy/happ-android/releases/latest/download/Happ.apk"),
        ],
      },
      {
        id: "v2rayng",
        name: "v2rayNG",
        scheme: "v2rayng://install-config?name=Remnawave&url=",
        links: [link("Скачать APK", "Download APK", "دانلود APK", "https://github.com/2dust/v2rayNG/releases/download/1.10.31/v2rayNG_1.10.31_universal.apk")],
      },
      {
        id: "exclave",
        name: "Exclave",
        scheme: "exclave://subscription?url=",
        links: [
          link("Скачать APK", "Download APK", "دانلود APK", "https://github.com/dyhkwong/Exclave/releases/download/0.17.4/Exclave-0.17.4-arm64-v8a.apk"),
          link("Открыть в F-Droid", "Open in F-Droid", "باز کردن در F-Droid", "https://f-droid.org/packages/com.github.dyhkwong.sagernet"),
        ],
      },
    ],
  },
  {
    id: "macos",
    name: label("macOS", "macOS", "macOS"),
    icon: "apple",
    apps: [
      {
        id: "happ",
        name: "Happ",
        featured: true,
        scheme: "happ://add/",
        links: [
          link("App Store (RU)", "App Store (RU)", "App Store (RU)", "https://apps.apple.com/ru/app/happ-proxy-utility-plus/id6746188973"),
          link("App Store (Global)", "App Store (Global)", "App Store (Global)", "https://apps.apple.com/us/app/happ-proxy-utility/id6504287215"),
        ],
      },
      {
        id: "koala-clash",
        name: "Koala Clash",
        featured: true,
        scheme: "koala-clash://install-config?url=",
        links: [
          link("Apple Silicon", "Apple Silicon", "Apple Silicon", "https://github.com/coolcoala/clash-verge-rev-lite/releases/latest/download/Koala.Clash_aarch64.dmg"),
          link("Intel", "Intel", "Intel", "https://github.com/coolcoala/clash-verge-rev-lite/releases/latest/download/Koala.Clash_x64.dmg"),
        ],
      },
      {
        id: "flclashx",
        name: "FlClashX",
        featured: true,
        scheme: "flclashx://install-config?url=",
        links: [
          link("Apple Silicon", "Apple Silicon", "Apple Silicon", "https://github.com/pluralplay/FlClashX/releases/download/v0.2.1/FlClashX-0.2.1-macos-arm64.dmg"),
          link("Intel", "Intel", "Intel", "https://github.com/pluralplay/FlClashX/releases/download/v0.2.1/FlClashX-0.2.1-macos-amd64.dmg"),
        ],
      },
      {
        id: "prizrak-box",
        name: "Prizrak-Box",
        scheme: "prizrak-box://install-config?url=",
        links: [
          link("Apple Silicon", "Apple Silicon", "Apple Silicon", "https://github.com/legiz-ru/Prizrak-Box/releases/latest/download/macos-arm64-dmg.zip"),
          link("Intel", "Intel", "Intel", "https://github.com/legiz-ru/Prizrak-Box/releases/latest/download/macos-amd64-dmg.zip"),
        ],
      },
      {
        id: "clash-verge",
        name: "Clash Verge",
        scheme: "clash://install-config?url=",
        links: [
          link("Intel", "Intel", "Intel", "https://github.com/clash-verge-rev/clash-verge-rev/releases/download/v2.4.4-rc/Clash.Verge_2.4.4-rc_x64.dmg"),
          link("Apple Silicon", "Apple Silicon", "Apple Silicon", "https://github.com/clash-verge-rev/clash-verge-rev/releases/download/v2.4.4-rc/Clash.Verge_2.4.4-rc_aarch64.dmg"),
        ],
      },
    ],
  },
  {
    id: "windows",
    name: label("Windows", "Windows", "Windows"),
    icon: "windows",
    apps: [
      {
        id: "flclashx",
        name: "FlClashX",
        featured: true,
        scheme: "flclashx://install-config?url=",
        links: [
          link("Установщик x64", "x64 installer", "نصب کننده x64", "https://github.com/pluralplay/FlClashX/releases/download/v0.2.1/FlClashX-0.2.1-windows-amd64-setup.exe"),
          link("Портативная x64", "Portable x64", "نسخه قابل حمل x64", "https://github.com/pluralplay/FlClashX/releases/download/v0.2.1/FlClashX-0.2.1-windows-amd64.zip"),
          link("Версия ARM", "ARM version", "نسخه ARM", "https://github.com/pluralplay/FlClashX/releases/download/v0.2.1/FlClashX-0.2.1-windows-arm64-setup.exe"),
          link("Портативная ARM", "Portable ARM", "نسخه قابل حمل ARM", "https://github.com/pluralplay/FlClashX/releases/download/v0.2.1/FlClashX-0.2.1-windows-arm64.zip"),
        ],
      },
      {
        id: "koala-clash",
        name: "Koala Clash",
        featured: true,
        scheme: "koala-clash://install-config?url=",
        links: [
          link("Установщик x64", "x64 installer", "نصب کننده x64", "https://github.com/coolcoala/clash-verge-rev-lite/releases/latest/download/Koala.Clash_x64-setup.exe"),
          link("Версия ARM", "ARM version", "نسخه ARM", "https://github.com/coolcoala/clash-verge-rev-lite/releases/latest/download/Koala.Clash_arm64-setup.exe"),
        ],
      },
      {
        id: "prizrak-box",
        name: "Prizrak-Box",
        featured: true,
        scheme: "prizrak-box://install-config?url=",
        links: [
          link("Установщик x64", "x64 installer", "نصب کننده x64", "https://github.com/legiz-ru/Prizrak-Box/releases/latest/download/windows-amd64.msi"),
          link("Портативная x64", "Portable x64", "نسخه قابل حمل x64", "https://github.com/legiz-ru/Prizrak-Box/releases/latest/download/windows-amd64.zip"),
          link("Версия ARM", "ARM version", "نسخه ARM", "https://github.com/legiz-ru/Prizrak-Box/releases/latest/download/windows-arm64.msi"),
          link("Портативная ARM", "Portable ARM", "نسخه قابل حمل ARM", "https://github.com/legiz-ru/Prizrak-Box/releases/latest/download/windows-arm64.zip"),
        ],
      },
      {
        id: "happ",
        name: "Happ",
        scheme: "happ://add/",
        links: [link("Скачать для Windows", "Download for Windows", "دانلود برای Windows", "https://github.com/Happ-proxy/happ-desktop/releases/latest/download/setup-Happ.x64.exe")],
      },
      {
        id: "clash-verge",
        name: "Clash Verge",
        scheme: "clash://install-config?url=",
        links: [
          link("Установщик x64", "x64 installer", "نصب کننده x64", "https://github.com/clash-verge-rev/clash-verge-rev/releases/download/v2.4.4-rc/Clash.Verge_2.4.4-rc_x64-setup.exe"),
          link("Версия ARM", "ARM version", "نسخه ARM", "https://github.com/clash-verge-rev/clash-verge-rev/releases/download/v2.4.4-rc/Clash.Verge_2.4.4-rc_arm64-setup.exe"),
        ],
      },
    ],
  },
  {
    id: "linux",
    name: label("Linux", "Linux", "Linux"),
    icon: "linux",
    apps: [
      {
        id: "flclashx",
        name: "FlClashX",
        featured: true,
        scheme: "flclashx://install-config?url=",
        links: [
          link("amd64 (.deb)", "amd64 (.deb)", "amd64 (.deb)", "https://github.com/pluralplay/FlClashX/releases/download/v0.2.1/FlClashX-0.2.1-linux-amd64.deb"),
          link("amd64 (AppImage)", "amd64 (AppImage)", "amd64 (AppImage)", "https://github.com/pluralplay/FlClashX/releases/download/v0.2.1/FlClashX-0.2.1-linux-amd64.AppImage"),
          link("amd64 (.rpm)", "amd64 (.rpm)", "amd64 (.rpm)", "https://github.com/pluralplay/FlClashX/releases/download/v0.2.1/FlClashX-0.2.1-linux-amd64.rpm"),
          link("arm64 (.deb)", "arm64 (.deb)", "arm64 (.deb)", "https://github.com/pluralplay/FlClashX/releases/download/v0.2.1/FlClashX-0.2.1-linux-arm64.deb"),
        ],
      },
      {
        id: "koala-clash",
        name: "Koala Clash",
        featured: true,
        scheme: "koala-clash://install-config?url=",
        links: [
          link("amd64 (.deb)", "amd64 (.deb)", "amd64 (.deb)", "https://github.com/coolcoala/clash-verge-rev-lite/releases/latest/download/Koala.Clash_amd64.deb"),
          link("amd64 (.rpm)", "amd64 (.rpm)", "amd64 (.rpm)", "https://github.com/coolcoala/clash-verge-rev-lite/releases/latest/download/Koala.Clash.x86_64.rpm"),
          link("arm64 (.deb)", "arm64 (.deb)", "arm64 (.deb)", "https://github.com/coolcoala/clash-verge-rev-lite/releases/latest/download/Koala.Clash_arm64.deb"),
          link("arm64 (.rpm)", "arm64 (.rpm)", "arm64 (.rpm)", "https://github.com/coolcoala/clash-verge-rev-lite/releases/latest/download/Koala.Clash.aarch64.rpm"),
        ],
      },
      {
        id: "prizrak-box",
        name: "Prizrak-Box",
        featured: true,
        scheme: "prizrak-box://install-config?url=",
        links: [
          link("amd64 (.deb)", "amd64 (.deb)", "amd64 (.deb)", "https://github.com/legiz-ru/Prizrak-Box/releases/latest/download/linux-amd64.deb"),
          link("amd64 (.rpm)", "amd64 (.rpm)", "amd64 (.rpm)", "https://github.com/legiz-ru/Prizrak-Box/releases/latest/download/linux-amd64.rpm"),
          link("arm64 (.deb)", "arm64 (.deb)", "arm64 (.deb)", "https://github.com/legiz-ru/Prizrak-Box/releases/latest/download/linux-arm64.deb"),
          link("arm64 (.rpm)", "arm64 (.rpm)", "arm64 (.rpm)", "https://github.com/legiz-ru/Prizrak-Box/releases/latest/download/linux-arm64.rpm"),
        ],
      },
      {
        id: "clash-verge",
        name: "Clash Verge",
        scheme: "clash://install-config?url=",
        links: [
          link("amd64 (.deb)", "amd64 (.deb)", "amd64 (.deb)", "https://github.com/clash-verge-rev/clash-verge-rev/releases/download/v2.4.4-rc/Clash.Verge_2.4.4-rc_amd64.deb"),
          link("amd64 (.rpm)", "amd64 (.rpm)", "amd64 (.rpm)", "https://github.com/clash-verge-rev/clash-verge-rev/releases/download/v2.4.4-rc/Clash.Verge-2.4.4-rc-1.x86_64.rpm"),
          link("arm64 (.deb)", "arm64 (.deb)", "arm64 (.deb)", "https://github.com/clash-verge-rev/clash-verge-rev/releases/download/v2.4.4-rc/Clash.Verge_2.4.4-rc_arm64.deb"),
          link("arm64 (.rpm)", "arm64 (.rpm)", "arm64 (.rpm)", "https://github.com/clash-verge-rev/clash-verge-rev/releases/download/v2.4.4-rc/Clash.Verge-2.4.4-rc-1.aarch64.rpm"),
        ],
      },
    ],
  },
  {
    id: "android-tv",
    name: label("Android TV", "Android TV", "Android TV"),
    icon: "tv",
    apps: [
      {
        id: "happ",
        name: "Happ",
        featured: true,
        scheme: "happ://add/",
        links: [
          link("Открыть в Google Play", "Open in Google Play", "باز کردن در Google Play", "https://play.google.com/store/apps/details?id=com.happproxy"),
          link("Скачать APK", "Download APK", "دانلود APK", "https://github.com/Happ-proxy/happ-android/releases/latest/download/Happ.apk"),
        ],
      },
      {
        id: "flclashx",
        name: "FlClashX",
        featured: true,
        scheme: "flclashx://install-config?url=",
        links: [
          link("APK (ARMv8)", "APK (ARMv8)", "APK (ARMv8)", "https://github.com/pluralplay/FlClashX/releases/download/v0.2.1/FlClashX-0.2.1-android-arm64-v8a.apk"),
          link("APK (ARMv7)", "APK (ARMv7)", "APK (ARMv7)", "https://github.com/pluralplay/FlClashX/releases/download/v0.2.1/FlClashX-0.2.1-android-armeabi-v7a.apk"),
          link("APK (x86_64)", "APK (x86_64)", "APK (x86_64)", "https://github.com/pluralplay/FlClashX/releases/download/v0.2.1/FlClashX-0.2.1-android-x86_64.apk"),
        ],
      },
      {
        id: "vpn4tv",
        name: "vpn4tv",
        scheme: "hiddify://import/",
        links: [
          link("Открыть в Google Play", "Open in Google Play", "باز کردن در Google Play", "https://play.google.com/store/apps/details?id=com.vpn4tv.hiddify"),
          link("Скачать APK", "Download APK", "دانلود APK", "https://vpn4tv.com/download/vpn4tv.apk"),
        ],
      },
    ],
  },
  {
    id: "apple-tv",
    name: label("Apple TV", "Apple TV", "Apple TV"),
    icon: "tv",
    apps: [
      {
        id: "happ",
        name: "Happ",
        featured: true,
        scheme: "happ://add/",
        links: [link("Открыть в App Store", "Open in App Store", "باز کردن در App Store", "https://apps.apple.com/us/app/happ-proxy-utility-for-tv/id6748297274")],
      },
      {
        id: "shadowrocket",
        name: "Shadowrocket",
        scheme: "sub://",
        base64: true,
        links: [link("Открыть в App Store", "Open in App Store", "باز کردن در App Store", "https://apps.apple.com/ru/app/shadowrocket/id932747118")],
      },
      {
        id: "stash",
        name: "Stash",
        scheme: "stash://install-config?url=",
        links: [link("Открыть в App Store", "Open in App Store", "باز کردن در App Store", "https://apps.apple.com/us/app/stash-rule-based-proxy/id1596063349")],
      },
    ],
  },
]);

export function getSetupPlatform(platformID) {
  return SETUP_PLATFORMS.find((platform) => platform.id === platformID) || SETUP_PLATFORMS[0];
}

export function getSetupApp(platformID, appID) {
  const platform = getSetupPlatform(platformID);
  return platform.apps.find((app) => app.id === appID) || platform.apps.find((app) => app.featured) || platform.apps[0];
}

export function getLocalizedSetupValue(value, locale = "ru") {
  if (!value || typeof value !== "object") return String(value || "");
  return String(value[locale] || value.ru || value.en || "");
}

export function detectSetupPlatform(userAgent = "", telegramPlatform = "") {
  const ua = String(userAgent || "").toLowerCase();
  const tg = String(telegramPlatform || "").toLowerCase();
  if (/appletv|apple tv/.test(ua)) return "apple-tv";
  if (/android/.test(ua) && /tv|aft|bravia|smart-tv|googletv/.test(ua)) return "android-tv";
  if (/iphone|ipad|ipod/.test(ua) || tg === "ios") return "ios";
  if (/android/.test(ua) || tg === "android") return "android";
  if (/macintosh|mac os x/.test(ua) || tg === "macos") return "macos";
  if (/linux|x11/.test(ua)) return "linux";
  if (/windows|win64|win32/.test(ua) || tg === "tdesktop") return "windows";
  return "windows";
}

function toBase64(value) {
  const bytes = new TextEncoder().encode(String(value || ""));
  let binary = "";
  bytes.forEach((byte) => { binary += String.fromCharCode(byte); });
  return btoa(binary);
}

export function buildSetupClientURL(platformID, appID, subscriptionURL) {
  const parsed = new URL(String(subscriptionURL || ""));
  if (!/^https?:$/.test(parsed.protocol)) throw new Error("Invalid subscription URL");
  const app = getSetupApp(platformID, appID);
  const payload = app.base64 ? toBase64(parsed.toString()) : parsed.toString();
  return `${app.scheme}${payload}`;
}

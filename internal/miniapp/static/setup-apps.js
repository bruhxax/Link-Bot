const label = (ru, en, fa) => Object.freeze({ ru, en, fa });
const link = (ru, en, fa, url) => Object.freeze({ label: label(ru, en, fa), url });

const happ = (links) => Object.freeze({
  id: "happ",
  name: "Happ",
  featured: true,
  scheme: "happ://add/",
  links,
});

const incy = (links) => Object.freeze({
  id: "incy",
  name: "INCY",
  scheme: "incy://import/",
  links,
});

const happAppleLinks = Object.freeze([
  link("App Store (RU)", "App Store (RU)", "App Store (RU)", "https://apps.apple.com/ru/app/happ-proxy-utility-plus/id6746188973"),
  link("App Store (Global)", "App Store (Global)", "App Store (Global)", "https://apps.apple.com/us/app/happ-proxy-utility/id6504287215"),
]);

const happAndroidLinks = Object.freeze([
  link("Google Play", "Google Play", "Google Play", "https://play.google.com/store/apps/details?id=com.happproxy"),
  link("Скачать APK", "Download APK", "دانلود APK", "https://github.com/Happ-proxy/happ-android/releases/latest/download/Happ.apk"),
]);

const incyAppleLinks = Object.freeze([
  link("Открыть в App Store", "Open in App Store", "باز کردن در App Store", "https://apps.apple.com/ru/app/incy/id6756943388"),
]);

const incyAndroidLinks = Object.freeze([
  link("Google Play", "Google Play", "Google Play", "https://play.google.com/store/apps/details?id=llc.itdev.incy"),
  link("Скачать APK", "Download APK", "دانلود APK", "https://github.com/INCY-DEV/incy-platforms/releases/latest/download/Incy.apk"),
]);

export const SETUP_PLATFORMS = Object.freeze([
  {
    id: "ios",
    name: label("iOS", "iOS", "iOS"),
    icon: "apple",
    apps: [happ(happAppleLinks), incy(incyAppleLinks)],
  },
  {
    id: "android",
    name: label("Android", "Android", "Android"),
    icon: "android",
    apps: [happ(happAndroidLinks), incy(incyAndroidLinks)],
  },
  {
    id: "macos",
    name: label("macOS", "macOS", "macOS"),
    icon: "apple",
    apps: [
      happ(happAppleLinks),
      incy(Object.freeze([
        link("Apple Silicon", "Apple Silicon", "Apple Silicon", "https://github.com/INCY-DEV/incy-platforms/releases/latest/download/incy-macos-arm64.dmg"),
        link("Intel", "Intel", "Intel", "https://github.com/INCY-DEV/incy-platforms/releases/latest/download/incy-macos-intel.dmg"),
        ...incyAppleLinks,
      ])),
    ],
  },
  {
    id: "windows",
    name: label("Windows", "Windows", "Windows"),
    icon: "windows",
    apps: [
      happ(Object.freeze([
        link("Скачать Happ", "Download Happ", "دانلود Happ", "https://github.com/Happ-proxy/happ-desktop/releases/latest/download/setup-Happ.x64.exe"),
      ])),
      incy(Object.freeze([
        link("Установщик", "Installer", "نصب کننده", "https://github.com/INCY-DEV/incy-platforms/releases/latest/download/incy-windows-setup.exe"),
        link("Портативная версия", "Portable", "نسخه قابل حمل", "https://github.com/INCY-DEV/incy-platforms/releases/latest/download/incy-windows-portable.zip"),
      ])),
    ],
  },
  {
    id: "android-tv",
    name: label("Android TV", "Android TV", "Android TV"),
    icon: "tv",
    apps: [happ(happAndroidLinks), incy(incyAndroidLinks)],
  },
  {
    id: "apple-tv",
    name: label("Apple TV", "Apple TV", "Apple TV"),
    icon: "tv",
    apps: [
      happ(Object.freeze([
        link("Открыть в App Store", "Open in App Store", "باز کردن در App Store", "https://apps.apple.com/us/app/happ-proxy-utility-for-tv/id6748297274"),
      ])),
      incy(incyAppleLinks),
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
  if (/windows|win64|win32/.test(ua) || tg === "tdesktop") return "windows";
  return "windows";
}

export function buildSetupClientURL(platformID, appID, subscriptionURL) {
  const parsed = new URL(String(subscriptionURL || ""));
  if (!/^https?:$/.test(parsed.protocol)) throw new Error("Invalid subscription URL");
  const app = getSetupApp(platformID, appID);
  return `${app.scheme}${parsed.toString()}`;
}

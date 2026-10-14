export function validDeviceTrafficRules(rules) {
  if (!Array.isArray(rules) || !rules.length || rules.length > 100) return false;
  const devices = new Set();
  return rules.every((rule) => {
    if (!Number.isInteger(rule?.devices) || rule.devices < 1 || rule.devices > 1000 ||
        !Number.isInteger(rule?.trafficGb) || rule.trafficGb < 1 || rule.trafficGb > 1000000 || devices.has(rule.devices)) return false;
    devices.add(rule.devices);
    return true;
  });
}

export function sortedDeviceTrafficRules(rules) {
  if (!validDeviceTrafficRules(rules)) return [];
  return rules.map((rule) => ({ devices: rule.devices, trafficGb: rule.trafficGb })).sort((a, b) => a.devices - b.devices);
}

export function deviceTrafficLabels(rules, locale = "ru", enforced = false) {
  return sortedDeviceTrafficRules(rules).map(({devices, trafficGb}) => {
    const number = new Intl.NumberFormat(locale).format(devices);
    const traffic = new Intl.NumberFormat(locale).format(trafficGb);
    if (enforced) return locale === "ru" ? `Устройство №${number} — ${traffic} GB` : locale === "fa" ? `دستگاه ${number} — ${traffic} گیگابایت` : `Device #${number} — ${traffic} GB`;
    if (locale === "en") return `${number} ${devices === 1 ? "device" : "devices"} — ${traffic} GB`;
    if (locale === "fa") return `${number} دستگاه — ${traffic} گیگابایت`;
    const word = devices % 10 === 1 && devices % 100 !== 11 ? "устройство"
      : devices % 10 >= 2 && devices % 10 <= 4 && (devices % 100 < 12 || devices % 100 > 14) ? "устройства" : "устройств";
    return `${number} ${word} — ${traffic} GB`;
  });
}

export function deviceTrafficProgress(device) {
  const used = device?.trafficUsedBytes;
  const limit = device?.trafficLimitBytes;
  if (used == null || limit == null || !Number.isFinite(used) || !Number.isFinite(limit) || used < 0 || limit < 0) {
    return device?.slot > 0 ? { unknown: true } : null;
  }
  return { used, limit, percent: limit > 0 ? Math.min(100, used / limit * 100) : 0, exceeded: limit > 0 && used >= limit };
}

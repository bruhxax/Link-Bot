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

export function deviceTrafficLabels(rules, locale = "ru") {
  return sortedDeviceTrafficRules(rules).map(({devices, trafficGb}) => {
    const number = new Intl.NumberFormat(locale).format(devices);
    const traffic = new Intl.NumberFormat(locale).format(trafficGb);
    if (locale === "en") return `${number} ${devices === 1 ? "device" : "devices"} — ${traffic} GB`;
    if (locale === "fa") return `${number} دستگاه — ${traffic} گیگابایت`;
    const word = devices % 10 === 1 && devices % 100 !== 11 ? "устройство"
      : devices % 10 >= 2 && devices % 10 <= 4 && (devices % 100 < 12 || devices % 100 > 14) ? "устройства" : "устройств";
    return `${number} ${word} — ${traffic} GB`;
  });
}

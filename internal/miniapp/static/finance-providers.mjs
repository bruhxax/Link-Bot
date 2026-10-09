export function financeProviderKey(value) {
  const key = String(value || "")
    .trim()
    .toLowerCase();
  switch (key) {
    case "yookasa":
      return "yookassa";
    case "tribute_shop":
      return "tribute";
    case "cryptopay":
      return "crypto";
    default:
      return key;
  }
}

// Invoice names and integration names can differ. Merge their settled totals
// before adding empty catalog cards, and never add different currencies together.
export function buildFinanceProviders(items, catalog) {
  const merged = new Map();
  const amount = (value) =>
    Number.isFinite(Number(value)) ? Number(value) : 0;
  for (const item of Array.isArray(items) ? items : []) {
    const key = financeProviderKey(item.key);
    if (!key) continue;
    let provider = merged.get(key);
    if (!provider) {
      provider = {
        key,
        name: item.name || key,
        paymentCount: 0,
        totals: new Map(),
      };
      merged.set(key, provider);
    }
    const currency =
      String(item.currency || "")
        .trim()
        .toUpperCase() || (key === "telegram" ? "STARS" : "RUB");
    const total = provider.totals.get(currency) || {
      currency,
      revenue: 0,
      refunds: 0,
    };
    total.revenue += amount(item.revenue);
    total.refunds += amount(item.refunds);
    provider.paymentCount += amount(item.paymentCount);
    provider.totals.set(currency, total);
  }
  const providers = catalog.map(([key, name, logo]) => {
    const provider = merged.get(key);
    merged.delete(key);
    return {
      key,
      name,
      logo,
      paymentCount: provider?.paymentCount || 0,
      totals: provider
        ? [...provider.totals.values()]
        : [
            {
              currency: key === "telegram" ? "STARS" : "RUB",
              revenue: 0,
              refunds: 0,
            },
          ],
    };
  });
  for (const provider of merged.values())
    providers.push({ ...provider, totals: [...provider.totals.values()] });
  return providers.sort((a, b) => b.paymentCount - a.paymentCount);
}

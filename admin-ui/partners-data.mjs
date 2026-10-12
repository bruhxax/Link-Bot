export const applicationStatuses = [
  { value: "pending", label: "Ожидает решения" },
  { value: "approved", label: "Принята" },
  { value: "rejected", label: "Отклонена" },
];
export function partnerRows(
  items,
  { query = "", status = "", applications = false, descending = true } = {},
) {
  const needle = query.trim().toLowerCase();
  return items
    .filter(
      (item) =>
        (!status ||
          (applications
            ? item.status
            : item.isActive
              ? "active"
              : "disabled") === status) &&
        (!needle ||
          [item.username, item.telegramId, item.code].some((value) =>
            String(value || "")
              .toLowerCase()
              .includes(needle),
          )),
    )
    .sort((a, b) =>
      descending
        ? String(b.createdAt || "").localeCompare(String(a.createdAt || "")) ||
          b.id - a.id
        : String(a.createdAt || "").localeCompare(String(b.createdAt || "")) ||
          a.id - b.id,
    );
}
export function partnerMetrics(data) {
  const all = data?.partners || [],
    requests = data?.applications || [];
  const partners = partnerRows(all),
    applications = partnerRows(requests, { applications: true });
  return {
    total: partners.length,
    active: partners.filter((p) => p.isActive).length,
    applications: applications.length,
    totalShare: all.length ? (partners.length / all.length) * 100 : 0,
    activeShare: partners.length
      ? (partners.filter((p) => p.isActive).length / partners.length) * 100
      : 0,
    applicationsShare: requests.length
      ? (applications.length / requests.length) * 100
      : 0,
  };
}

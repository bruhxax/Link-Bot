import { userDateKey, userInDateRange } from "./users-list-data.mjs";

export const applicationStatuses = [
  { value: "pending", label: "Ожидает решения" },
  { value: "approved", label: "Принята" },
  { value: "rejected", label: "Отклонена" },
];
export function partnersMonthRange(now = new Date()) {
  const month = userDateKey(now.toISOString()).slice(0, 7);
  const [year, index] = month.split("-").map(Number);
  return [
    month + "-01",
    new Date(Date.UTC(year, index, 0, 12)).toISOString().slice(0, 10),
  ];
}
export function partnerRows(
  items,
  {
    query = "",
    status = "",
    range = [null, null],
    applications = false,
    descending = true,
  } = {},
) {
  const needle = query.trim().toLowerCase();
  return items
    .filter(
      (item) =>
        userInDateRange(item.createdAt, range) &&
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
export function partnerMetrics(data, range) {
  const all = data?.partners || [],
    requests = data?.applications || [];
  const partners = partnerRows(all, { range }),
    applications = partnerRows(requests, { range, applications: true });
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

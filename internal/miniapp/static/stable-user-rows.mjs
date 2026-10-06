// Preserve mounted rows and avatars during unrelated realtime updates.
export function reuseAdminUserRows(current, next) {
  const rows = current.querySelectorAll(".admin-users__list > .admin-user-row");
  const key = row => `${row.dataset.action}:${row.dataset.value}`;
  const previous = new Map([...rows].map(row => [key(row), row]));
  for (const row of next.querySelectorAll(".admin-users__list > .admin-user-row")) {
    const old = previous.get(key(row));
    if (!old) continue;
    if (old.outerHTML === row.outerHTML) row.replaceWith(old);
    else {
      const avatar = row.querySelector(".admin-user-avatar");
      const oldAvatar = old.querySelector(".admin-user-avatar");
      if (avatar && oldAvatar && avatar.outerHTML === oldAvatar.outerHTML) avatar.replaceWith(oldAvatar);
    }
  }
}

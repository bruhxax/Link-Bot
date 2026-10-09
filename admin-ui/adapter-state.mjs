// Reuse unchanged table data and converted DOM between background refreshes.
// Content comes from the authenticated cabinet's existing draft/action layer.
export function snapshot(value, previous) {
  const signature = JSON.stringify(value);
  return previous?.signature === signature ? previous : { signature, value };
}

export function controlKey(node, fallback) {
  return (
    node.dataset?.settingPath ||
    node.id ||
    (node.dataset?.input
      ? `${node.dataset.input}:${["checkbox", "radio"].includes(node.type) ? node.value : ""}`
      : fallback)
  );
}

export function selectedTab(tabs) {
  return (
    tabs.find(
      (tab) =>
        tab.getAttribute("aria-selected") === "true" ||
        tab.getAttribute("aria-pressed") === "true" ||
        tab.classList.contains("active") ||
        tab.classList.contains("is-active"),
    ) || tabs[0]
  );
}

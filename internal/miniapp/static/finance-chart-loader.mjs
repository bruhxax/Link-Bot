// Load chart code only on the cabinet finance page. Invalidate a pending mount
// when navigation or a refresh replaces its DOM while the bundle is downloading.
export function createFinanceChartLoader(
  load = () => import("./finance-ui.mjs"),
) {
  let charts;
  let pending;
  let generation = 0;
  return {
    async mountFinanceCharts(container) {
      if (!container.querySelector(".premium-finance-chart")) return;
      const current = ++generation;
      try {
        charts ||= await (pending ||= load().catch((error) => {
          pending = null;
          throw error;
        }));
        if (current === generation && container.isConnected)
          charts.mountFinanceCharts(container);
      } catch {
        if (current !== generation) return;
        const status = container.querySelector(".rn-chart-loading");
        if (status)
          status.textContent =
            "Не удалось загрузить график. Обновите страницу.";
      }
    },
    unmountFinanceCharts() {
      generation++;
      charts?.unmountFinanceCharts();
    },
  };
}

export const { mountFinanceCharts, unmountFinanceCharts } =
  createFinanceChartLoader();

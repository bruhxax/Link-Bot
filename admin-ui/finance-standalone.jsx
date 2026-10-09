import React from "react";
import { createRoot } from "react-dom/client";
import { flushSync } from "react-dom";
import { FinanceChart } from "./finance-chart.jsx";

const roots = new Map();

export function mountFinanceCharts(container) {
  for (const host of container.querySelectorAll(
    ".premium-finance-chart[data-finance-series]",
  )) {
    if (roots.has(host)) continue;
    const series = JSON.parse(host.dataset.financeSeries);
    const root = createRoot(host);
    roots.set(host, root);
    flushSync(() => root.render(<FinanceChart series={series} />));
  }
}

export function unmountFinanceCharts() {
  for (const root of roots.values()) flushSync(() => root.unmount());
  roots.clear();
}

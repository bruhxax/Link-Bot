// Only navigation calls this: background observations never replay the entrance.
export function animateAdminEntrance(root) {
  if (!root || matchMedia("(prefers-reduced-motion: reduce)").matches) return;
  const selectors =
    ".rn-page-header,.rn-section-toolbar,.rn-user-metric,.rn-users-list-title,.rn-users-toolbar,.rn-users-grid,.rn-users-footer,.rn-partners-period,.rn-partner-metric,.rn-partner-card,.rn-overview-distributions > section,.rn-overview-income > h2,.rn-overview-income-card,.rn-overview-graphs > .rn-overview-section-head,.rn-overview-charts > section,.rn-overview-error";
  const candidates = [
    ...root.querySelectorAll(
      selectors +
        ",.rn-content > section,.rn-content .admin-setting-group,.rn-content .admin-card,.rn-content article",
    ),
  ];
  const elements = candidates.filter(
    (element) =>
      !candidates.some(
        (parent) => parent !== element && parent.contains(element),
      ),
  );
  const ordered = elements
    .map((element) => ({ element, rect: element.getBoundingClientRect() }))
    .filter(({ rect }) => rect.width && rect.height)
    .sort((a, b) =>
      Math.abs(a.rect.top - b.rect.top) > 16
        ? a.rect.top - b.rect.top
        : a.rect.left - b.rect.left,
    );
  const animations = ordered.map(({ element }, index) =>
    element.animate(
      [
        { opacity: 0, transform: "translate(-4px, 8px)" },
        { opacity: 1, transform: "translate(0, 0)" },
      ],
      {
        duration: 320,
        delay: Math.min(index * 40, 440),
        easing: "cubic-bezier(.22,1,.36,1)",
        fill: "backwards",
      },
    ),
  );
  return () => animations.forEach((animation) => animation.cancel());
}

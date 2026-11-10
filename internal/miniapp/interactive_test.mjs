import test from "node:test";
import assert from "node:assert/strict";
import {
  defaultInteractive,
  renderInteractiveEditor,
} from "./static/interactive-editor.mjs";
const escape = (s) =>
  String(s).replace(
    /[&<>"']/g,
    (c) =>
      ({ "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#39;" })[
        c
      ],
  );
test("interactive editor escapes saved texts, keeps editable settings safe inside one collapsed control", () => {
  const cfg = defaultInteractive();
  cfg.addresses = ["203.0.113.1", "2001:db8::1"];
  cfg.protectedText = "<img src=x onerror=alert(1)> {brand}";
  const html = renderInteractiveEditor(
    cfg,
    { escapeHtml: escape, escapeAttribute: escape, icon: () => "" },
    { brand: "<BruhVPN>" },
  );
  assert.doesNotMatch(html, /<img/);
  assert.match(html, /&lt;img/);
  assert.match(html, /interactive-import/);
  assert.match(html, /2001:db8::1/);
  assert.match(html, /aria-expanded="false"/);
  assert.match(html, /data-interactive-fields disabled/);
  assert.doesNotMatch(
    html,
    /Предпросмотр|interactive-card|interactive-preview|Плашка под меню|Импорт добавит/,
  );
  assert.match(html, /Статус защиты/);
});
test("interactive defaults are disabled and permanent display keeps the seconds field hidden", () => {
  const cfg = defaultInteractive();
  assert.equal(cfg.enabled, false);
  cfg.durationSeconds = 0;
  const html = renderInteractiveEditor(cfg, {
    escapeHtml: escape,
    escapeAttribute: escape,
    icon: () => "",
  });
  assert.match(html, /data-interactive-field="always" checked/);
  assert.match(html, /data-interactive-duration hidden/);
  assert.match(html, /Показывать всегда/);
  const open = renderInteractiveEditor(
    cfg,
    { escapeHtml: escape, escapeAttribute: escape, icon: () => "" },
    { open: true },
  );
  assert.match(open, /aria-expanded="true"/);
  assert.doesNotMatch(open, /data-interactive-fields disabled/);
});

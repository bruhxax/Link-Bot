import test from "node:test";
import assert from "node:assert/strict";
import {
  defaultInteractive,
  interactiveText,
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
test("interactive editor escapes saved texts, provides list/import and both preview states", () => {
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
  assert.match(html, /data-value="unprotected"/);
  assert.equal(
    interactiveText(cfg, true, "VPN"),
    "\x3cimg src=x onerror=alert(1)> VPN",
  );
  assert.equal(interactiveText(cfg, false, "VPN"), "Вы не под защитой");
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
  assert.match(html, /Остаётся на странице/);
});

import test from "node:test";
import assert from "node:assert/strict";
import { canAdmin, changePermission, presetPermissions, roleDot, renderAdministrators } from "./static/administrators.mjs";

const catalog = [
  { id: "status", label: "Статус", group: "Система" },
  { id: "users", label: "Пользователи", group: "Операции" },
  { id: "users.balance", label: "Баланс", group: "Действия", requires: ["users"] },
  { id: "support.view", label: "Обращения", group: "Поддержка" },
  { id: "support.reply", label: "Ответы", group: "Поддержка", requires: ["support.view"] },
];
const helpers = {
  escapeHtml: (s) => String(s).replaceAll("&", "&amp;").replaceAll("<", "&lt;").replaceAll('"', "&quot;"),
  escapeAttribute: (s) => String(s).replaceAll('"', "&quot;"),
  icon: (name) => `<svg data-icon="${name}"></svg>`, avatar: () => '<i class="avatar"></i>', displayName: (u) => u.username,
  loading: () => '<div class="loading"></div>',
};

test("only the owner can manage administrators, even with an injected permission", () => {
  assert.equal(canAdmin({ isAdmin: true, permissions: ["status", "administrators"] }, "administrators"), false);
  assert.equal(canAdmin({ isAdmin: true, permissions: ["status"] }, "users"), false);
  assert.equal(canAdmin({ isAdmin: true, permissions: ["status"] }, "status"), true);
  assert.equal(canAdmin({ isAdmin: true, isOwner: true }, "administrators"), true);
  assert.equal(canAdmin({ permissions: ["status"] }, "status"), false);
});

test("enabling an action selects its section and removing the section revokes actions", () => {
  assert.deepEqual(changePermission([], "users.balance", true, catalog), ["users", "users.balance"]);
  assert.deepEqual(changePermission(["status", "users", "users.balance"], "users", false, catalog), ["status"]);
  assert.deepEqual(changePermission([], "support.reply", true, catalog), ["support.view", "support.reply"]);
  assert.deepEqual(changePermission([], "unknown", true, catalog), []);
});

test("presets grant explicit permissions, without administrator assignment rights", () => {
  assert.deepEqual(presetPermissions("admin", catalog), catalog.map((p) => p.id));
  assert.deepEqual(presetPermissions("support", catalog), ["users", "support.view", "support.reply"]);
  assert.deepEqual(presetPermissions("observer", catalog), ["status", "users"]);
});

test("role dots reject CSS injection and escape the role label", () => {
  assert.equal(roleDot("red;background:url(x)", "Роль", helpers), "");
  assert.match(roleDot("#55c58a", 'Роль "1"', helpers), /&quot;1&quot;/);
});

test("role editor displays all permission groups, editable name and removal confirmation", () => {
  const state = { catalog, editor: { customerId: 2, telegramId: 22, username: "<script>", role: "Поддержка", color: "#55c58a", permissions: ["support.view"], isNew: false }, confirmRemove: true };
  const output = renderAdministrators(state, helpers);
  for (const p of catalog) assert.ok(output.includes(`data-value="${p.id}"`));
  assert.ok(output.includes("&lt;script>"));
  assert.ok(output.includes('data-action="administrators-remove"'));
  assert.ok(output.includes('data-input="administrators-role"'));
});

test("owner row cannot be saved, recolored or removed", () => {
  const output = renderAdministrators({ catalog, editor: { customerId: 1, telegramId: 1, username: "owner", role: "Главный администратор", color: "#69a8d4", permissions: [], isOwner: true } }, helpers);
  for (const action of ["administrators-save", "administrators-remove", "administrators-color"]) assert.ok(!output.includes(`data-action="${action}"`));
  assert.equal((output.match(/checked disabled/g) || []).length, catalog.length);
});

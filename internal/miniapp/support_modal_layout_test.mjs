import assert from "node:assert/strict";
import fs from "node:fs";
import vm from "node:vm";
import test from "node:test";

const source = fs.readFileSync(new URL("./static/app.js", import.meta.url), "utf8");
function section(start, end) {
  return source.slice(source.indexOf(start), source.indexOf(end, source.indexOf(start)));
}

test("loading a historical ticket keeps its title, metadata and closed footer", () => {
  const ticket = { id: 95, subject: "Не работает VPN", status: "closed", customerName: "test", customerUsername: "test", subscriptionLabel: "Годовой" };
  const state = { data: { support: { isAdmin: true, historyTickets: [ticket] } }, activeSupportTicketId: 95, activeSupportThread: null };
  const context = vm.createContext({ state, supportText: () => ({ loadingThread: "Загрузка", customer: "Пользователь", subscription: "Подписка", closed: "Закрыто", closedHint: "История" }),
    localizedText: ru => ru, modalStateClass: () => "", escapeHtml: String, escapeAttribute: String,
    supportTicketTitle: ticket => ticket.subject, formatTelegramUsername: value => `@${value}`,
    renderSupportMessage: () => "message", renderSupportPendingMedia: () => "", icon: () => "",
  });
  vm.runInContext(section("function renderSupportThreadModal()", "function formatSupportMediaSize("), context);
  const loading = context.renderSupportThreadModal();
  state.activeSupportThread = { ticket, canReply: false, canClose: false, messages: [{ id: 1 }] };
  const ready = context.renderSupportThreadModal();
  const header = html => html.slice(html.indexOf('<div class="modal__header'), html.indexOf('<div class="support-thread__messages'));
  assert.equal(header(loading), header(ready));
  assert.match(loading, /aria-busy="true"/);
  assert.match(ready, /aria-busy="false"/);
  assert.match(loading, /support-thread__closed/);
  assert.doesNotMatch(loading, /support-reply__textarea/);
});

test("the new ticket form contains a message field without a subject field", () => {
  const context = vm.createContext({ state: { supportDraftMessage: "Вопрос" },
    supportText: () => ({ createTitle: "Обращение", message: "Сообщение", messagePlaceholder: "Вопрос", send: "Отправить" }),
    localizedText: ru => ru, modalStateClass: () => "", escapeHtml: String, escapeAttribute: String, icon: () => "",
  });
  vm.runInContext(section("function renderSupportComposerModal()", "function renderSupportThreadModal()"), context);
  const html = context.renderSupportComposerModal();
  assert.match(html, /data-input="support-message"/);
  assert.doesNotMatch(html, /support-subject|<input\b/);
});

function mountedModal() {
  const classes = new Set(["modal--animate"]);
  const sheet = { children: ["loading"], attributes: {}, setAttribute(name, value) { this.attributes[name] = value; }, replaceChildren(...children) { this.children = children; } };
  const current = { dataset: { supportModal: "thread-95" }, classList: {
    toggle(name, enabled) { if (enabled) classes.add(name); else classes.delete(name); }, remove(name) { classes.delete(name); },
  }, querySelector: () => sheet };
  const nextSheet = { childNodes: ["messages"], getAttribute: () => "false" };
  const next = { dataset: { supportModal: "thread-95" }, closing: false, classList: { contains: name => name === "modal--closing" && next.closing }, querySelector: () => nextSheet };
  let active = "support-thread";
  let overlay = null;
  const context = vm.createContext({ app: { querySelector: selector => selector === ".modal:not(.modal--support-chat)" ? overlay : current },
    document: { createElement: () => ({ content: { querySelector: () => next } }) }, getActiveModalName: () => active,
  });
  vm.runInContext(section("function updateMountedSupportModal(", "function mountCabinetShell("), context);
  return { context, current, sheet, classes, next, setActive(value) { active = value; }, setOverlay(value) { overlay = value; } };
}

test("receiving the conversation keeps the connected modal and entrance animation", () => {
  const page = mountedModal();
  assert.equal(page.context.updateMountedSupportModal("markup"), true);
  assert.equal(page.current.querySelector(), page.sheet);
  assert.deepEqual(page.sheet.children, ["messages"]);
  assert.equal(page.sheet.attributes["aria-busy"], "false");
  assert.equal(page.classes.has("modal--animate"), true);
  page.next.closing = true;
  page.context.updateMountedSupportModal("markup");
  assert.equal(page.classes.has("modal--closing"), true);
  assert.equal(page.classes.has("modal--animate"), false);
});

test("switching tickets or opening a media viewer does not reuse the wrong modal", () => {
  const page = mountedModal();
  page.next.dataset.supportModal = "thread-101";
  assert.equal(page.context.updateMountedSupportModal("markup"), false);
  assert.deepEqual(page.sheet.children, ["loading"]);
  page.next.dataset.supportModal = "thread-95";
  page.setActive("support-media-viewer");
  assert.equal(page.context.updateMountedSupportModal("markup"), false);
  page.setActive("support-thread");
  page.setOverlay({}); // The viewer must be removed when returning to the chat.
  assert.equal(page.context.updateMountedSupportModal("markup"), false);
});

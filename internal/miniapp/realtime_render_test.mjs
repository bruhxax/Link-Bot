import assert from "node:assert/strict";
import fs from "node:fs";
import vm from "node:vm";
import test from "node:test";

const source = fs.readFileSync(new URL("./static/app.js", import.meta.url), "utf8");
const start = source.indexOf("function patchRealtimeSupportMessages()");
const end = source.indexOf("function queueRealtimeRefresh(", start);
assert.ok(start >= 0 && end > start);

function harness({ modal = false, editing = false, recentInteraction = false, thread = false } = {}) {
  let renders = 0;
  let hydrated = 0;
  let timeout = null;
  const messages = { innerHTML: "old", scrollHeight: 100, scrollTop: 80, clientHeight: 20, querySelectorAll: () => [{ dataset: { messageId: "1" } }] };
  const app = {
    contains: () => editing,
    querySelector: (selector) => selector === ".app-shell" ? {} : selector === "#support-thread-messages" && thread ? messages : null,
  };
  const context = vm.createContext({
    app,
    state: { data: {}, maintenance: null, blocked: null, subscriptionGate: null, supportThreadOpen: thread, activeSupportThread: thread ? { messages: [{ id: 2, body: "new" }] } : null },
    document: { activeElement: editing ? { matches: () => true } : null, body: { classList: { contains: () => modal } } },
    window: { clearTimeout: () => {}, setTimeout: (callback) => { timeout = callback; return 1; } },
    renderSupportMessage: (message) => `<p>${message.body}</p>`,
    hydrateSupportMedia: () => { hydrated++; },
    render: () => { renders++; },
    realtimeBatching: false,
    realtimeBatchRenderRequested: false,
    realtimeRenderPending: false,
    realtimeRenderTimer: 0,
    realtimeLastInteraction: recentInteraction ? Date.now() : 0,
  });
  vm.runInContext(source.slice(start, end), context);
  return { context, messages, get renders() { return renders; }, get hydrated() { return hydrated; }, get timeout() { return timeout; } };
}

test("live update keeps an open chat mounted while adding messages", () => {
  const page = harness({ modal: true, thread: true });
  page.context.renderRealtime();
  assert.equal(page.renders, 0);
  assert.equal(page.messages.innerHTML, "<p>new</p>");
  assert.equal(page.hydrated, 1);
  assert.equal(page.context.realtimeRenderPending, true);
});

test("unchanged chat messages keep their existing DOM and media", () => {
  const page = harness({ modal: true, thread: true });
  page.messages.querySelectorAll = () => [{ dataset: { messageId: "2" } }];
  page.context.renderRealtime();
  assert.equal(page.messages.innerHTML, "old");
  assert.equal(page.hydrated, 0);
});

test("new chat message appends without replacing previous messages", () => {
  const page = harness({ modal: true, thread: true });
  page.context.state.activeSupportThread.messages = [{ id: 1, body: "old" }, { id: 2, body: "new" }];
  page.messages.insertAdjacentHTML = (_position, html) => { page.messages.innerHTML += html; };
  page.context.renderRealtime();
  assert.equal(page.messages.innerHTML, "old<p>new</p>");
  assert.equal(page.renders, 0);
});

test("live update waits for scrolling or editing to finish", () => {
  const page = harness({ recentInteraction: true });
  page.context.renderRealtime();
  assert.equal(page.renders, 0);
  assert.equal(typeof page.timeout, "function");
  page.context.realtimeLastInteraction = 0;
  page.timeout();
  assert.equal(page.renders, 1);

  const editing = harness({ editing: true });
  editing.context.renderRealtime();
  assert.equal(editing.renders, 0);
  assert.equal(editing.context.realtimeRenderPending, true);
});

test("background updates batch into one render", () => {
  const page = harness();
  page.context.realtimeBatching = true;
  page.context.renderRealtime();
  page.context.renderRealtime();
  assert.equal(page.renders, 0);
  assert.equal(page.context.realtimeBatchRenderRequested, true);
});

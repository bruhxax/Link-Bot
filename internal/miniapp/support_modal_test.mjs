import assert from "node:assert/strict";
import fs from "node:fs";
import vm from "node:vm";
import test from "node:test";

const source = fs.readFileSync(new URL("./static/app.js", import.meta.url), "utf8");
function section(start, end) {
  const first = source.indexOf(start);
  const last = source.indexOf(end, first);
  assert.ok(first >= 0 && last > first);
  return source.slice(first, last);
}

const code = section("let supportThreadVersion =", "function isSupportedSupportMedia(") +
  section("async function openSupportTicket(", "function syncSupportPolling(") +
  section("function requestModalClose(", "function closePayModal(");

function harness() {
  const requests = [];
  const timers = new Map();
  let timerID = 0;
  let renders = 0;
  const state = {
    data: {}, currentPage: "support", supportBusy: "", supportThreadOpen: true,
    activeSupportTicketId: 1, activeSupportThread: thread(1, "original"),
    supportReplyDraft: "", supportPendingMedia: null, supportComposeOpen: false,
    supportDraftSubject: "", supportDraftMessage: "",
  };
  const post = (path, body) => new Promise((resolve, reject) => requests.push({path, body, resolve, reject}));
  const context = vm.createContext({
    state, post, postForm: post, previewMode: false, closingModalName: "",
    animatedModalName: "", previousActiveModalName: "support-thread",
    closingModalTimer: 0, supportThreadPollTimer: 0, MODAL_CLOSE_MS: 180,
    reducedMotionMedia: { matches: false },
    window: {
      clearInterval() {},
      clearTimeout(id) { timers.delete(id); },
      setTimeout(fn) { timers.set(++timerID, fn); return timerID; },
    },
    render() { renders++; }, renderRealtime() { renders++; },
    clearPendingSupportMedia() { state.supportPendingMedia = null; },
    clearSupportMediaCache() {}, refreshSupport() {}, showToast() {},
    supportText: () => ({}), isAdminUser: () => false,
    FormData: class { append() {} },
  });
  vm.runInContext(code, context);
  return {state, context, requests, timers, get renders() { return renders; },
    close() { context.requestModalClose("support-thread", context.closeSupportThreadState); },
    finishAnimation() { const callbacks = [...timers.values()]; timers.clear(); callbacks.forEach(fn => fn()); },
  };
}

function thread(id, body = "new") {
  return {ticket: {id, status: "open"}, canReply: true, messages: [{id: 10, body}]};
}

test("a delayed poll never reopens a closed support modal", async () => {
  for (const duringAnimation of [true, false]) {
    const page = harness();
    const pending = page.context.openSupportTicket(1, {silent: true});
    page.close();
    if (!duringAnimation) page.finishAnimation();
    page.requests[0].resolve({data: thread(1, "late")});
    await pending;
    if (duringAnimation) {
      assert.equal(page.state.activeSupportThread.messages[0].body, "original");
      page.finishAnimation();
    }
    assert.equal(page.state.supportThreadOpen, false);
    assert.equal(page.state.activeSupportThread, null);
    assert.equal(page.state.activeSupportTicketId, 0);
  }
});

test("closing the loading modal invalidates its initial request", async () => {
  const page = harness();
  page.context.closeSupportThreadState();
  const pending = page.context.openSupportTicket(2);
  assert.equal(page.state.supportThreadOpen, true);
  assert.equal(page.state.activeSupportThread, null);
  page.close(); page.finishAnimation();
  page.requests[0].resolve({data: thread(2)});
  await pending;
  assert.equal(page.state.supportThreadOpen, false);
});

test("out-of-order ticket loads cannot replace a newer ticket", async () => {
  const page = harness();
  page.state.supportReplyDraft = "draft for ticket one";
  const old = page.context.openSupportTicket(1, {silent: true});
  const latest = page.context.openSupportTicket(2);
  page.requests[1].resolve({data: thread(2)}); await latest;
  page.requests[0].resolve({data: thread(1)}); await old;
  assert.equal(page.state.activeSupportTicketId, 2);
  assert.equal(page.state.activeSupportThread.ticket.id, 2);
  assert.equal(page.state.supportReplyDraft, "");
});

test("reopening the same ticket still discards the previous session's response", async () => {
  const page = harness();
  const old = page.context.openSupportTicket(1, {silent: true});
  page.close(); page.finishAnimation();
  const latest = page.context.openSupportTicket(1);
  page.requests[1].resolve({data: thread(1, "latest")}); await latest;
  page.requests[0].resolve({data: thread(1, "old")}); await old;
  assert.equal(page.state.activeSupportThread.messages[0].body, "latest");
});

test("background refreshes do not overlap or open a hidden modal", async () => {
  const page = harness();
  const pending = page.context.openSupportTicket(1, {silent: true});
  await page.context.openSupportTicket(1, {silent: true});
  assert.equal(page.requests.length, 1);
  page.requests[0].resolve({data: thread(1)}); await pending;
  page.context.closeSupportThreadState();
  await page.context.openSupportTicket(1, {silent: true});
  assert.equal(page.requests.length, 1);
});

test("a poll started before a send cannot erase the pending or saved message", async () => {
  const page = harness();
  const poll = page.context.openSupportTicket(1, {silent: true});
  page.state.supportReplyDraft = "my message";
  const send = page.context.sendSupportMessage();
  page.requests[0].resolve({data: thread(1, "old poll")}); await poll;
  assert.equal(page.state.activeSupportThread.messages.at(-1).body, "my message");
  page.requests[1].resolve({data: thread(1, "saved")}); await send;
  assert.equal(page.state.activeSupportThread.messages[0].body, "saved");
});

test("late send and close responses leave a dismissed chat closed", async () => {
  for (const method of ["sendSupportMessage", "closeSupportTicket"]) {
    for (const failed of [false, true]) {
      const page = harness();
      page.state.supportReplyDraft = "message";
      const pending = page.context[method]();
      page.close(); page.finishAnimation();
      if (failed) page.requests[0].reject(new Error("offline"));
      else page.requests[0].resolve({data: thread(1)});
      await pending;
      assert.equal(page.state.activeSupportThread, null);
      assert.equal(page.state.supportReplyDraft, "");
      assert.equal(page.state.supportBusy, "");
    }
  }
});

test("old failed sends cannot reset a new ticket's draft or busy operation", async () => {
  const page = harness();
  page.state.supportReplyDraft = "old draft";
  const old = page.context.sendSupportMessage();
  page.close(); page.finishAnimation();
  const opening = page.context.openSupportTicket(2);
  page.requests[1].resolve({data: thread(2)}); await opening;
  page.state.supportReplyDraft = "new draft";
  const latest = page.context.sendSupportMessage();
  page.requests[0].reject(new Error("offline")); await old;
  assert.equal(page.state.supportBusy, "send-support-message");
  assert.equal(page.state.activeSupportThread.ticket.id, 2);
  page.requests[2].resolve({data: thread(2, "saved")}); await latest;
  assert.equal(page.state.supportBusy, "");
});

test("late media uploads cannot clear an attachment in the new chat", async () => {
  const page = harness();
  const pending = page.context.sendSupportMediaMessage({name: "old.png"}, "old");
  page.close(); page.finishAnimation();
  const opening = page.context.openSupportTicket(2);
  page.requests[1].resolve({data: thread(2)}); await opening;
  page.state.supportPendingMedia = {name: "new.png"};
  page.requests[0].resolve({data: thread(1)}); await pending;
  assert.equal(page.state.supportPendingMedia.name, "new.png");
  assert.equal(page.state.activeSupportThread.ticket.id, 2);
});

test("closing and reopening the composer prevents a late create from opening a chat", async () => {
  const page = harness();
  page.context.closeSupportThreadState();
  page.context.openSupportComposer();
  page.state.supportDraftMessage = "old question";
  const pending = page.context.submitSupportTicket();
  page.context.requestModalClose("support-compose", page.context.closeSupportComposeState);
  page.finishAnimation();
  page.context.openSupportComposer();
  page.state.supportDraftMessage = "new question";
  page.requests[0].resolve({data: thread(3)}); await pending;
  assert.equal(page.state.supportComposeOpen, true);
  assert.equal(page.state.supportDraftMessage, "new question");
  assert.equal(page.state.supportThreadOpen, false);
});

test("creating a ticket requires only its message and sends no subject", async () => {
  const page = harness();
  page.context.closeSupportThreadState();
  page.context.openSupportComposer();
  page.state.supportDraftMessage = "  Не подключается VPN  ";
  const pending = page.context.submitSupportTicket();
  assert.equal(page.requests[0].path, "/api/mini-app/support/create");
  assert.deepEqual(JSON.parse(JSON.stringify(page.requests[0].body)), { message: "Не подключается VPN" });
  page.requests[0].resolve({ data: thread(3) });
  await pending;
  assert.equal(page.state.supportComposeOpen, false);
  assert.equal(page.state.activeSupportThread.ticket.id, 3);
});

test("repeated close clicks keep one animation and block background work", async () => {
  const page = harness();
  page.close(); page.close();
  assert.equal(page.timers.size, 1);
  await page.context.openSupportTicket(1, {silent: true});
  await page.context.openSupportTicket(2);
  assert.equal(page.requests.length, 0);
  page.finishAnimation();
  assert.equal(page.state.supportThreadOpen, false);
});

test("an initial load failure removes the loading modal and permits retry", async () => {
  const page = harness();
  page.context.closeSupportThreadState();
  const pending = page.context.openSupportTicket(2);
  page.requests[0].reject(new Error("offline"));
  await assert.rejects(pending, /offline/);
  assert.equal(page.state.supportThreadOpen, false);
  const retry = page.context.openSupportTicket(2);
  page.requests[1].resolve({data: thread(2)}); await retry;
  assert.equal(page.state.supportThreadOpen, true);
});

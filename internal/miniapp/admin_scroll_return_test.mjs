import test from "node:test";
import assert from "node:assert/strict";
import fs from "node:fs";
import vm from "node:vm";

const source = fs.readFileSync(new URL("./static/app.js", import.meta.url), "utf8");
function harness() {
  let top = 720;
  const calls = [];
  const state = { currentPage: "admin", adminSection: "home", adminMenuScrollTop: 0 };
  const context = vm.createContext({ state, getCurrentScrollTop: () => top, render: options => calls.push(options), haptic() {}, window: { clearTimeout() {} }, adminStatusPollTimer: 0, adminUsersSearchTimer: 0, adminSettingsSearchCatalog: {}, adminUsersSearchRequestID: 0, adminUserDetailRequestID: 0, adminFinanceRequestID: 0 });
  vm.runInContext(source.slice(source.indexOf("function rememberAdminMenuScroll("), source.indexOf("function setPage(")), context);
  return {context,state,calls,setTop:value=>{top=value;}};
}

test("returning from an admin section restores the menu position, not the section scroll", () => {
  const {context,state,calls,setTop}=harness();
  context.rememberAdminMenuScroll();
  state.adminSection="users";
  context.renderAdminTransition();
  assert.equal(calls.at(-1).scrollTop,0);
  setTop(1500);
  context.rememberAdminMenuScroll();
  context.closeAdminSection();
  assert.equal(state.adminSection,"home");
  assert.equal(calls.at(-1).scrollTop,720);
  assert.equal(calls.at(-1).preserveScroll,false);
  assert.equal(context.adminUsersSearchRequestID,1);
  setTop(410);
  context.rememberAdminMenuScroll();
  state.adminSection="status";
  context.closeAdminSection();
  assert.equal(calls.at(-1).scrollTop,410);
});

test("builder and plan editor returns reuse the menu scroll while their internal pages start at zero", () => {
  const {context,state,calls}=harness();
  context.rememberAdminMenuScroll();
  for (const [page,section] of [["dashboard","layout"],["buy","plans"]]) {
    state.currentPage=page; state.adminSection=section;
    context.renderAdminTransition();
    assert.equal(calls.at(-1).scrollTop,0);
    context.rememberAdminMenuScroll();
    state.currentPage="admin"; state.adminSection="home";
    context.renderAdminTransition();
    assert.equal(calls.at(-1).scrollTop,720);
  }
});

test("an absent scroller cannot erase the saved menu position and explicit transition offsets still work", () => {
  const {context,state,calls,setTop}=harness();
  context.rememberAdminMenuScroll();
  setTop(null);
  context.rememberAdminMenuScroll();
  assert.equal(state.adminMenuScrollTop,720);
  context.renderAdminTransition({scrollTop:25});
  assert.equal(calls.at(-1).scrollTop,25);
});

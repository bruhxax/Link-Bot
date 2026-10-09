import assert from "node:assert/strict";
import fs from "node:fs";
import test from "node:test";
import vm from "node:vm";
import { adminConsoleEnabled, visibleConsoleGroups, workspaceNavigationURL } from "./static/admin-console.mjs";
import { canAdmin } from "./static/administrators.mjs";

const source = fs.readFileSync(new URL("./static/app.js", import.meta.url), "utf8");
const entry = source.slice(source.indexOf("function getEntryPage()"), source.indexOf("function getEntryAdminSection()"));
function entryPage({ dedicated = false, adminURL = "", query = "", reload = false, saved = "dashboard" } = {}) {
  const redirects = [];
  const context = vm.createContext({ adminEntry: dedicated, adminDedicated: dedicated, adminBaseURL: adminURL, previewMode: false, remnaAdminEnabled: dedicated,
    urlParams: new URLSearchParams(query), PAGES: ["dashboard", "admin", "support", "servers", "reviews", "buy"],
    STORAGE_KEYS: { page: "page" }, isPageReload: () => reload, readSetting: () => saved,
    tg: null, workspaceNavigationURL,
    window: { location: { replace: url => redirects.push(url) } } });
  vm.runInContext(entry, context);
  return { page: context.getEntryPage(), redirects };
}

test("empty admin host preserves customer navigation and the embedded admin entry", () => {
  assert.equal(entryPage().page, "dashboard");
  assert.equal(entryPage({ query: "page=admin" }).page, "admin");
  assert.equal(entryPage({ query: "page=buy", reload: true, saved: "admin" }).page, "buy");
});

test("cross-origin administration and return navigation keep Telegram launch data only in the fragment", () => {
  const tg={initData:"user=%7B%22id%22%3A42%7D&auth_date=123&hash=signed",version:"9.1",platform:"android",themeParams:{bg_color:"#000000"}};
  for(const target of ["https://admin.example.com/?page=admin&section=users", "https://example.com/mini-app/?cabinet=1"]){
    const url=new URL(workspaceNavigationURL(target,tg));
    assert.equal(url.origin,new URL(target).origin);
    assert.equal(url.search,new URL(target).search);
    const hash=new URLSearchParams(url.hash.slice(1));
    assert.equal(hash.get("tgWebAppData"),tg.initData);
    assert.equal(hash.get("tgWebAppPlatform"),"android");
    assert.deepEqual(JSON.parse(hash.get("tgWebAppThemeParams")),tg.themeParams);
    assert.ok(!url.search.includes("hash=")&&!url.search.includes("auth_date"));
  }
  assert.equal(workspaceNavigationURL("https://admin.example.com/#private",null),"https://admin.example.com/");
});

test("opening administration in Mini App navigates the WebView without calling Telegram openLink", () => {
  const redirects=[];
  const tg={initData:"signed-launch",platform:"android",openLink:()=>{throw new Error("must stay in WebView");}};
  const context=vm.createContext({adminBaseURL:"https://admin.example.com",adminDedicated:false,previewMode:false,tg,workspaceNavigationURL,window:{location:{assign:url=>redirects.push(url)}}});
  vm.runInContext(source.slice(source.indexOf("function setPage(page)"), source.indexOf("function getCurrentScrollTop()")),context);
  context.setPage("admin");
  assert.equal(redirects.length,1);
  assert.equal(new URLSearchParams(new URL(redirects[0]).hash.slice(1)).get("tgWebAppData"),tg.initData);
});

test("legacy and saved admin entry redirects to the configured origin without forwarding secrets", () => {
  for (const options of [{ query: "page=admin&token=private" }, { reload: true, saved: "admin" }]) {
    assert.deepEqual(entryPage({ ...options, adminURL: "https://admin.example.com" }), {
      page: "dashboard", redirects: ["https://admin.example.com/"]
    });
  }
});

test("dedicated root opens administration and retains support deep links", () => {
  assert.equal(entryPage({ dedicated: true }).page, "admin");
  assert.equal(entryPage({ dedicated: true, query: "page=buy" }).page, "admin");
  assert.equal(entryPage({ dedicated: true, query: "page=admin&section=support" }).page, "support");
  assert.equal(entryPage({ dedicated: true, reload: true, saved: "reviews" }).page, "reviews");
});

test("shared console links redirect to the dedicated host, while legacy routes retain the old admin entry", () => {
  for (const page of ["support", "servers", "reviews"]) {
    assert.equal(entryPage({query: `page=admin&section=${page}`, reload:true}).page, "admin");
    assert.equal(entryPage({dedicated:true,query: `page=admin&section=${page}`, reload:true}).page, page);
    assert.deepEqual(entryPage({query:`page=admin&section=${page}&token=private`,adminURL:"https://admin.example.com"}), {
      page:"dashboard",redirects:[`https://admin.example.com/?page=admin&section=${page}`]
    });
  }
});

test("shared administration pages keep the admin entry in their saved URL", () => {
  const code = source.slice(source.indexOf("let lastPersistedNavigation ="),source.indexOf("function readSessionSetting("));
  for (const page of ["support","servers","reviews"]) {
    const paths=[];
    const context=vm.createContext({URL,state:{adminWorkspace:true,currentPage:page,adminSection:"home"},
      window:{location:{href:"https://example.com/mini-app/?page=admin&section=home"},history:{replaceState:(_,__,path)=>paths.push(path)}},
      writeSetting:()=>{},STORAGE_KEYS:{page:"page"}});
    vm.runInContext(code,context);context.persistNavigationState();
    assert.deepEqual(paths,[`/mini-app/?page=admin&section=${page}`]);
  }
});

test("console navigation honors restricted roles and the separate SMTP permission", () => {
  const render = permissions => visibleConsoleGroups(permission => canAdmin({ isAdmin: true, isOwner: false, permissions }, permission)).flatMap(group => group[2]).map(route => route[0]);
  const statusOnly = render(["status"]);
  assert.deepEqual(statusOnly, ['status']);
  const mailOnly = render(["smtp"]);
  assert.deepEqual(mailOnly, ['broadcast']);
});

test('plan and layout editors stay in the administration workspace', () => {
  for (const [name, next] of [['enterAdminPlanEditor', 'exitAdminPlanEditor'], ['enterAdminLayoutEditor', 'exitAdminLayoutEditor']]) {
    const code = source.slice(source.indexOf(`function ${name}()`), source.indexOf(`function ${next}()`));
    const state = {adminSettingsDraft:{plans:[],layout:{elements:[]}},adminJSONDrafts:{}};
    const noop = () => {};
    const context = vm.createContext({state,remnaAdminEnabled:true,deepClone:structuredClone,syncAdminSettingsDraft:noop,rememberAdminMenuScroll:noop,ensureAdminVisualLayoutDraft:noop,ensureSelections:noop,haptic:noop,renderAdminTransition:noop,previousBottomNavIndex:0,notificationPopoverTimer:0,window:{clearTimeout:noop}});
    vm.runInContext(code,context);context[name]();
    assert.equal(state.currentPage,'admin');assert.equal(state.adminWorkspace,true);
    assert.equal(state.adminLayoutEditing,false);assert.notEqual(state.adminPlanEditing,true);
  }
});

test('empty configuration never enables the new console through an admin URL or stored preference', () => {
  assert.equal(adminConsoleEnabled({adminBaseURL:"",origin:"https://example.com"}),false);
  assert.equal(adminConsoleEnabled({adminBaseURL:"",origin:"https://example.com",previewConsole:true}),false);
  assert.equal(adminConsoleEnabled({adminBaseURL:"https://admin.example.com",origin:"https://example.com"}),false);
  assert.equal(adminConsoleEnabled({adminBaseURL:"https://admin.example.com",origin:"https://admin.example.com"}),true);
  assert.equal(adminConsoleEnabled({adminBaseURL:"",origin:"http://127.0.0.1",previewMode:true,previewConsole:true}),true);
});

test('legacy plan and layout editors retain their original customer previews', () => {
  for (const [name,next,page,flag] of [['enterAdminPlanEditor','exitAdminPlanEditor','buy','adminPlanEditing'],['enterAdminLayoutEditor','exitAdminLayoutEditor','dashboard','adminLayoutEditing']]) {
    const code=source.slice(source.indexOf(`function ${name}()`),source.indexOf(`function ${next}()`));
    const state={adminSettingsDraft:{plans:[],layout:{elements:[]}},adminJSONDrafts:{}};
    const noop=()=>{};
    const context=vm.createContext({state,remnaAdminEnabled:false,deepClone:structuredClone,syncAdminSettingsDraft:noop,rememberAdminMenuScroll:noop,ensureAdminVisualLayoutDraft:noop,ensureSelections:noop,haptic:noop,renderAdminTransition:noop,previousBottomNavIndex:0,notificationPopoverTimer:0,window:{clearTimeout:noop}});
    vm.runInContext(code,context);context[name]();
    assert.equal(state.currentPage,page);assert.equal(state[flag],true);assert.equal(state.adminWorkspace,false);
  }
});

test('legacy admin entry renders its original settings menu instead of an empty page', () => {
  const code = source.slice(source.indexOf('function renderAdminPage()'), source.indexOf('let adminSettingsSearchCatalog'));
  const context = vm.createContext({state:{adminSection:'home'}, remnaAdminEnabled:false,
    syncAdminSettingsDraft:()=>{}, pageClass:()=> 'active', localizedText:ru=>ru,
    renderAdminSettingsSearch:()=> '<input type="search">', renderAdminExtraAccess:()=> '',
    renderAdminMenuGroup:(_,items)=>items.map(item=> `<button data-value="${item[2]}">${item[0]}</button>`).join('')});
  vm.runInContext(code,context);
  const menu = context.renderAdminPage();
  for (const route of ['users','administrators','plans','layout','appearance','integrations','content','broadcast']) {
    assert.ok(menu.includes(`data-value="${route}"`), `Missing legacy route: ${route}`);
  }
  assert.ok(menu.includes('page admin-page active'));
});

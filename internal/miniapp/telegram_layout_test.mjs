import assert from "node:assert/strict";
import fs from "node:fs";
import vm from "node:vm";
import test from "node:test";
import { telegramLayout, syncTelegramLayout } from "./static/telegram-layout.mjs";

test("fullscreen reserves both device and Telegram controls, using stable height", () => {
  assert.deepEqual(telegramLayout({isFullscreen:true,isExpanded:true,viewportStableHeight:844,viewportHeight:700,
    safeAreaInset:{top:24,bottom:34,left:8,right:8},contentSafeAreaInset:{top:56,bottom:12,left:4,right:4}}, 600),
    {mode:"fullscreen",height:844,top:80,bottom:46,left:12,right:12});
});

test("compact and fullsize are detected without expanding the app", () => {
  assert.equal(telegramLayout({isExpanded:false,viewportHeight:350},844).mode,"compact");
  assert.equal(telegramLayout({isExpanded:true,viewportStableHeight:780},844).mode,"fullsize");
  assert.equal(telegramLayout(null,844).mode,"browser");
  assert.equal(telegramLayout({viewportHeight:0},844).height,844);
});

test("leaving fullscreen clears all stale insets", () => {
  const values = new Map(); const root = {dataset:{},style:{setProperty:(key,value)=>values.set(key,value)}};
  syncTelegramLayout(root,{isFullscreen:true,safeAreaInset:{top:24},contentSafeAreaInset:{top:56}},844);
  assert.equal(values.get("--telegram-safe-top"),"80px");
  syncTelegramLayout(root,{isExpanded:true,viewportStableHeight:780},844);
  assert.equal(root.dataset.launchMode,"fullsize");
  assert.equal(values.get("--telegram-safe-top"),"0px");
  syncTelegramLayout(root,null,600);
  assert.equal(root.dataset.launchMode,"browser");
  assert.equal(values.get("--app-viewport-height"),"600px");
});

test("invalid inset values do not break the layout", () => {
  const result=telegramLayout({safeAreaInset:{top:NaN,bottom:-1,left:Infinity,right:"12"}},844);
  assert.deepEqual([result.top,result.bottom,result.left,result.right],[0,0,0,12]);
});

test("Telegram mode and inset events update mounted layout without rerendering", () => {
  const source=fs.readFileSync(new URL("./static/app.js",import.meta.url),"utf8");
  const start=source.indexOf("function initTelegram()");
  const end=source.indexOf("async function handlePostBootstrapFlow()",start);
  const events=new Map(); const values=new Map(); const root={dataset:{},style:{setProperty:(key,value)=>values.set(key,value)}};
  const tg={initData:"test",isExpanded:false,viewportStableHeight:350,ready(){},onEvent:(name,fn)=>events.set(name,fn),expand(){throw Error("unexpected forced expansion");}};
  const context=vm.createContext({tg,clientSurface:"telegram",syncTelegramLayout,document:{documentElement:root},window:{innerHeight:844,addEventListener(){}},cabinetWideMedia:{addEventListener(){}},syncCabinetLayout(){}});
  vm.runInContext(source.slice(start,end),context); context.initTelegram();
  assert.equal(root.dataset.launchMode,"compact");
  tg.isFullscreen=true; tg.viewportStableHeight=844; tg.safeAreaInset={top:24}; tg.contentSafeAreaInset={top:56}; events.get("fullscreenChanged")();
  assert.equal(root.dataset.launchMode,"fullscreen"); assert.equal(values.get("--telegram-safe-top"),"80px");
  tg.contentSafeAreaInset.top=60; events.get("contentSafeAreaChanged")(); assert.equal(values.get("--telegram-safe-top"),"84px");
  tg.safeAreaInset.top=30; events.get("safeAreaChanged")(); assert.equal(values.get("--telegram-safe-top"),"90px");
  tg.isFullscreen=false; tg.isExpanded=true; tg.safeAreaInset={}; tg.contentSafeAreaInset={}; tg.viewportStableHeight=780; events.get("viewportChanged")();
  assert.equal(root.dataset.launchMode,"fullsize"); assert.equal(values.get("--telegram-safe-top"),"0px");
});

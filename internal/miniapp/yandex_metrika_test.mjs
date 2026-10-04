import test from "node:test";
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import vm from "node:vm";

const source=readFileSync(new URL("./static/yandex-metrika.js",import.meta.url),"utf8");
function run(id="12345678",pathname="/mini-app/",search="?page=reviews&token=SECRET&tgWebAppData=AUTH") {
  const events=new Map(),scripts=[];
  const window={addEventListener:(name,callback)=>events.set(name,callback)};
  const document={querySelector:()=>({content:id}),referrer:"https://example.com/login?token=REF_SECRET#auth=HASH_SECRET",createElement:()=>({}),head:{appendChild:s=>scripts.push(s)}};
  vm.runInNewContext(source,{window,document,location:{origin:"https://vpn.example",pathname,search,hash:"#tgWebAppData=HASH_AUTH"},URL,URLSearchParams});
  return {window,events,scripts,calls:()=>Array.from(window.ym?.a||[],args=>Array.from(args))};
}
test("Metrika sends sanitized pageviews and ignores sensitive query/hash/referrers",()=>{
  const c=run(),calls=c.calls();
  assert.equal(c.scripts[0].src,"https://mc.yandex.ru/metrika/tag.js");
  assert.equal(calls[0][1],"init"); assert.equal(calls[0][2].defer,true); assert.equal(calls[0][2].webvisor,false); assert.equal(calls[0][2].trackLinks,false);
  assert.equal(calls[1][2],"https://vpn.example/mini-app/#reviews");
  assert.equal(calls[1][3].referer,"https://example.com/login");
  assert.doesNotMatch(JSON.stringify(calls),/SECRET|AUTH|tgWebAppData|token=/);
  c.events.get("miniapp:pageview")({detail:{page:"settings"}});
  c.events.get("miniapp:pageview")({detail:{page:"settings"}});
  c.events.get("miniapp:pageview")({detail:{page:"token=SECRET"}});
  const hits=c.calls().filter(call=>call[1]==="hit"); assert.equal(hits.length,2); assert.equal(hits[1][2],"https://vpn.example/mini-app/#settings");
});
test("disabled or invalid counter does not load tracking",()=>{
  for (const id of ["","__YANDEX_METRIKA_COUNTER_ID__","0","-1","<script>"]) { const c=run(id); assert.equal(c.scripts.length,0); assert.equal(c.events.size,0); }
});
test("landing page is counted without mini app hash",()=>{
  assert.equal(run("12345678","/").calls()[1][2],"https://vpn.example/");
});

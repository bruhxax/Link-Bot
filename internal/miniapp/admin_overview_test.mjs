import test from "node:test";
import assert from "node:assert/strict";
import fs from "node:fs";
import vm from "node:vm";
import {groupOverviewPoints, overviewSegments, overviewChartGeometry, overviewRingPath, overviewMorphPaths, selectOverviewPeriod} from "../../admin-ui/overview-data.mjs";
import {applyConsoleAppearance, CONSOLE_COLORS, CONSOLE_GROUPS} from "./static/admin-console.mjs";

test("dedicated console palette replaces every shared control token with independent values",()=>{
 const root={dataset:{glass:"on",background:"custom"},style:{setProperty:(name,value)=>tokens[name]=value}},tokens={"--text":"#ff0000","--control-bg":"#00ff00","--accent":"#0000ff"};
 applyConsoleAppearance(root);
 for(const [key,value] of Object.entries(CONSOLE_COLORS)) assert.equal(tokens[key],value);
 assert.equal(root.dataset.background,"none");assert.equal(root.dataset.glass,"off");
 const routes=CONSOLE_GROUPS.flatMap(group=>group[2]);
 assert.equal(routes.length,new Set(routes.map(route=>route[0])).size);
 assert.ok(routes.every(Boolean));
 const material=fs.readFileSync(new URL("./static/control-material.css",import.meta.url),"utf8");
 assert.ok(material.includes(':root:not([data-admin-console="on"])'));
});

test("overview percentages use real totals including empty and single-segment data",()=>{
 const empty=overviewSegments([{key:"zero",count:0}]);assert.equal(empty.total,0);assert.equal(empty.items[0].fraction,0);
 const graph=overviewSegments([{count:29},{count:13},{count:4}]);assert.equal(graph.total,46);
 assert.ok(Math.abs(graph.items.reduce((sum,item)=>sum+item.fraction,0)-1)<1e-10);
 assert.equal(overviewSegments([{count:5}]).items[0].fraction,1);
 for(const segment of [...graph.items,...overviewSegments([{count:5}]).items]) {
  const path=overviewRingPath(segment);assert.ok(path.startsWith("M"));assert.ok(path.endsWith("Z"));assert.ok(!path.includes("NaN"));
 }
});

test("days, weeks and months retain all real observations without mixing dates",()=>{
 const points=[{date:"2026-01-31",value:2},{date:"2026-02-01",value:3},{date:"2026-02-02",value:7}];
 assert.equal(groupOverviewPoints(points,"days").length,3);
 assert.deepEqual(groupOverviewPoints(points,"weeks"),[{date:"2026-01-26",value:5},{date:"2026-02-02",value:7}]);
 assert.deepEqual(groupOverviewPoints(points,"months"),[{date:"2026-01-01",value:2},{date:"2026-02-01",value:10}]);
 for(const input of [[],[{date:"2026-01-01",value:0}],[{date:"2026-01-01",value:5}]]) {
  const graph=overviewChartGeometry(input);assert.ok(Number.isFinite(graph.ceiling));assert.ok(!graph.line.includes("NaN"));
 }
});

test("late overview response cannot replace a newer period",async()=>{
 const source=fs.readFileSync(new URL("./static/app.js",import.meta.url),"utf8");
 const part=source.slice(source.indexOf("let adminOverviewRequestID = 0;"),source.indexOf("async function refreshAdminFinance("));
 const pending=[],state={adminWorkspace:true,adminSection:"home",adminOverviewDays:7,adminOverviewBusy:false};
 const context=vm.createContext({state,previewMode:false,isAdminUser:()=>true,render:()=>{},post:()=>new Promise(resolve=>pending.push(resolve))});
 vm.runInContext(part,context);
 const first=context.refreshAdminOverview();state.adminOverviewDays=90;
 const second=context.refreshAdminOverview();
 pending[1]({data:{days:90}});await second;pending[0]({data:{days:7}});await first;
 assert.equal(state.adminOverview.days,90);assert.equal(state.adminOverviewBusy,false);
});

test("panel outage retains only previously loaded panel counts and period switches reuse the catalog",async()=>{
 const source=fs.readFileSync(new URL("./static/app.js",import.meta.url),"utf8");
 const part=source.slice(source.indexOf("let adminOverviewRequestID = 0;"),source.indexOf("async function refreshAdminFinance("));
 const state={adminWorkspace:true,adminSection:"home",adminOverviewDays:7,adminOverviewBusy:false,adminOverview:{days:30,users:{source:"panel",total:551},subscriptions:[{label:"1 месяц · Безлимит",count:12}]}},requests=[];
 const context=vm.createContext({state,previewMode:false,isAdminUser:()=>true,render:()=>{},post:async(url,request)=>{requests.push(request);return {data:{days:request.days,users:null,subscriptions:[],panelError:"Панель недоступна",revenue:{allRub:100}}}}});
 vm.runInContext(part,context);
 await context.refreshAdminOverview({force:false});
 assert.equal(requests[0].force,false);
 assert.equal(state.adminOverview.users.total,551);
 assert.equal(state.adminOverview.days,30);
 assert.equal(state.adminOverview.revenue.allRub,100);
 assert.equal(state.adminOverview.panelError,"Панель недоступна");
 state.adminOverview={days:7,users:{total:59000},subscriptions:[]};
 await context.refreshAdminOverview();
 assert.equal(requests[1].force,false);
 assert.equal(state.adminOverview.users,null);
 assert.equal(state.adminOverview.days,90);
});

test("all periods use the same 90-day snapshot without requesting the panel",async()=>{
 const source=fs.readFileSync(new URL("./static/app.js",import.meta.url),"utf8");
 const part=source.slice(source.indexOf("let adminOverviewRequestID = 0;"),source.indexOf("async function refreshAdminFinance("));
 const daily=Array.from({length:90},(_,i)=>({date:`2026-${i}`,value:i}));
 const snapshot={days:90,users:{source:"panel",total:551,daily},revenue:{daily}};
 let renders=0,requests=0;
 const state={adminWorkspace:true,adminSection:"home",adminOverviewDays:30,adminOverviewBusy:true,adminOverview:snapshot};
 const context=vm.createContext({state,previewMode:false,isAdminUser:()=>true,render:()=>renders++,post:()=>{requests++;throw Error("period switch must be local");}});
 vm.runInContext(part,context);
 for(const days of [7,30,90,7]) {
  context.setAdminOverviewDays(days);
  const selected=selectOverviewPeriod(state.adminOverview,days);
  assert.equal(state.adminOverviewDays,days);
  assert.equal(selected.users.daily.length,days);
  assert.equal(selected.revenue.daily.at(-1).value,89);
  assert.equal(selected.users.total,551);
  assert.equal(snapshot.users.daily.length,90);
 }
 assert.equal(requests,0);assert.equal(renders,4);
});

test("curve morphs retain identical topology across empty, single, daily and monthly data",()=>{
 const cases=[[],[{value:0}],[{value:17}],Array.from({length:90},(_,i)=>({value:i%11})),[{value:20},{value:4},{value:12}]];
 const paths=cases.map(points=>overviewMorphPaths(overviewChartGeometry(points)));
 const commands=path=>path.match(/[MLZ]/g).join("");
 for(const path of paths) {
  assert.equal(commands(path.line),commands(paths[0].line));
  assert.equal(commands(path.area),commands(paths[0].area));
  assert.ok(!/NaN|Infinity|undefined/.test(path.line+path.area));
  assert.ok(path.line.startsWith("M44.000,"));assert.ok(path.line.includes("L586.000,"));
 }
 const graph=overviewChartGeometry(cases[4]);
 const path=overviewMorphPaths(graph);
 assert.equal(path.line.split(" ")[90],`L${graph.coordinates[1].x.toFixed(3)},${graph.coordinates[1].y.toFixed(3)}`);
});

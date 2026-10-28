import test from "node:test";
import assert from "node:assert/strict";
import fs from "node:fs";
import vm from "node:vm";
import { mergeUserActivity, renderUserActivity, formatActivityAmount, activityRows, activityDetails } from "./static/user-activity.mjs";

test("torrent reports have a dedicated filter and escape panel data in both admin histories",()=>{
  const torrent={id:2,source:"remnawave",action:"torrent_blocker.report",title:"Обнаружен торрент-трафик",status:"info",createdAt:"2026-10-28T10:00:00Z",details:[{label:"Нода",value:"<script>bad()</script>"},{label:"IP",value:"2001:db8::1"},{label:"Результат",value:"IP заблокирован на ноде"}]};
  const ordinary={id:1,source:"miniapp",action:"purchase",status:"success",details:[]};
  assert.deepEqual(activityRows([ordinary,torrent],"torrents").map(row=>row.item.id),[2]);
  const html=renderUserActivity({data:{items:[ordinary,torrent]},filter:"torrents",expanded:["event:2"]});
  assert.match(html,/data-value="torrents" aria-pressed="true"/);
  assert.match(html,/Remnawave/);assert.match(html,/2001:db8::1/);assert.match(html,/&lt;script&gt;/);
  assert.doesNotMatch(html,/<script>/);assert.doesNotMatch(html,/Запрос оплаты/);
});

test("activity amounts trim storage zeroes and preserve fractional and crypto amounts",()=>{
  assert.equal(formatActivityAmount("89.00000000 RUB"),"89 ₽");
  assert.equal(formatActivityAmount("10.500000 USD"),"10,5 $");
  assert.equal(formatActivityAmount("0.00000001 BTC"),"0,00000001 BTC");
  assert.equal(formatActivityAmount("not an amount"),"not an amount");
});

test("payment transitions group by order without merging requests or different payments",()=>{
  const event=(id,order,action,status="pending")=>({id,source:"payment",action,status,details:[{label:"Платёж",value:order}]});
  const items=[event(1,"#9","payment.new"),event(3,"#9","payment.paid","success"),event(2,"#10","payment.pending"),{id:4,source:"miniapp",action:"purchase",status:"failed",details:[]}];
  const rows=activityRows(items);
  assert.deepEqual(rows.map(row=>row.key),["event:4","payment:9","payment:10"]);
  assert.equal(rows[1].item.action,"payment.paid");
  assert.deepEqual(rows[1].events.map(event=>event.id),[3,1]);
  assert.equal(activityRows(items,"errors").length,1);
  assert.equal(activityRows(items,"payments").length,3);
  assert.deepEqual(activityDetails({details:[{label:"Тариф",value:"Месяц"},{label:"Месяцев",value:"1"},{label:"Автоплатёж",value:"Нет"},{label:"Тип покупки",value:"Подписка"}]}),[{label:"Тариф",value:"Месяц"}]);
});

const page=(ids,hasMore=false,customerId=9)=>({customerId,items:ids.map(id=>({id,title:`Event ${id}`,source:"payment",status:"success",createdAt:"2026-10-27T00:00:00Z",details:[]})),hasMore});

test("history keeps stable pages while live events arrive, deduplicates, and resets for another customer",()=>{
  let data=mergeUserActivity(null,page([9,8],true));
  data=mergeUserActivity(data,page([7,6],false),true);
  data=mergeUserActivity(data,page([10,9],true));
  assert.deepEqual(data.items.map(item=>item.id),[10,9,8,7,6]);
  assert.equal(data.hasMore,false);
  data=mergeUserActivity(data,page([100,99],true));
  assert.deepEqual(data.items.map(item=>item.id),[100,99]);
  assert.equal(data.hasMore,true);
  data=mergeUserActivity(data,page([3],false,22));
  assert.deepEqual(data.items.map(item=>item.id),[3]);
  assert.equal(data.customerId,22);
});

test("history renderer escapes event fields and exposes retry and pagination",()=>{
  const escape=value=>String(value).replaceAll("&","&amp;").replaceAll("<","&lt;").replaceAll('"',"&quot;");
  const data=page([1],true);
  data.items[0].title='<img src=x onerror="alert(1)">';
  data.items[0].details=[{label:"Тариф",value:"<script>secret()</script>"}];
  const html=renderUserActivity({data,busy:false,error:"<failure>"},{escapeHtml:escape,escapeAttribute:escape});
  assert.ok(!html.includes("<script>")&&!html.includes("<img"));
  assert.ok(html.includes("&lt;script>")&&html.includes("&lt;failure>"));
  assert.ok(html.includes('data-action="admin-user-activity-refresh"')&&html.includes('data-action="admin-user-activity-more"'));
});

test("history expands complete payment transitions without repeating them as large cards",()=>{
  const data=page([3,2,1]);
  data.items.forEach((item,index)=>Object.assign(item,{source:"payment",action:["payment.paid","payment.pending","payment.new"][index],details:[{label:"Платёж",value:"#8"},{label:"Сумма",value:"89.00000000 RUB"}]}));
  const closed=renderUserActivity({data});
  assert.equal((closed.match(/class="user-activity-entry"/g)||[]).length,1);
  assert.ok(!closed.includes("user-activity-states")&&!closed.includes("89.00000000"));
  const open=renderUserActivity({data,expanded:["payment:8"]});
  assert.ok(open.includes("user-activity-states")&&open.includes("Ожидает оплаты")&&open.includes('aria-expanded="true"'));
});

const source=fs.readFileSync(new URL("./static/app.js",import.meta.url),"utf8");
function setup(){
  const requests=[];
  const ctx=vm.createContext({state:{adminUserDetail:{customerId:9},adminSection:"users",adminUserActivity:null,adminUserActivityBusy:false},previewMode:false,adminUserActivityRequestID:0,adminUserDetailRequestID:0,canAdmin:()=>true,renderRealtime(){},render(){},mergeUserActivity,post:(url,body)=>new Promise((resolve,reject)=>requests.push({url,body,resolve,reject}))});
  const start=source.indexOf("function resetAdminUserActivity(");
  const end=source.indexOf("async function refreshAdminDirectMessage(",start);
  vm.runInContext(source.slice(start,end),ctx);
  return {ctx,requests};
}

test("late history response cannot replace another user's card or clear its busy state",async()=>{
  const {ctx,requests}=setup();
  const old=ctx.refreshAdminUserActivity();
  ctx.resetAdminUserActivity();ctx.adminUserDetailRequestID++;ctx.state.adminUserDetail={customerId:22};
  const fresh=ctx.refreshAdminUserActivity();
  requests[0].resolve({data:page([1])});await old;
  assert.equal(ctx.state.adminUserActivity,null);assert.equal(ctx.state.adminUserActivityBusy,true);
  requests[1].resolve({data:page([2],false,22)});await fresh;
  assert.equal(ctx.state.adminUserActivity.customerId,22);assert.equal(ctx.state.adminUserActivityBusy,false);
});

test("history uses the oldest ID cursor and preserves visible events on a failed refresh",async()=>{
  const {ctx,requests}=setup();
  ctx.state.adminUserActivity=page([9,8],true);
  const more=ctx.refreshAdminUserActivity({more:true});
  assert.equal(requests[0].body.beforeId,8);
  requests[0].resolve({data:page([7,6])});await more;
  const failed=ctx.refreshAdminUserActivity({silent:true});
  requests[1].reject(new Error("offline"));await failed;
  assert.equal(ctx.state.adminUserActivity.items.length,4);assert.equal(ctx.state.adminUserActivityError,"offline");
  ctx.state.adminUserDetail.panelOnly=true;
  await ctx.refreshAdminUserActivity();assert.equal(requests.length,2);
  ctx.state.adminUserDetail.panelOnly=false;ctx.canAdmin=()=>false;
  await ctx.refreshAdminUserActivity();assert.equal(requests.length,2);
});

test("live history refresh preserves its filter and expanded rows until the user changes card",async()=>{
  const {ctx,requests}=setup();
  ctx.filterAdminUserActivity("errors");ctx.toggleAdminUserActivity("event:9");
  ctx.filterAdminUserActivity("invalid");ctx.toggleAdminUserActivity("invalid-key");
  const refresh=ctx.refreshAdminUserActivity({silent:true});requests[0].resolve({data:page([9])});await refresh;
  assert.equal(ctx.state.adminUserActivityFilter,"errors");
  assert.equal(ctx.state.adminUserActivityExpanded[0],"event:9");
  ctx.resetAdminUserActivity();
  assert.equal(ctx.state.adminUserActivityFilter,"all");assert.equal(ctx.state.adminUserActivityExpanded.length,0);
});

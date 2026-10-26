import test from "node:test";
import assert from "node:assert/strict";
import fs from "node:fs";
import vm from "node:vm";
import { mergeUserActivity, renderUserActivity } from "./static/user-activity.mjs";

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

const source=fs.readFileSync(new URL("./static/app.js",import.meta.url),"utf8");
function setup(){
  const requests=[];
  const ctx=vm.createContext({state:{adminUserDetail:{customerId:9},adminSection:"users",adminUserActivity:null,adminUserActivityBusy:false},previewMode:false,adminUserActivityRequestID:0,adminUserDetailRequestID:0,canAdmin:()=>true,renderRealtime(){},mergeUserActivity,post:(url,body)=>new Promise((resolve,reject)=>requests.push({url,body,resolve,reject}))});
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

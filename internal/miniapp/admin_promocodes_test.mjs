import test from "node:test";
import assert from "node:assert/strict";
import fs from "node:fs";
import vm from "node:vm";
import {initialPromoDraft,promoCreatePayload,promoSettingsPayload,promoStatus,promoRows,promoReward,promoLocalDate} from "../../admin-ui/promocodes-data.mjs";
test("all five promo rewards produce the strict server payload with only their own values",()=>{
 const base={...initialPromoDraft,code:"work_20"};
 for(const [type,unit,rewardType,rewardValue,discountPercent,rewardTrafficGb] of [["discount","days","discount",0,20,0],["balance","days","balance",100,0,0],["subscription","days","days",30,0,0],["subscription","traffic","traffic",10,0,0],["subscription","days_traffic","days_traffic",30,0,10]]){
 assert.deepEqual(promoCreatePayload({...base,type,unit}),{code:"WORK_20",rewardType,rewardValue,discountPercent,rewardTrafficGb,maxRedemptions:0,expiresAt:""});
 }
});
test("invalid rewards, fractional limits and past expiry fail before a request",()=>{
 for(const changes of [{code:"!"},{discount:100},{discount:0},{type:"balance",balance:1.2},{limit:1.9},{limit:-1},{expires:"bad"},{expires:"2000-01-01T00:00"},{type:"subscription",unit:"days_traffic",combinedTraffic:0}])assert.throws(()=>promoCreatePayload({...initialPromoDraft,code:"WORK",...changes}));
 assert.deepEqual(promoSettingsPayload({limit:"",expires:""}),{maxRedemptions:0,expiresAt:""});
});
test("search retains every status and sorting is stable; expiration uses the current time",()=>{
 const items=[{id:1,code:"WORK",status:"active",createdAt:"2026-10-01",expiresAt:"2026-10-10"},{id:2,code:"WELCOME",status:"exhausted",createdAt:"2026-10-12"}];
 assert.equal(promoRows(items,"wo")[0].id,1);assert.deepEqual(promoRows(items).map(i=>i.id),[2,1]);assert.equal(promoStatus(items[0],Date.parse("2026-10-12")),"expired");
 assert.equal(promoStatus({status:"active",isActive:false}),"inactive");assert.equal(promoStatus({maxRedemptions:2,redemptionCount:2}),"exhausted");assert.equal(promoStatus(items[1]),"exhausted");
 assert.equal(promoReward({rewardType:"days_traffic",rewardValue:30,rewardTrafficGb:10}),"+30 дн. · +10 ГБ");
 const value="2026-11-01T15:30:00Z";assert.equal(new Date(promoLocalDate(value)).getTime(),Date.parse(value));
});
test("native promo mutations update the list without dropping the draft and always release the busy guard",async()=>{
 const source=fs.readFileSync(new URL("./static/app.js",import.meta.url),"utf8");const state={adminBusy:"",data:{admin:{}},adminPromoConsoleDraft:{code:"UNSAVED"}};let request;
 const ctx=vm.createContext({state,canAdmin:()=>true,render(){},showToast(){},post:async(url,body)=>{request={url,body};return{data:{promoCodes:[{id:1,code:"WORK"}]}}}});
 vm.runInContext(source.slice(source.indexOf("async function mutateConsolePromo("),source.indexOf("async function deleteAdminPromoCode(")),ctx);
 await ctx.mutateConsolePromo("update",{id:1,updatedAt:"revision",isActive:false});assert.equal(request.url,"/api/mini-app/admin/promocodes/update");assert.equal(state.data.admin.promoCodes[0].code,"WORK");assert.equal(state.adminPromoConsoleDraft.code,"UNSAVED");assert.equal(state.adminBusy,"");
 ctx.post=async()=>{throw Error("stale")};await assert.rejects(ctx.mutateConsolePromo("update",{id:1}),/stale/);assert.equal(state.adminBusy,"");ctx.canAdmin=()=>false;await assert.rejects(ctx.mutateConsolePromo("delete",{id:1}),/Недостаточно прав/);
});

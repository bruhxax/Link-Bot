import test from "node:test";
import assert from "node:assert/strict";
import fs from "node:fs";
import vm from "node:vm";

const source=fs.readFileSync(new URL("./static/app.js",import.meta.url),"utf8");
function load(context,name,async=false) {
  const start=source.indexOf(`${async?"async ":""}function ${name}(`);
  const next=/\n(?:async )?function /.exec(source.slice(start+1));
  assert.ok(start>=0&&next,name);
  vm.runInContext(source.slice(start,start+1+next.index),context);
}
function context() {
  const state={adminBroadcast:{updatedAt:"one",deleteAfterMinutes:0},adminBroadcastExpirationDraft:0,adminBroadcastExpirationDirty:false,adminAudienceDraft:{telegram:{}},adminBroadcastButtonsDraft:[],adminBroadcastButtonsDirty:false,adminBroadcastBusy:"",adminBroadcastConfirmOpen:true};
  const ctx=vm.createContext({adminBroadcastWorkflowMode:()=>"message",defaultAdminSurvey:()=>({enabled:false}),state,canAdmin:()=>true,render(){},deepClone:structuredClone,icon:()=>"<svg></svg>"});
  load(ctx,"setAdminBroadcastDraft");
  load(ctx,"updateConsoleBroadcastExpiration");
  load(ctx,"saveAdminBroadcastExpiration",true);
  load(ctx,"ensureAdminBroadcastButtonsSaved",true);
  return ctx;
}

test("changing temporary duration invalidates confirmation and preserves it across polling or button saves",()=>{
  const ctx=context();
  ctx.updateConsoleBroadcastExpiration(720);
  assert.equal(ctx.state.adminBroadcastExpirationDraft,720);
  assert.equal(ctx.state.adminBroadcastExpirationDirty,true);
  assert.equal(ctx.state.adminBroadcastConfirmOpen,false);
  ctx.setAdminBroadcastDraft({updatedAt:"poll",deleteAfterMinutes:0});
  assert.equal(ctx.state.adminBroadcastExpirationDraft,720);
  ctx.setAdminBroadcastDraft({updatedAt:"buttons",deleteAfterMinutes:0},{forceButtons:true});
  assert.equal(ctx.state.adminBroadcastExpirationDraft,720);
  ctx.setAdminBroadcastDraft({updatedAt:"template",deleteAfterMinutes:60},{forceButtons:true,forceExpiration:true});
  assert.equal(ctx.state.adminBroadcastExpirationDraft,60);
  assert.equal(ctx.state.adminBroadcastExpirationDirty,false);
  for(const invalid of [-1,2821,1.5]) ctx.updateConsoleBroadcastExpiration(invalid);
  assert.equal(ctx.state.adminBroadcastExpirationDraft,60);
  ctx.state.adminBroadcast.status="running";
  ctx.updateConsoleBroadcastExpiration(0);
  assert.equal(ctx.state.adminBroadcastExpirationDraft,60);
});

test("send preparation saves expiration with draft version before saving buttons",async()=>{
  const ctx=context(),calls=[];
  ctx.updateConsoleBroadcastExpiration(720);
  ctx.state.adminBroadcastButtonsDirty=true;
  ctx.post=async(url,body)=>{calls.push({url,body});return {data:{updatedAt:"expiration-saved",deleteAfterMinutes:720}}};
  ctx.saveAdminBroadcastButtons=async()=>{assert.equal(ctx.state.adminBroadcast.updatedAt,"expiration-saved");calls.push("buttons");return ctx.state.adminBroadcast};
  await ctx.ensureAdminBroadcastButtonsSaved();
  assert.equal(calls[0].url,"/api/mini-app/admin/broadcast/expiration");
  assert.equal(calls[0].body.deleteAfterMinutes,720);
  assert.equal(calls[0].body.updatedAt,"one");
  assert.equal(calls[1],"buttons");
  assert.equal(ctx.state.adminBroadcastExpirationDirty,false);
  assert.equal(ctx.state.adminBroadcastBusy,"");
});

test("a failed duration save prevents sending and keeps the unsaved duration",async()=>{
  const ctx=context();
  ctx.updateConsoleBroadcastExpiration(720);
  ctx.post=async()=>{throw Error("draft changed")};
  ctx.saveAdminBroadcastButtons=async()=>assert.fail("must not continue after failed expiration save");
  await assert.rejects(ctx.ensureAdminBroadcastButtonsSaved(),/draft changed/);
  assert.equal(ctx.state.adminBroadcastExpirationDirty,true);
  assert.equal(ctx.state.adminBroadcastExpirationDraft,720);
  assert.equal(ctx.state.adminBroadcastBusy,"");
});

test("send confirmation states per-recipient expiration and stays empty for permanent messages",()=>{
  const ctx=context();
  ctx.icon=()=>"";
  ctx.state.adminAudienceCount={telegram:10};
  load(ctx,"renderAdminBroadcastConfirm");
  assert.doesNotMatch(ctx.renderAdminBroadcastConfirm(false),/удалится/);
  ctx.state.adminBroadcast.deleteAfterMinutes=720;
  assert.match(ctx.renderAdminBroadcastConfirm(false),/у каждого получателя через 720 мин/);
  assert.match(ctx.renderAdminBroadcastConfirm(true),/deleted after 720 min/);
});

test("the embedded admin editor exposes a switch and guarded minute field",()=>{
  const ctx=context();
  load(ctx,"renderAdminBroadcastExpiration");
  assert.match(ctx.renderAdminBroadcastExpiration(false,false),/role="switch"/);
  assert.doesNotMatch(ctx.renderAdminBroadcastExpiration(false,false),/type="number"/);
  ctx.updateConsoleBroadcastExpiration(720);
  const html=ctx.renderAdminBroadcastExpiration(true,false);
  assert.match(html,/data-input="admin-broadcast-temporary" checked disabled/);
  assert.match(html,/min="1" max="2820" step="1"/);
  assert.match(html,/value="720" disabled/);
});

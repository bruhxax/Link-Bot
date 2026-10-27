import assert from "node:assert/strict";
import fs from "node:fs";
import test from "node:test";
import vm from "node:vm";

const source=fs.readFileSync(new URL("./static/app.js",import.meta.url),"utf8");
function load(ctx,name,async=false) {
  const start=source.indexOf(`${async?"async ":""}function ${name}(`),next=/\n(?:async )?function /.exec(source.slice(start+1));
  assert.ok(start>=0&&next,name);
  vm.runInContext(source.slice(start,start+1+next.index),ctx);
}
const settings=()=>({enabled:false,text:"Это рассылка",windowHours:24,supportButton:true,supportButtonText:"Поддержка",updatedAt:"settings-one"});
function setup() {
  const state={adminBroadcast:{status:"draft",updatedAt:"campaign-one"},adminBroadcastBusy:"",adminAudienceDraft:{telegram:{}},adminBroadcastButtonsDirty:false,adminBroadcastAutoReplyDraft:settings(),adminBroadcastAutoReplyDirty:false,adminBroadcastConfirmOpen:true};
  const ctx=vm.createContext({scheduleAdminBroadcastSave(){},flushAdminBroadcastAutoSave:async()=>{},discardAdminBroadcastAutoSave:async()=>{},adminBroadcastWorkflowMode:()=>"message",defaultAdminSurvey:()=>({enabled:false}),state,deepClone:structuredClone,canAdmin:()=>true,render(){},showToast(){},escapeHtml:String,escapeAttribute:String,icon:()=>"<svg></svg>"});
  for(const name of ["updateConsoleBroadcastAutoReply","setAdminBroadcastDraft","renderAdminBroadcastAutoReply"])load(ctx,name);
  load(ctx,"saveAdminBroadcastAutoReply",true);
  load(ctx,"ensureAdminBroadcastButtonsSaved",true);
  return ctx;
}

test("editing auto-reply invalidates confirmation and guards running campaigns, ranges and unknown fields",()=>{
  const ctx=setup();
  ctx.updateConsoleBroadcastAutoReply("enabled",true);
  assert.equal(ctx.state.adminBroadcastAutoReplyDraft.enabled,true);
  assert.equal(ctx.state.adminBroadcastConfirmOpen,false);
  assert.equal(ctx.state.adminBroadcastAutoReplyDirty,true);
  ctx.updateConsoleBroadcastAutoReply("windowHours",0);
  ctx.updateConsoleBroadcastAutoReply("windowHours",169);
  ctx.updateConsoleBroadcastAutoReply("untrusted",true);
  assert.equal(ctx.state.adminBroadcastAutoReplyDraft.windowHours,24);
  assert.equal(ctx.state.adminBroadcastAutoReplyDraft.untrusted,undefined);
  ctx.state.adminBroadcast.status="running";
  ctx.updateConsoleBroadcastAutoReply("text","changed");
  assert.equal(ctx.state.adminBroadcastAutoReplyDraft.text,"Это рассылка");
});

test("auto-reply settings have no manual save button, including when switched off",()=>{
  const ctx=setup();
  assert.doesNotMatch(ctx.renderAdminBroadcastAutoReply(false,false),/data-action="admin-broadcast-save-auto-reply"/);
  ctx.updateConsoleBroadcastAutoReply("enabled",true);
  assert.doesNotMatch(ctx.renderAdminBroadcastAutoReply(false,false),/data-action="admin-broadcast-save-auto-reply"/);
  ctx.updateConsoleBroadcastAutoReply("enabled",false);
  const html=ctx.renderAdminBroadcastAutoReply(false,false);
  assert.doesNotMatch(html,/<textarea/);
  assert.doesNotMatch(html,/data-action="admin-broadcast-save-auto-reply"/);
});

test("polling and draft reset preserve unsaved global auto-reply settings",()=>{
  const ctx=setup();
  ctx.updateConsoleBroadcastAutoReply("text","<b>Мой автоответ</b>");
  ctx.setAdminBroadcastDraft({status:"draft",updatedAt:"poll",autoReply:settings()});
  assert.equal(ctx.state.adminBroadcastAutoReplyDraft.text,"<b>Мой автоответ</b>");
  ctx.setAdminBroadcastDraft({status:"idle",autoReply:settings()},{forceButtons:true,forceExpiration:true});
  assert.equal(ctx.state.adminBroadcastAutoReplyDraft.text,"<b>Мой автоответ</b>");
  ctx.state.adminBroadcastAutoReplyDirty=false;
  ctx.setAdminBroadcastDraft({status:"idle",autoReply:settings()});
  assert.equal(ctx.state.adminBroadcastAutoReplyDraft.text,"Это рассылка");
});

test("preparing a broadcast saves the settings version without changing campaign state",async()=>{
  const ctx=setup();let request;
  ctx.updateConsoleBroadcastAutoReply("enabled",true);
  ctx.post=async(url,body)=>{request={url,body:structuredClone(body)};return {data:{...body,updatedAt:"settings-two"}}};
  await ctx.ensureAdminBroadcastButtonsSaved();
  assert.equal(request.url,"/api/mini-app/admin/broadcast/auto-reply");
  assert.equal(request.body.updatedAt,"settings-one");
  assert.equal(request.body.enabled,true);
  assert.equal(ctx.state.adminBroadcastAutoReplyDraft.updatedAt,"settings-two");
  assert.equal(ctx.state.adminBroadcastAutoReplyDirty,false);
  assert.equal(ctx.state.adminBroadcast.updatedAt,"campaign-one");
  assert.equal(ctx.state.adminBroadcastBusy,"");
});

test("a rejected settings save preserves edits and stops broadcast preparation",async()=>{
  const ctx=setup();
  ctx.updateConsoleBroadcastAutoReply("text","Мой текст");
  ctx.state.adminBroadcastButtonsDirty=true;
  ctx.post=async()=>{throw Error("settings changed")};
  ctx.saveAdminBroadcastButtons=()=>assert.fail("must not continue after failed settings save");
  await assert.rejects(ctx.ensureAdminBroadcastButtonsSaved(),/settings changed/);
  assert.equal(ctx.state.adminBroadcastAutoReplyDraft.text,"Мой текст");
  assert.equal(ctx.state.adminBroadcastAutoReplyDirty,true);
  assert.equal(ctx.state.adminBroadcastBusy,"");
});

test("a delayed poll cannot overwrite auto-reply settings saved after it started",()=>{
  const ctx=setup();
  ctx.state.adminBroadcastAutoReplyDraft={...settings(),text:"Saved text",updatedAt:"2026-10-26T16:00:01Z"};
  ctx.setAdminBroadcastDraft({status:"draft",autoReply:{...settings(),text:"Old text",updatedAt:"2026-10-26T16:00:00Z"}});
  assert.equal(ctx.state.adminBroadcastAutoReplyDraft.text,"Saved text");
  ctx.setAdminBroadcastDraft({status:"draft",autoReply:{...settings(),text:"Newer text",updatedAt:"2026-10-26T16:00:02Z"}});
  assert.equal(ctx.state.adminBroadcastAutoReplyDraft.text,"Newer text");
});

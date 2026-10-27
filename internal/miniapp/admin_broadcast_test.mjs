import test from "node:test";
import assert from "node:assert/strict";
import fs from "node:fs";
import vm from "node:vm";
import {broadcastMetrics,broadcastButtonText,BROADCAST_BUTTON_TYPES} from "../../admin-ui/broadcast-data.mjs";
const source=fs.readFileSync(new URL("./static/app.js",import.meta.url),"utf8");
const refreshCode=source.slice(source.indexOf("let adminTelegramStatePending="),source.indexOf("async function saveAdminEmailBroadcast("));
function refreshContext(state){
 let renders=0;
 const ctx=vm.createContext({state,canAdmin:()=>true,adminBroadcastWorkflowMode:()=>state.adminBroadcastMode||"message",loadAdminSurveyResults(){state.adminSurveyResultsOpen=true},loadAdminSMTP(){},deepClone:structuredClone,render(){renders++},post:async()=>({data:{}}),setAdminBroadcastDraft(d){state.adminBroadcast=d}});
 vm.runInContext(refreshCode,ctx);return{ctx,get renders(){return renders}};
}
test("broadcast metrics show delivery errors and handle empty or excessive progress",()=>{
 assert.deepEqual(broadcastMetrics({recipientCount:100,sentCount:85,failedCount:15}),{total:100,sent:85,failed:15,percent:100,errorPercent:15});
 assert.equal(broadcastMetrics({recipientCount:10,sentCount:40}).percent,100);
 assert.equal(broadcastMetrics().errorPercent,0);
 assert.equal(BROADCAST_BUTTON_TYPES.length,8);
 assert.equal(broadcastButtonText('<tg-emoji emoji-id="123456789">🎉</tg-emoji> Привет'),"🎉 Привет");
});
test("a delayed Telegram poll cannot replace a more recently saved draft or clear another operation",async()=>{
 const state={adminSection:"broadcast",adminBroadcastTab:"telegram",adminBroadcastBusy:"",adminBroadcast:{updatedAt:"one"}};
 const {ctx}=refreshContext(state);let resolve,calls=0;ctx.post=()=>{calls++;return new Promise(r=>resolve=r)};
 const pending=ctx.refreshAdminBroadcast({silent:true});await ctx.refreshAdminBroadcast({silent:true});assert.equal(calls,1);
 state.adminBroadcast={updatedAt:"two",sourceText:"Новое сообщение"};state.adminBroadcastBusy="preview";
 resolve({data:{updatedAt:"one",sourceText:"Старое сообщение"}});await pending;
 assert.equal(state.adminBroadcast.sourceText,"Новое сообщение");assert.equal(state.adminBroadcastBusy,"preview");
});
test("unchanged silent email refresh preserves unsaved input and does not replay the page",async()=>{
 const draft={updatedAt:"one",subject:"Saved",body:"Saved body"};
 const state={adminSection:"broadcast",adminBroadcastTab:"email",adminEmailEditorTab:"message",adminEmailBusy:"",adminEmailBroadcast:{draft},adminEmailDraftDirty:true,adminEmailDraftSubject:"Unsaved",adminEmailDraftBody:"Unsaved body",adminAudienceDraft:{}};
 const check=refreshContext(state);check.ctx.post=async()=>({data:{draft:structuredClone(draft)}});
 await check.ctx.refreshAdminEmailBroadcast({silent:true});assert.equal(check.renders,0);assert.equal(state.adminEmailDraftSubject,"Unsaved");assert.equal(state.adminEmailDraftBody,"Unsaved body");
});
test("a rejected refresh releases its request guard so retry can load the draft",async()=>{
 const state={adminSection:"broadcast",adminBroadcastTab:"telegram",adminBroadcastBusy:"",adminBroadcast:null};const {ctx}=refreshContext(state);
 ctx.post=async()=>{throw Error("offline")};await ctx.refreshAdminBroadcast();assert.equal(state.adminBroadcastError,"offline");assert.equal(state.adminBroadcastBusy,"");
 ctx.post=async()=>({data:{updatedAt:"new"}});await ctx.refreshAdminBroadcast();assert.equal(state.adminBroadcast.updatedAt,"new");assert.equal(state.adminBroadcastError,"");
});

test("opening a saved survey loads its inline answers once without replacing the composer",async()=>{
 const state={adminSection:"broadcast",adminBroadcastTab:"telegram",adminBroadcastMode:"survey",adminBroadcastBusy:"",adminBroadcast:null};
 const{ctx}=refreshContext(state);let loads=0;
 ctx.loadAdminSurveyResults=()=>{loads++;state.adminSurveyResultsOpen=true};
 ctx.post=async()=>({data:{updatedAt:"survey",survey:{enabled:true}}});
 await ctx.refreshAdminBroadcast();await ctx.refreshAdminBroadcast({silent:true});
 assert.equal(loads,1);assert.equal(state.adminBroadcastMode,"survey");assert.equal(state.adminBroadcastError,"");
});
test("applying a Telegram template uses the saved nonce and only restores state",async()=>{
 const state={adminBroadcastTab:"telegram",adminBroadcast:{updatedAt:"one"},adminAudienceDraft:{},adminAudienceCount:{telegram:18},adminBroadcastConfirmOpen:true,adminEmailConfirmOpen:true};let saved=0,payload;
 const ctx=vm.createContext({state,canAdmin:()=>true,ensureAdminBroadcastButtonsSaved:async()=>{saved++;state.adminBroadcast.updatedAt="saved"},currentBroadcastAudience:()=>({category:"active"}),deepClone:structuredClone,render(){},setAdminBroadcastDraft(d){state.adminBroadcast=d},post:async(url,body)=>{payload={url,body};return{data:{updatedAt:"restored",audience:{category:"trial"}}}}});
 vm.runInContext(source.slice(source.indexOf("async function consoleBroadcastTemplates("),source.indexOf("function setAdminBroadcastDraft(")),ctx);
 await ctx.consoleBroadcastTemplates("apply",{id:7});assert.equal(saved,1);assert.equal(payload.url,"/api/mini-app/admin/broadcast/templates");assert.equal(payload.body.updatedAt,"saved");assert.equal(payload.body.id,7);assert.equal(state.adminBroadcast.updatedAt,"restored");assert.equal(state.adminAudienceDraft.telegram.category,"trial");assert.equal(state.adminAudienceCount.telegram,null);assert.equal(state.adminBroadcastConfirmOpen,false);assert.equal(state.adminEmailConfirmOpen,false);
});

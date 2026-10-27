import test from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';
import vm from 'node:vm';
import {createBroadcastAutosave} from './static/broadcast-autosave.mjs';
const source=fs.readFileSync(new URL('./static/app.js',import.meta.url),'utf8');
function load(context,name){const start=source.indexOf(`function ${name}(`),end=source.slice(start+1).search(/\n(?:async )?function /);vm.runInContext(source.slice(start,start+1+end),context);}
for(const initial of [0,720])test(`timer reversal during a delayed save keeps the last choice (${initial})`,async()=>{
 const state={adminBroadcast:{status:'draft',updatedAt:'v0',deleteAfterMinutes:initial},adminBroadcastExpirationDraft:initial,adminAudienceDraft:{}};
 let resolveFirst;const first=new Promise(done=>{resolveFirst=done;}),requests=[];
 let queue;
 const c=vm.createContext({state,canAdmin:()=>true,deepClone:structuredClone,render(){},scheduleAdminBroadcastSave:key=>queue.schedule(key)});
 for(const name of ['defaultAdminSurvey','setAdminBroadcastDraft','updateConsoleBroadcastExpiration'])load(c,name);
 queue=createBroadcastAutosave({state,applyTelegramDraft:c.setAdminBroadcastDraft,setTimer:()=>1,clearTimer(){},post:async(path,body)=>{requests.push(structuredClone(body));if(requests.length===1)await first;return{data:{status:'draft',updatedAt:`v${requests.length}`,deleteAfterMinutes:body.deleteAfterMinutes}};}});
 c.updateConsoleBroadcastExpiration(initial?0:720);const saving=queue.flush();
 c.updateConsoleBroadcastExpiration(initial);assert.equal(state.adminBroadcastExpirationDirty,true);
 resolveFirst();await saving;
 assert.deepEqual(requests.map(r=>r.deleteAfterMinutes),[initial?0:720,initial]);
 assert.equal(state.adminBroadcastExpirationDraft,initial);assert.equal(state.adminBroadcast.deleteAfterMinutes,initial);assert.equal(state.adminBroadcastExpirationDirty,false);
});

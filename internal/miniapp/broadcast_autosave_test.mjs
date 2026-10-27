import test from 'node:test';
import assert from 'node:assert/strict';
import { createBroadcastAutosave } from './static/broadcast-autosave.mjs';

function deferred() { let resolve; const promise = new Promise(done => { resolve = done; }); return { promise, resolve }; }
function harness() {
  const requests=[],errors=[],timers=new Map();let id=0,version=0;
  const state={adminBroadcast:{status:'draft',updatedAt:'v0'},adminEmailBroadcast:{draft:{status:'draft'}},adminAudienceDraft:{telegram:{category:'all'},email:{category:'active'}},adminAudienceDirty:{},adminAudienceCount:{telegram:null,email:null},adminBroadcastButtonsDraft:[],adminBroadcastAutoReplyDraft:{enabled:true,text:'Ответ',windowHours:24,updatedAt:'reply0'}};
  const h={state,requests,errors,timers,reply:async(path,body)=>({data:path.endsWith('/audience')?{draft:{status:'draft',updatedAt:`v${++version}`},count:12}:path.endsWith('/auto-reply')?{...body,updatedAt:`reply${++version}`}:{status:'draft',updatedAt:`v${++version}`}})};
  h.queue=createBroadcastAutosave({state,post:async(path,body)=>{requests.push({path,body:structuredClone(body)});return h.reply(path,body);},applyTelegramDraft:draft=>{state.adminBroadcast=draft;},onError:e=>errors.push(e.message),setTimer:fn=>{timers.set(++id,fn);return id;},clearTimer:key=>timers.delete(key)});
  return h;
}
test('rapid changes coalesce and preserve the current draft nonce across different settings',async()=>{
  const h=harness(),s=h.state;
  s.adminBroadcastExpirationDraft=60;s.adminBroadcastExpirationDirty=true;h.queue.schedule('expiration');
  s.adminBroadcastExpirationDraft=120;h.queue.schedule('expiration');
  s.adminAudienceDirty.telegram=true;h.queue.schedule('audience:telegram');
  assert.equal(h.timers.size,1);await h.queue.flush();
  assert.equal(h.requests.length,2);assert.equal(h.requests[0].body.deleteAfterMinutes,120);assert.equal(h.requests[0].body.updatedAt,'v0');assert.equal(s.adminAudienceCount.telegram,12);assert.equal(s.adminBroadcastAutoSaving,false);
});
test('editing during a slow save keeps the new value and writes it with the acknowledged version',async()=>{
  const h=harness(),s=h.state,first=deferred();
  s.adminBroadcastExpirationDraft=60;s.adminBroadcastExpirationDirty=true;
  h.reply=async()=>h.requests.length===1?first.promise:{data:{status:'draft',updatedAt:'v2'}};
  h.queue.schedule('expiration');const saving=h.queue.flush();
  s.adminBroadcastExpirationDraft=90;s.adminBroadcastExpirationDirty=true;h.queue.schedule('expiration');
  first.resolve({data:{status:'draft',updatedAt:'v1'}});await saving;
  assert.deepEqual(h.requests.map(r=>[r.body.deleteAfterMinutes,r.body.updatedAt]),[[60,'v0'],[90,'v1']]);assert.equal(s.adminBroadcastExpirationDraft,90);assert.equal(s.adminBroadcastExpirationDirty,false);
});
test('a stale recipient count cannot replace the count for newer filters',async()=>{
  const h=harness(),s=h.state,first=deferred();let applied=0;
  s.adminAudienceDirty.telegram=true;h.reply=async()=>++applied===1?first.promise:{data:{draft:{status:'draft',updatedAt:'v2'},count:3}};
  h.queue.schedule('audience:telegram');const saving=h.queue.flush();
  s.adminAudienceDraft.telegram={category:'expired'};s.adminAudienceDirty.telegram=true;h.queue.schedule('audience:telegram');
  first.resolve({data:{draft:{status:'draft',updatedAt:'v1'},count:100}});await saving;
  assert.equal(h.requests[1].body.audience.category,'expired');assert.equal(s.adminAudienceCount.telegram,3);assert.equal(s.adminAudienceDirty.telegram,false);
});
test('failed writes preserve dirty data and block a flush until a successful retry',async()=>{
  const h=harness(),s=h.state;s.adminBroadcastExpirationDraft=240;s.adminBroadcastExpirationDirty=true;
  h.reply=async()=>{throw Error('network unavailable');};h.queue.schedule('expiration');
  await assert.rejects(h.queue.flush(),/network unavailable/);assert.equal(s.adminBroadcastExpirationDirty,true);assert.equal(s.adminBroadcastAutoSaving,false);assert.equal(h.errors.length,1);
  h.reply=async()=>({data:{status:'draft',updatedAt:'retry'}});await h.queue.flush();assert.equal(s.adminBroadcastExpirationDirty,false);assert.equal(s.adminBroadcastAutoSaveError,'');
});
test('partially entered buttons wait for complete fields, then save without an extra action',async()=>{
  const h=harness(),s=h.state;s.adminBroadcastButtonsDraft=[{type:'url',text:'Открыть',url:'https://'}];s.adminBroadcastButtonsDirty=true;
  h.queue.schedule('buttons');await h.queue.flush();assert.equal(h.requests.length,0);assert.equal(s.adminBroadcastButtonsDirty,true);
  s.adminBroadcastButtonsDraft[0].url='https://example.com';h.queue.schedule('buttons');await h.queue.flush();assert.equal(h.requests.length,1);assert.equal(s.adminBroadcastButtonsDirty,false);
});
test('a rejected button does not prevent independent recipient settings from saving',async()=>{
  const h=harness(),s=h.state;s.adminBroadcastButtonsDraft=[{type:'promo',text:'Бонус',promoCode:'UNKNOWN'}];s.adminBroadcastButtonsDirty=true;s.adminAudienceDirty.telegram=true;
  h.reply=async(path)=>{if(path.endsWith('/buttons'))throw Error('unknown promo');return{data:{draft:{status:'draft',updatedAt:'v1'},count:7}};};
  h.queue.schedule('buttons');h.queue.schedule('audience:telegram');await assert.rejects(h.queue.flush(),/unknown promo/);
  assert.equal(s.adminBroadcastButtonsDirty,true);assert.equal(s.adminAudienceDirty.telegram,false);assert.equal(s.adminAudienceCount.telegram,7);assert.equal(h.errors.length,1);
});
test('partial survey drafts persist before capturing a new question and get a default name',async()=>{
 const h=harness(),s=h.state;s.adminSurveyDraft={enabled:true,startText:'Пройти',introText:'Вступление',finishText:'Спасибо',questions:[{text:'',options:['Да']}]};s.adminSurveyDirty=true;
 h.queue.schedule('survey');await h.queue.flush();assert.equal(h.requests.length,1);assert.equal(h.requests[0].body.draft,true);assert.equal(h.requests[0].body.survey.questions[0].text,'');assert.equal(h.requests[0].body.survey.title,'Опрос');
 s.adminSurveyDraft.questions[0].text='Нравится?';s.adminSurveyDirty=true;h.queue.schedule('survey');await h.queue.flush();assert.equal(h.requests[1].body.survey.questions[0].text,'Нравится?');assert.equal(s.adminSurveyDirty,false);
});

test('auto-reply edits during a request retain their text and use the next settings version',async()=>{
  const h=harness(),s=h.state,first=deferred();s.adminBroadcastAutoReplyDirty=true;
  h.reply=async()=>h.requests.length===1?first.promise:{data:{...h.requests.at(-1).body,updatedAt:'reply2'}};
  h.queue.schedule('auto-reply');const saving=h.queue.flush();
  s.adminBroadcastAutoReplyDraft.text='Новый ответ';h.queue.schedule('auto-reply');first.resolve({data:{enabled:true,text:'Ответ',windowHours:24,updatedAt:'reply1'}});await saving;
  assert.equal(h.requests[1].body.text,'Новый ответ');assert.equal(h.requests[1].body.updatedAt,'reply1');assert.equal(s.adminBroadcastAutoReplyDraft.text,'Новый ответ');assert.equal(s.adminBroadcastAutoReplyDirty,false);
});
test('email and Telegram recipients save independently and discarding waits for the current request',async()=>{
  const h=harness(),s=h.state,first=deferred();s.adminAudienceDirty.telegram=true;s.adminAudienceDirty.email=true;
  h.reply=async()=>first.promise;h.queue.schedule('audience:telegram');const saving=h.queue.flush();h.queue.schedule('audience:email');
  const discarded=h.queue.discard();first.resolve({data:{draft:{status:'draft',updatedAt:'v1'},count:5}});await Promise.all([saving,discarded]);
  assert.equal(h.requests.length,1);assert.equal(s.adminAudienceDirty.email,true);assert.equal(h.timers.size,0);
});

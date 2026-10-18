import test from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';
import vm from 'node:vm';

const source=fs.readFileSync(new URL('./static/app.js',import.meta.url),'utf8');
function load(context,name) {
 const re=new RegExp(`(?:async )?function ${name}\\(`),start=source.search(re);
 const tail=source.slice(start+1),next=tail.search(/\n(?:async )?function /);
 assert.ok(start>=0&&next>=0,name);vm.runInContext(source.slice(start,start+1+next),context);
}
const escape=value=>String(value).replaceAll('<','&lt;').replaceAll('>','&gt;');

test('pin editor escapes captured text and enables only valid actions',()=>{
 const state={adminPinnedMessage:null,adminPinnedBusy:'',adminPinnedError:''};
 const c=vm.createContext({state,escapeHtml:escape,icon:()=>''});load(c,'renderAdminPinnedMessage');
 assert.match(c.renderAdminPinnedMessage(),/data-input="admin-pinned-enabled"[^>]*disabled/);
 state.adminPinnedMessage={enabled:true,hasMessage:true,total:10,completed:8,failed:2,sourcePreview:'<script>alert(1)</script>'};
 let html=c.renderAdminPinnedMessage();assert.match(html,/&lt;script&gt;/);assert.doesNotMatch(html,/<script>/);
 assert.match(html,/Получили при \/start: 8/);assert.doesNotMatch(html,/admin-pinned-retry/);assert.doesNotMatch(html,/role="progressbar"/);
 assert.match(html,/data-input="admin-pinned-enabled"[^>]*checked/);
 state.adminPinnedMessage.awaitingMessage=true;html=c.renderAdminPinnedMessage();assert.match(html,/admin-pinned-cancel/);assert.match(html,/admin-pinned-capture" disabled/);
 assert.match(html,/при следующем \/start/);assert.match(html,/Массовой отправки нет/);
 state.adminPinnedMessage.enabled=false;html=c.renderAdminPinnedMessage();assert.match(html,/aria-valuenow="8"/);assert.match(html,/admin-pinned-retry/);
});

test('pin toggle uses current version and preserves unsaved content and scroll',async()=>{
 const settings={content:{startTextRu:'unsaved'}},requests=[],renders=[];
 const state={adminPinnedMessage:{revision:3,enabled:false},adminPinnedBusy:'',adminSettingsDraft:settings};
 const c=vm.createContext({state,adminPinnedMutationGeneration:0,render:options=>renders.push(options),showToast(){},async post(path,body){requests.push({path,body});return{data:{revision:4,enabled:true}};}});
 load(c,'actAdminPinnedMessage');await c.actAdminPinnedMessage('toggle',true);
 assert.equal(requests[0].body.revision,3);assert.equal(requests[0].body.enabled,true);assert.equal(state.adminPinnedMessage.enabled,true);
 assert.equal(state.adminSettingsDraft,settings);assert.ok(renders.every(o=>o.preserveScroll&&o.preserveInteraction));assert.equal(state.adminPinnedBusy,'');
});

test('an older poll cannot undo a completed toggle',async()=>{
 let resolvePoll;const state={currentPage:'admin',adminSection:'content',adminContentSection:'start',adminPinnedMessage:{revision:3,enabled:false},adminPinnedBusy:''};
 let requests=0;
 const c=vm.createContext({state,adminPinnedRequestPending:false,adminPinnedMutationGeneration:0,render(){},showToast(){},post(){requests++;if(requests===1)return new Promise(resolve=>resolvePoll=resolve);return Promise.resolve({data:{revision:4,enabled:true}});}});
 load(c,'refreshAdminPinnedMessage');load(c,'actAdminPinnedMessage');
 const polling=c.refreshAdminPinnedMessage();await c.actAdminPinnedMessage('toggle',true);resolvePoll({data:{revision:3,enabled:false}});await polling;
 assert.equal(state.adminPinnedMessage.revision,4);assert.equal(state.adminPinnedMessage.enabled,true);
});

test('message capture returns to bot chat and preview never toggles delivery',async()=>{
 let closed=0,opened=0;const requests=[];
 const state={adminPinnedMessage:{revision:1},adminPinnedBusy:'',data:{meta:{botUrl:'https://t.me/test_bot'}}};
 const c=vm.createContext({state,adminPinnedMutationGeneration:0,tg:{close(){closed++;}},openExternal(){opened++;},render(){},showToast(){},async post(path){requests.push(path);return{data:{revision:1,awaitingMessage:true}};}});
 load(c,'openPinnedMessageChat');load(c,'actAdminPinnedMessage');await c.actAdminPinnedMessage('capture');assert.equal(closed,1);
 await c.actAdminPinnedMessage('preview');assert.equal(requests.length,2);assert.ok(requests[1].endsWith('/preview'));assert.equal(closed,1);
 c.tg=null;c.openPinnedMessageChat();assert.equal(opened,1);
});

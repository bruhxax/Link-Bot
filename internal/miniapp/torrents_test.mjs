import test from 'node:test';import assert from 'node:assert/strict';
import {createTorrentController,renderTorrentWorkspace,torrentTemplatePreview,torrentReportStatus} from './static/torrents.mjs';
import {createTorrentAutosave} from './static/torrent-autosave.mjs';
const deferred=()=>{let resolve,reject;const promise=new Promise((a,b)=>{resolve=a;reject=b});return {promise,resolve,reject};};
test('report requests ignore old filters and preserve pagination on failure',async()=>{
 const calls=[];const c=createTorrentController({post:(url,body)=>{const task=deferred();calls.push({body,...task});return task.promise;},render:()=>{}});
 const old=c.load();c.model.query='alice';const fresh=c.load();calls[1].resolve({data:{items:[{id:9}],hasMore:true}});await fresh;calls[0].resolve({data:{items:[{id:1}]}});await old;assert.equal(c.model.data.items[0].id,9);
 const more=c.load({more:true});assert.equal(calls[2].body.beforeId,9);calls[2].resolve({data:{items:[{id:9},{id:8}],hasMore:false}});await more;assert.deepEqual(c.model.data.items.map(x=>x.id),[9,8]);
 const failure=c.load();calls[3].reject(Error('offline'));await failure;assert.equal(c.model.error,'offline');assert.equal(c.model.busy,false);assert.equal(c.model.data.items.length,2);
});
test('leaving the page discards data and always releases loading state',async()=>{
 let active=true;const task=deferred();const c=createTorrentController({post:()=>task.promise,render:()=>{},isActive:()=>active});const loading=c.load();active=false;task.resolve({data:{items:[{id:1}]}});await loading;assert.equal(c.model.busy,false);assert.equal(c.model.data,null);
});
test('cards escape panel text, retain IPv6, and show only permitted user links',()=>{
 const c=createTorrentController({post:()=>{},render:()=>{}});c.model.data={items:[{id:1,username:'<script>x</script>',userUuid:'uuid',node:'Node <x>',ip:'2001:db8::1',blocked:true,duration:3600,unblockAt:'2099-01-01T00:00:00Z',occurredAt:'2026-10-28T20:00:00Z',customerId:42}],summary:{total:1},webhookConfigured:true};
 let html=renderTorrentWorkspace(c.model,{},{});assert.match(html,/&lt;script&gt;/);assert.match(html,/2001:db8::1/);assert.doesNotMatch(html,/<script>|torrent-open-user/);
 html=renderTorrentWorkspace(c.model,{}, {canUsers:true});assert.match(html,/torrent-open-user/);assert.match(html,/1 ч\./);
 assert.equal(torrentReportStatus({blocked:true,unblockAt:'2000-01-01'}),'released');assert.equal(torrentReportStatus({blocked:false}),'observed');
});
test('two independent template editors expose IP only to the admin and never execute panel data',()=>{
 const model={tab:'templates',data:null};const html=renderTorrentWorkspace(model,{userText:'Нода {node}',adminText:'IP {ip}'},{preview:text=>text});
 assert.match(html,/id="torrent-user-text"/);assert.match(html,/id="torrent-admin-text"/);assert.doesNotMatch(html,/data-kind="user" data-value="ip"/);assert.match(html,/data-kind="admin" data-value="ip"/);
 assert.match(torrentTemplatePreview('{ip}',true,text=>text),/\{ip\}/);assert.equal(torrentTemplatePreview('{duration}',true,text=>text),'1 ч.');
});
test('torrent workspace keeps the setup below metrics and has no manual template save',()=>{
 const model={tab:'templates',data:{summary:{},items:[],webhookConfigured:true}};
 const html=renderTorrentWorkspace(model,{userText:'{node}',adminText:'{ip}'},{preview:text=>text});
 assert.ok(html.indexOf('tp-metrics')<html.indexOf('tp-setup')&&html.indexOf('tp-setup')<html.indexOf('tp-tabs'));
 assert.doesNotMatch(html,/КОНТРОЛЬ ТРАФИКА|Сообщения о блокировке|Сохранить шаблоны|tp-hero/);
 assert.match(html,/События и блокировки/);assert.match(html,/Тексты сообщений/);
 assert.match(html,/data-torrent-save-status/);
});
test('torrent settings autosave keeps the latest edit during an in-flight request',async()=>{
 const first=deferred(),saved=[],timers=new Map();let id=0;
 const autosave=createTorrentAutosave({save:async value=>{saved.push(value);if(value==='first')await first.promise;},setTimer:fn=>{timers.set(++id,fn);return id;},clearTimer:key=>timers.delete(key)});
 autosave.schedule('first');const saving=autosave.flush();assert.equal(autosave.state.phase,'saving');
 autosave.schedule('latest');first.resolve();await saving;assert.equal(autosave.state.phase,'pending');
 const queued=[...timers.values()];timers.clear();queued.at(-1)();await Promise.resolve();await Promise.resolve();
 assert.deepEqual(saved,['first','latest']);assert.equal(autosave.state.phase,'saved');
});
test('torrent settings autosave reports validation failure and retries after editing',async()=>{
 let calls=0;const autosave=createTorrentAutosave({save:async()=>{if(++calls===1)throw Error('Некорректный HTML');},setTimer:()=>1,clearTimer:()=>{}});
 autosave.schedule('bad');await autosave.flush();assert.equal(autosave.state.phase,'error');assert.equal(autosave.state.error,'Некорректный HTML');
 autosave.schedule('good');await autosave.flush();assert.equal(autosave.state.phase,'saved');assert.equal(calls,2);
});

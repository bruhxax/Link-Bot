import test from 'node:test';import assert from 'node:assert/strict';
import {createTorrentController,renderTorrentWorkspace,torrentTemplatePreview,torrentReportStatus} from './static/torrents.mjs';
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

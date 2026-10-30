import test from 'node:test';
import assert from 'node:assert/strict';
import {createAbuseController,renderAbuseWorkspace} from './static/abuses.mjs';

test('expanded evidence escapes panel data and hides actions without permission',()=>{
 const c=createAbuseController({post:async()=>{},render:()=>{}});
 const item={key:'sharing:42',kind:'sharing',username:'<script>x</script>',customerId:7,subscriptionId:9,deviceLimit:1,ips:[{ip:'2001:db8::1',node:'<b>node</b>'},{ip:'203.0.113.5',node:'B'}]};
 c.model.data={items:[item],checkedAt:'2026-10-30T00:00:00Z'};c.model.open.add(item.key);
 let html=renderAbuseWorkspace(c.model,{});
 assert.match(html,/2001:db8::1/);assert.match(html,/&lt;script&gt;/);assert.match(html,/&lt;b&gt;node&lt;\/b&gt;/);
 assert.doesNotMatch(html,/<script>|data-action="abuse-act"/);
 html=renderAbuseWorkspace(c.model,{canBlock:true,canRevoke:true});
 assert.match(html,/Заблокировать/);assert.match(html,/Отозвать подписку/);
});

test('trial actions target every matching subscription and require confirmation',async()=>{
 const calls=[];const c=createAbuseController({post:async(url,body)=>{calls.push({url,body});return {data:{items:[],scanning:false}};},render:()=>{}});
 const item={key:'trial:hwid',kind:'trial',username:'latest',customerId:2,hwid:'hwid',subscriptions:[{customerId:1,subscriptionId:11,username:'first'},{customerId:2,subscriptionId:22,username:'latest'}]};
 c.model.data={items:[item]};c.model.open.add(item.key);
 let html=renderAbuseWorkspace(c.model,{canBlock:true,canRevoke:true});
 assert.match(html,/Отозвать все подписки/);assert.match(html,/Отозвать последнюю подписку/);
 await c.act(item.key,'all');assert.equal(calls.length,0);
 await c.act(item.key,'all');assert.deepEqual(calls.filter(call=>call.url.endsWith('/delete')).map(call=>call.body.subscriptionId),[11,22]);
});

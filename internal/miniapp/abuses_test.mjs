import test from 'node:test';
import assert from 'node:assert/strict';
import {createAbuseController,renderAbuseWorkspace,applyAbuseFilters} from './static/abuses.mjs';

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

test('compact workspace omits the extra heading, refresh control, and card icons',()=>{
 const c=createAbuseController({post:async()=>{},render:()=>{}});
 c.model.data={items:[{key:'trial:one',kind:'trial',username:'alex',hwid:'device',subscriptions:[{customerId:1,subscriptionId:2,username:'alex'}]}]};
 const html=renderAbuseWorkspace(c.model,{});
 assert.match(html,/>Всего</);assert.match(html,/>Шаринг</);assert.match(html,/>Триал</);
 assert.doesNotMatch(html,/БЕЗОПАСНОСТЬ ПОДПИСОК|Подозрительные подключения и повторные пробные периоды|abuse-refresh|ab-card__glyph|<svg/);
});

test('search and expansion update existing cards without rebuilding the workspace',()=>{
 let renders=0;
 const c=createAbuseController({post:async()=>{},render:()=>{renders++}});
 c.model.data={items:[{key:'sharing:one',kind:'sharing',username:'alex',ips:[{ip:'203.0.113.1'}]},{key:'trial:two',kind:'trial',username:'maria',hwid:'device-42'}]};
 const head={setAttribute(name,value){this[name]=value}};
 const label={textContent:'Подробнее'};
 const details={inert:true,removeAttribute(){this.inert=false},setAttribute(){this.inert=true}};
 const inside={appended:false,append(){this.appended=true},offsetHeight:0};
 const template={content:{cloneNode(){return {}}},remove(){this.removed=true}};
 const card={dataset:{abuseKey:'trial:two'},hidden:false,open:false,classList:{toggle(name,value){card.open=value}},querySelector(selector){return selector==='.ab-card__head'?head:selector==='.ab-card__expand'?details:selector==='.ab-card__inside'?inside:selector==='.ab-card__template'?template:label}};
 const root={querySelectorAll(selector){return selector==='[data-abuse-key]'?[card]:[]},querySelector(){return null}};
 c.toggle('trial:two',root);
 assert.equal(renders,0);assert.equal(card.open,true);assert.equal(head['aria-expanded'],'true');assert.equal(label.textContent,'Свернуть');assert.equal(details.inert,false);assert.equal(inside.appended,true);
 c.model.query='alex';applyAbuseFilters(root,c.model);
 assert.equal(card.hidden,true);assert.equal(renders,0);
});

test('background polling does not redraw an unchanged list',async()=>{
 let renders=0;
 const c=createAbuseController({post:async()=>({data:{items:[],checkedAt:'2026-10-30T08:00:00Z',scanning:false}}),render:()=>{renders++}});
 await c.load();
 const first=renders;
 await c.load({silent:true});
 assert.equal(renders,first);
});

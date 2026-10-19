import test from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';
import vm from 'node:vm';
const source=fs.readFileSync(new URL('./static/app.js',import.meta.url),'utf8');
function harness(){
 const state={currentPage:'admin',adminSection:'status',adminStatus:{canUpdate:true,updater:{available:true,state:'idle'}},adminBotUpdateConfirm:false};
 const calls=[],timers=[],toasts=[];
 const c=vm.createContext({state,calls,timers,toasts,adminStatusPollTimer:0,window:{clearTimeout(){},setTimeout(fn,delay){timers.push({fn,delay});return timers.length}},render(){},showToast(...args){toasts.push(args)},escapeHtml:s=>String(s).replaceAll('<','&lt;'),icon:()=>'<svg></svg>',post:async(url,body)=>{calls.push({url,body});return {data:{available:true,state:'running',stage:'Сборка новой версии'}}}});
 for(const name of ['renderAdminBotUpdater','scheduleAdminStatusPoll','startAdminBotUpdate','refreshAdminBotUpdate']){
  const start=source.search(new RegExp(`(?:async )?function ${name}\\(`));const rest=source.slice(start+1);const end=start+1+rest.search(/\n(?:async )?function /);assert.ok(start>=0&&end>start,name);vm.runInContext(source.slice(start,end),c);
 }
 c.refreshAdminStatus=async()=>{calls.push({full:true})};return c;
}

test('update controls belong to owner; missing agent explains initial setup',()=>{
 const c=harness();assert.equal(c.renderAdminBotUpdater({canUpdate:false}),'');
 let markup=c.renderAdminBotUpdater(c.state.adminStatus);assert.match(markup,/Обновить бота/);assert.doesNotMatch(markup,/disabled/);
 c.state.adminStatus.updater={available:false,message:'Один раз bash ./update.sh'};markup=c.renderAdminBotUpdater(c.state.adminStatus);assert.match(markup,/disabled/);assert.match(markup,/bash/);
 c.state.adminStatus.updater={available:true,state:'running',stage:'<script>'};markup=c.renderAdminBotUpdater(c.state.adminStatus);assert.match(markup,/disabled/);assert.match(markup,/&lt;script>/);
});

test('confirmation is required and double clicks cannot create concurrent jobs',async()=>{
 const c=harness();await c.startAdminBotUpdate();assert.equal(c.calls.length,0);
 c.state.adminBotUpdateConfirm=true;const pending=c.startAdminBotUpdate();await c.startAdminBotUpdate();await pending;
 assert.equal(c.calls.length,1);assert.equal(c.calls[0].url,'/api/mini-app/admin/bot/update');assert.equal(c.calls[0].body.confirm,true);assert.equal(c.state.adminBotUpdateConfirm,false);assert.equal(c.timers.at(-1).delay,3000);
 c.state.adminBotUpdateConfirm=true;await c.startAdminBotUpdate();assert.equal(c.calls.length,1);
});

test('restart errors retain progress and use a lightweight polling endpoint',async()=>{
 const c=harness();c.state.adminStatus.updater={available:true,state:'running',stage:'Перезапуск бота'};c.post=async()=>{throw Error('network lost')};
 await c.refreshAdminBotUpdate();assert.equal(c.state.adminStatus.updater.state,'running');assert.equal(c.state.adminBotUpdateConnecting,true);assert.equal(c.toasts.length,0);assert.equal(c.timers.at(-1).delay,3000);
 c.post=async(url)=>{c.calls.push(url);return {data:{available:true,state:'completed',message:'Готово'}}};await c.refreshAdminBotUpdate();
 assert.equal(c.calls[0],'/api/mini-app/admin/bot/update/status');assert.equal(c.calls[1].full,true);assert.equal(c.state.adminBotUpdateConnecting,false);assert.equal(c.timers.at(-1).delay,60000);
});

test('leaving status page stops update polling',async()=>{
 const c=harness();c.state.adminSection='plans';c.scheduleAdminStatusPoll();await c.refreshAdminBotUpdate();assert.equal(c.calls.length,0);assert.equal(c.timers.length,0);
});

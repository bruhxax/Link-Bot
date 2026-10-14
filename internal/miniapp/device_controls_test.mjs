import test from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';
import vm from 'node:vm';
import {deviceClientName} from './static/device-client.mjs';

const source = fs.readFileSync(new URL('./static/app.js', import.meta.url), 'utf8');
test('device rows show client name without platform or version details', () => {
  assert.equal(deviceClientName('Happ/5.8.0/ios/2609081608681'), 'Happ');
  assert.equal(deviceClientName('INCY/2.6.2/ios CFNetwork/3896.100.1.2 Darwin/25'), 'INCY');
  assert.equal(deviceClientName('v2rayTun/3.5.0'), 'v2rayTun');
  assert.equal(deviceClientName(''), '');
});

test('administrator quotas hide only limit editor, keeping pause/delete/unblock', () => {
  const context = vm.createContext({state:{data:{subscription:{deviceManagement:true}},deviceBusyHwid:'',deviceLimitEditHwid:'device',deviceDeleteConfirmHwid:''},
    deviceText:()=>({added:'Добавлено',delete:'Удалить'}),getDeviceTitle:()=> 'iPhone', getDeviceSecondary:()=> 'Happ',formatDeviceCreatedAt:()=> '14.10.26',
    localizedText:ru=>ru,escapeHtml:String,escapeAttribute:String,icon:name=>name,renderDeviceTrafficProgress:()=>'<div>7/20 GB</div>'});
  const start=source.indexOf('function renderDeviceRow(');
  vm.runInContext(source.slice(start,source.indexOf('function renderSupportComposerModal(',start)),context);
  const admin=context.renderDeviceRow({hwid:'device',canSetTrafficLimit:false});
  assert.match(admin,/device-toggle/); assert.match(admin,/delete-device/);
  assert.doesNotMatch(admin,/device-edit-limit|device-limit-gb/);
  assert.match(context.renderBlockedDeviceRow({hwid:'device'}),/device-unblock/);
  const personal=context.renderDeviceRow({hwid:'device',canSetTrafficLimit:true});
  assert.match(personal,/device-edit-limit/);assert.match(personal,/device-limit-gb/);
});

test('manual quota cannot be sent for administrator devices and stale response cannot replace another subscription', async () => {
  const requests=[];
  let resolve;
  const context=vm.createContext({state:{data:{subscriptions:{activeId:1},subscription:{deviceManagement:true,devices:[{hwid:'device',canSetTrafficLimit:false}]}},deviceBusyHwid:'',deviceLimitDraft:'5'},render(){},localizedText:ru=>ru,showToast(){},post:(url,body)=>{requests.push({url,body});return new Promise(r=>{resolve=r;});}});
  const start=source.indexOf('function closeDeviceEditor()');
  vm.runInContext(source.slice(start,source.indexOf('async function deleteDevice(',start)),context);
  await context.controlDevice('device','limit');
  assert.equal(requests.length,0);
  const pending=context.controlDevice('device','pause');
  assert.equal(requests[0].body.subscriptionId,1);
  context.state.data.subscriptions.activeId=2;
  const other={deviceManagement:true,devices:[]};context.state.data.subscription=other;
  resolve({data:{devices:[{hwid:'device',paused:true}]}}); await pending;
  assert.equal(context.state.data.subscription,other);
  assert.equal(context.state.deviceBusyHwid,'');
});

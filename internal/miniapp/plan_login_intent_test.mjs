import assert from 'node:assert/strict';
import fs from 'node:fs';
import vm from 'node:vm';
import test from 'node:test';
import {createPlanLoginIntent} from './static/plan-login-intent.mjs';

function setup() {
  const values = new Map();
  const storage = {getItem:key=>values.get(key),setItem:(key,value)=>values.set(key,value),removeItem:key=>values.delete(key)};
  let time = 100000;
  const options = {getStorage:()=>storage,now:()=>time};
  return {values,options,create:()=>createPlanLoginIntent(options),advance:ms=>time+=ms};
}

test('plan choice survives authentication reload without query and is consumed once', () => {
  const fixture = setup();
  const beforeLogin = fixture.create();
  beforeLogin.capture('month-unlimited');
  assert.equal(beforeLogin.peek(),'month-unlimited'); // failed/cancelled login keeps intent
  const afterRedirect = fixture.create();
  assert.equal(afterRedirect.peek(),'month-unlimited');
  assert.equal(afterRedirect.consume([{id:'month',months:1},{id:'month-unlimited',months:1}]),'month-unlimited');
  assert.equal(fixture.create().peek(),'');
  assert.equal(afterRedirect.consume([{id:'month-unlimited'}]),'');
});

test('new landing selection replaces previous intent; logout, expiry and missing plan clear it', () => {
  const fixture = setup(), intent = fixture.create();
  intent.capture('old'); intent.capture('new');
  assert.equal(intent.peek(),'new');
  intent.clear(); assert.equal(fixture.create().peek(),'');
  intent.capture('old'); fixture.advance(2*60*60*1000);
  assert.equal(fixture.create().peek(),'');
  intent.capture('removed');
  assert.equal(intent.consume([{id:'available'}]),'');
  assert.equal(intent.peek(),'');
});

test('corrupt storage and blocked storage do not break authentication', () => {
  const fixture = setup();
  fixture.values.set('link-bot-plan-login-intent','broken');
  assert.equal(fixture.create().peek(),'');
  fixture.values.set('link-bot-plan-login-intent',JSON.stringify({id:'bad',createdAt:Infinity}));
  assert.equal(fixture.create().peek(),'');
  const intent = createPlanLoginIntent({getStorage:()=>{throw new Error('blocked');}});
  intent.capture('plan with & special/id');
  assert.equal(intent.consume([{id:'plan with & special/id'}]),'plan with & special/id');
  intent.capture('x'.repeat(257)); assert.equal(intent.peek(),'');
});

const appSource = fs.readFileSync(new URL('./static/app.js',import.meta.url),'utf8');
const selectionSource = appSource.slice(appSource.indexOf('function ensureSelections()'),appSource.indexOf('function syncGiftReceiptState()'));
function cabinet(intent, {surface='browser', paymentReturn=false, admin=false} = {}) {
  const plans=[{id:'ordinary',months:1,recommended:true},{id:'unlimited',months:1}];
  const state={data:null,currentPage:admin?'admin':'dashboard',adminWorkspace:admin,selectedPlanId:'ordinary',selectedPlanMonths:1};
  let currentURL=new URL('https://cabinet.example.com/login?other=kept');
  const context=vm.createContext({state,clientSurface:surface,adminEntry:admin,planLoginIntent:intent,URL,
    window:{location:{get href(){return currentURL.href;}},history:{replaceState:(_,__,url)=>{currentURL=new URL(url,currentURL);}}},
    getDisplayedPlans:()=>plans,getPaymentReturnState:()=>paymentReturn,
    planKey:plan=>plan?.id||'',getGiftPlans:()=>[],getSelectedPlan:()=>plans.find(x=>x.id===state.selectedPlanId),
    getAvailableMethods:()=>[],pageFeatureEnabled:()=>true,canAdminSection:()=>true,isAdminUser:()=>admin,
    getRuntimeSettings:()=>({}),writeSetting(){},STORAGE_KEYS:{},administrators:{},getSelectedGiftPlan:()=>null});
  vm.runInContext(selectionSource,context);
  return {state,select:()=>context.ensureSelections(),url:()=>currentURL};
}

test('successful shared bootstrap restores exact plan and plans page after any browser login', () => {
  for(const method of ['Telegram','QR','Google','email']) {
    const fixture=setup(); fixture.create().capture('unlimited');
    const {state,select,url}=cabinet(fixture.create());
    select(); assert.equal(fixture.create().peek(),'unlimited',method+' must wait for bootstrap');
    state.data={plans:[{id:'ordinary',months:1},{id:'unlimited',months:1}]}; select();
    assert.equal(state.currentPage,'buy',method);
    assert.equal(state.selectedPlanId,'unlimited',method);
    assert.equal(url().searchParams.get('planId'),'unlimited');
    assert.equal(url().searchParams.get('other'),'kept');
    assert.equal(fixture.create().peek(),'');
    state.selectedPlanId='ordinary'; select(); assert.equal(state.selectedPlanId,'ordinary');
  }
});

test('pending browser choice does not override payment return, admin or Telegram Mini App', () => {
  for(const options of [{surface:'telegram'},{paymentReturn:true},{admin:true}]) {
    const fixture=setup(), intent=fixture.create(); intent.capture('unlimited');
    const {state,select}=cabinet(intent,options); state.data={}; select();
    assert.equal(state.selectedPlanId,'ordinary');
    assert.notEqual(state.currentPage,'buy');
  }
});

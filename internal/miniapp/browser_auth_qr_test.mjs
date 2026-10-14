import assert from 'node:assert/strict';
import fs from 'node:fs';
import vm from 'node:vm';
import test from 'node:test';

const source=fs.readFileSync(new URL('./static/app.js',import.meta.url),'utf8');
const qrCode=source.slice(source.indexOf('async function pollTelegramQRLogin()'),source.indexOf('function startTelegramBrowserLogin('));
const flush=async()=>{for(let i=0;i<8;i++)await Promise.resolve();};
function harness() {
  const callbacks=new Map(); let nextTimer=0,authenticated=false,visible=true;
  const email={value:'qa@example.test'},password={value:'local-only'},focus=password;
  const updates=[]; const host={set outerHTML(value){updates.push(value);}};
  const context=vm.createContext({Date,Promise,String,Error,
    telegramQRLogin:null,telegramQRLoginPollTimer:null,telegramQRLoginRetryTimer:null,telegramQRLoginCreatePromise:null,telegramQRLoginPollBusy:false,
    hasAuth:()=>authenticated,app:{querySelector:()=>visible?host:null},
    document:{hidden:false,activeElement:focus},
    renderBrowserAuthQR:()=>context.telegramQRLogin?.url?'ready':context.telegramQRLogin?.error?'error':'loading',
    render(){throw new Error('QR updates must not recreate the form');},
    post:async()=>({data:{id:'qr',secret:'test',url:'https://t.me/example_bot',expiresAt:new Date(Date.now()+60000).toISOString()}}),
    scheduleTelegramQRLoginPoll:delay=>{context.pollDelay=delay;},
    stopTelegramQRLogin(){context.stopped=true;},
    window:{setTimeout:fn=>{const id=++nextTimer;callbacks.set(id,fn);return id;},clearTimeout:id=>callbacks.delete(id)},
    state:{},STORAGE_KEYS:{},writeSessionSetting(){},clearGoogleAuth(){},refreshDashboard:async()=>{},browserAuthCopy:()=>({loginFailed:'failed'}),showToast(){},clearBrowserTelegramAuth(){},
  });
  vm.runInContext(qrCode,context);
  return {context,updates,email,password,focus,callbacks,authenticate:()=>authenticated=true,hide:()=>visible=false};
}

test('late QR creation only replaces QR and preserves input nodes, values and focus',async()=>{
  const page=harness(); page.context.mountTelegramQRLogin(); await flush();
  assert.deepEqual(page.updates,['ready']);
  assert.equal(page.context.document.activeElement,page.password);
  assert.equal(page.email.value,'qa@example.test'); assert.equal(page.password.value,'local-only');
  assert.equal(page.context.pollDelay,900);
});

test('QR creation failure and retry leave the form mounted',async()=>{
  const page=harness();
  page.context.post=async()=>{throw new Error('unavailable');};
  page.context.mountTelegramQRLogin(); await flush();
  assert.deepEqual(page.updates,['error']);
  const retry=[...page.callbacks.values()][0]; assert.ok(retry);
  retry(); await flush();
  assert.deepEqual(page.updates,['error','loading','error']);
  assert.equal(page.context.document.activeElement,page.password);
});

test('expired QR creates a new code without recreating the form',async()=>{
  const page=harness();
  page.context.telegramQRLogin={id:'old',secret:'test'};
  page.context.post=async url=>{
    if(url.endsWith('/status')){const error=new Error('expired');error.code='qr_login_expired';throw error;}
    return {data:{id:'new',secret:'test',url:'https://t.me/example_bot',expiresAt:new Date(Date.now()+60000).toISOString()}};
  };
  await page.context.pollTelegramQRLogin(); await flush();
  assert.deepEqual(page.updates,['loading','ready']);
  assert.equal(page.context.document.activeElement,page.password);
});

test('late QR cannot bring back hidden form or overwrite successful authentication',async()=>{
  for(const action of ['hide','authenticate']){
    const page=harness(); let resolve;
    page.context.post=()=>new Promise(done=>{resolve=done;});
    page.context.mountTelegramQRLogin(); page[action]();
    resolve({data:{id:'qr',secret:'test',url:'https://t.me/example_bot',expiresAt:new Date(Date.now()+60000).toISOString()}});
    await flush(); assert.deepEqual(page.updates,[]);
  }
});

test('shrinking visible viewport keeps the focused login input above the keyboard',()=>{
  const properties=new Map([['--auth-viewport-height','844px']]),frames=[];
  let scrolled=0;
  const field={closest:()=>true,matches:()=>true,scrollIntoView:()=>scrolled++};
  const context=vm.createContext({parseFloat,Math,clientSurface:'browser',tg:null,
    syncTelegramLayout(){},
    document:{activeElement:field,documentElement:{clientHeight:844,style:{getPropertyValue:key=>properties.get(key),setProperty:(key,value)=>properties.set(key,value)}}},
    window:{innerHeight:844,visualViewport:{height:420},requestAnimationFrame:fn=>frames.push(fn)},
  });
  vm.runInContext(source.slice(source.indexOf('function syncAppViewportHeight()'),source.indexOf('async function handlePostBootstrapFlow()')),context);
  context.syncAppViewportHeight();
  assert.equal(properties.get('--auth-viewport-height'),'420px');
  frames.shift()(); assert.equal(scrolled,1);
  context.window.visualViewport.height=844; context.syncAppViewportHeight();
  assert.equal(frames.length,0);
  context.window.visualViewport.height=420; context.document.activeElement={closest:()=>false}; context.syncAppViewportHeight();
  assert.equal(frames.length,0);
});

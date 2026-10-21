import test from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';
import vm from 'node:vm';
const source=fs.readFileSync(new URL('./static/app.js',import.meta.url),'utf8');
function load(c,name){const start=source.indexOf(`function ${name}(`),next=source.slice(start+1).search(/\n(?:async )?function /),end=start+1+next;assert.ok(start>=0&&end>start,name);vm.runInContext(source.slice(start,end),c);}
function harness(){
 const settings={paymentMethodText:{},paymentMethodOrder:['card','sbp']};
 const state={adminWorkspace:true,adminSection:'plans',adminSettingsDraft:settings,data:{wallet:{balanceCents:500}}};
 const c=vm.createContext({URL,state,settings,render(){},showToast(){},haptic(){},queueMicrotask:fn=>fn(),app:{querySelector:()=>null},requestModalClose:(name,closed)=>closed(),PAYMENT_LOGO_URLS:{card:'card.svg'},localizedText:ru=>ru,formatMoneyCents:cents=>String(cents),getRuntimeSettings:()=>settings,t:()=>({payMethodCard:'Карта',payMethodCardHint:'Системное описание',payMethodSbp:'СБП',payMethodSbpHint:'По номеру телефона'})});
 c.loadAdminPaymentChannels=async()=>{};
 for(const name of ['validPaymentLogoURL','renderAdminPaymentLogoEditor','paymentMethodBase','paymentChannelsFor','renderPaymentMethodSettings','paymentMethodMeta','getAdminPaymentMethodEditor','openAdminPaymentMethodEditor','closeAdminPaymentMethodEditor','updateAdminPaymentMethodText','applyAdminPaymentMethodEditor','renderAdminPaymentChannels'])load(c,name);
 c.getAdminPaymentMethods=()=>[...new Set(['card','sbp',...Object.keys(c.settings.paymentMethodVariants||{})])].map(id=>c.paymentMethodMeta(id));return c;
}
test('checkout uses per-field fallback and keeps provider IDs and logos',()=>{
 const c=harness();c.settings.paymentMethodText.card={name:'  Моя карта  ',description:''};
 let m=c.paymentMethodMeta('card');assert.equal(m.label,'Моя карта');assert.equal(m.hint,'Системное описание');assert.equal(m.id,'card');assert.equal(m.logo,'card.svg');
 c.settings.paymentMethodText.card={name:' ',description:'Описание'};m=c.paymentMethodMeta('card');assert.equal(m.label,'Карта');assert.equal(m.hint,'Описание');
 assert.equal(c.paymentMethodMeta('card',true).hint,'Системное описание');assert.equal(c.paymentMethodMeta('unknown'),null);
 c.settings.paymentMethodText.balance={name:'Кошелёк'};assert.equal(c.paymentMethodMeta('balance').hint,'Доступно 500');c.state.data.wallet.balanceCents=900;assert.equal(c.paymentMethodMeta('balance').hint,'Доступно 900');
});
test('pencil edits a copy and Done saves only the selected payment method',()=>{
 const c=harness();c.settings.paymentMethodText.sbp={name:'СБП',description:'Another'};
 c.openAdminPaymentMethodEditor('card');assert.equal(c.state.adminPaymentMethodEditor.name,'');assert.equal(c.getAdminPaymentMethodEditor().systemName,'Карта');
 c.updateAdminPaymentMethodText({dataset:{input:'admin-payment-method-name'},value:' Моя карта '});
 assert.equal(c.settings.paymentMethodText.card,undefined);
 c.applyAdminPaymentMethodEditor();assert.equal(c.settings.paymentMethodText.card.name,'Моя карта');assert.equal(c.settings.paymentMethodText.sbp.description,'Another');assert.equal(c.state.adminSettingsDirty,true);
 c.openAdminPaymentMethodEditor('card');c.updateAdminPaymentMethodText({dataset:{input:'admin-payment-method-name'},value:' '});c.applyAdminPaymentMethodEditor();assert.equal(c.settings.paymentMethodText.card,undefined);assert.equal(c.paymentMethodMeta('card').label,'Карта');
});
test('opening payment text editing outside the plan editor does nothing',()=>{
 const c=harness();c.state.adminSection='content';c.openAdminPaymentMethodEditor('card');assert.equal(c.state.adminPaymentMethodEditor,undefined);
 c.state.adminSection='plans';c.openAdminPaymentMethodEditor('unknown');assert.equal(c.state.adminPaymentMethodEditor,undefined);
});

test('payment editor is a separate modal and cancelling returns to the open payment list',()=>{
 const c=harness();c.state.payModalOpen=true;c.escapeHtml=value=>String(value).replaceAll('<','&lt;');c.escapeAttribute=c.escapeHtml;c.icon=name=>`<svg data-test-icon="${name}"></svg>`;c.modalStateClass=()=>'';
 load(c,'renderAdminPaymentMethodEditor');load(c,'getActiveModalName');
 c.openAdminPaymentMethodEditor('card');c.updateAdminPaymentMethodText({dataset:{input:'admin-payment-method-name'},value:'Discard this'});
 assert.equal(c.getActiveModalName(),'admin-payment-method');
 const markup=c.renderAdminPaymentMethodEditor();assert.match(markup,/role="dialog" aria-modal="true"/);assert.match(markup,/aria-labelledby="pay-method-editor-title"/);assert.match(markup,/support-field__input/);
 assert.doesNotMatch(markup,/pay-order-list/);
 c.closeAdminPaymentMethodEditor();assert.equal(c.state.adminPaymentMethodEditor,null);assert.equal(c.state.payModalOpen,true);assert.equal(c.settings.paymentMethodText.card,undefined);assert.equal(c.getActiveModalName(),'pay');
});

test('payment list has a real pencil and does not append text fields below its rows',()=>{
 const c=harness();c.state.adminPlanEditing=true;c.modalStateClass=()=>'';c.escapeHtml=String;c.escapeAttribute=String;c.icon=name=>`<svg data-test-icon="${name}"></svg>`;c.renderPaymentMethodLogo=()=>'';load(c,'renderPayModal');
 c.openAdminPaymentMethodEditor('card');const markup=c.renderPayModal();
 assert.match(markup,/data-action="admin-payment-method-edit"[^>]*><svg data-test-icon="pencil"/);
 assert.doesNotMatch(markup,/admin-payment-method-name|<input|<textarea/);
});

test('copies keep independent names and channel settings and can be removed',()=>{
 const c=harness();c.crypto={randomUUID:()=> 'aabbccdd-0000-0000-0000-000000000000'};
 c.settings.paymentMethodVariants={card:{method:'card',channels:['bank_card']}};
 c.settings.paymentMethodText.card={name:'Карта',description:'Original'};
 for(const name of ['duplicateAdminPaymentMethod','removeAdminPaymentMethod'])load(c,name);
 c.duplicateAdminPaymentMethod('card');const id='card__aabbccdd';
 assert.equal(c.state.adminPaymentMethodEditor.id,id);assert.equal(c.paymentMethodMeta(id).logo,'card.svg');assert.equal(c.settings.paymentMethodOrder[1],id);
 c.state.adminPaymentMethodEditor.name='СБП';c.state.adminPaymentMethodEditor.channels=['sbp','sberbank'];c.applyAdminPaymentMethodEditor();
 assert.equal(c.settings.paymentMethodText[id].name,'СБП');assert.equal(c.settings.paymentMethodText.card.name,'Карта');assert.deepEqual([...c.settings.paymentMethodVariants.card.channels],['bank_card']);assert.deepEqual([...c.settings.paymentMethodVariants[id].channels],['sbp','sberbank']);
 c.openAdminPaymentMethodEditor(id);c.removeAdminPaymentMethod();assert.equal(c.settings.paymentMethodVariants[id],undefined);assert.equal(c.settings.paymentMethodOrder.includes(id),false);assert.equal(c.settings.paymentMethodText[id],undefined);
});

test('channel chooser accepts only allowed methods and resumes checkout',()=>{
 const c=harness();c.settings.paymentMethodVariants={sbp:{method:'sbp',channels:['sbp','sberbank']}};
 c.state.data.paymentChannels={sbp:{multiple:false,options:[{id:'sbp',name:'СБП'},{id:'sberbank',name:'SberPay'}]}};
 for(const name of ['selectedPaymentChannel','requirePaymentChannel','confirmPaymentChannel'])load(c,name);
 let calls=0;c.startPayment=async()=>{calls++};c.startGiftPayment=async()=>{calls++};c.getSelectedPaymentMethod=()=>({id:'sbp'});
 assert.equal(c.requirePaymentChannel('sbp',{mode:'purchase',deviceOnly:true}),true);
 c.confirmPaymentChannel('bank_card');assert.equal(calls,0);
 c.confirmPaymentChannel('sberbank');assert.equal(calls,1);assert.equal(c.selectedPaymentChannel('sbp'),'sberbank');assert.equal(c.state.payModalOpen,false);
 c.settings.paymentMethodVariants.sbp.channels=['sbp'];assert.equal(c.selectedPaymentChannel('sbp'),'');assert.equal(c.requirePaymentChannel('sbp',{}),false);
});

test('an API with only preferred methods is never described as a hard restriction',()=>{
 const c=harness();c.state.data.paymentChannels={card:{options:[{id:'card',name:'Карта'}],multiple:true,preference:true,note:'Preference only'}};
 c.escapeHtml=String;c.escapeAttribute=String;c.icon=()=>'';
 const html=c.renderAdminPaymentChannels({id:'card',channels:['card']});assert.match(html,/Другие способы могут остаться доступными/);assert.doesNotMatch(html,/В форме оплаты останутся выбранные методы/);
});


test('payment logo overrides persist independently, cancel cleanly, hide, and reset',()=>{
 const c=harness();c.escapeHtml=String;c.escapeAttribute=String;c.icon=()=>'<svg></svg>';load(c,'renderPaymentMethodLogo');load(c,'setAdminPaymentLogo');load(c,'previewAdminPaymentLogo');
 c.openAdminPaymentMethodEditor('card');c.state.adminPaymentMethodEditor.logoUrl=' https://example.com/card.svg ';
 c.previewAdminPaymentLogo();assert.equal(c.state.adminPaymentMethodEditor.previewLogoUrl,'https://example.com/card.svg');assert.equal(c.settings.paymentMethodText.card,undefined);
 c.applyAdminPaymentMethodEditor();assert.equal(c.paymentMethodMeta('card').logo,'https://example.com/card.svg');assert.equal(c.paymentMethodMeta('card',true).logo,'card.svg');
 c.openAdminPaymentMethodEditor('card');c.setAdminPaymentLogo('none');c.closeAdminPaymentMethodEditor();assert.equal(c.paymentMethodMeta('card').logo,'https://example.com/card.svg');
 c.openAdminPaymentMethodEditor('card');c.setAdminPaymentLogo('none');c.applyAdminPaymentMethodEditor();assert.equal(c.paymentMethodMeta('card').hideLogo,true);assert.equal(c.renderPaymentMethodLogo(c.paymentMethodMeta('card')),'');
 c.openAdminPaymentMethodEditor('card');c.setAdminPaymentLogo('system');c.applyAdminPaymentMethodEditor();assert.equal(c.settings.paymentMethodText.card,undefined);assert.equal(c.paymentMethodMeta('card').logo,'card.svg');
 c.settings.paymentMethodVariants={card__aabbccdd:{method:'card',channels:['bank_card']}};c.settings.paymentMethodText.card__aabbccdd={logoUrl:'https://example.com/copy.png'};
 assert.equal(c.paymentMethodMeta('card__aabbccdd').logo,'https://example.com/copy.png');assert.equal(c.paymentMethodMeta('card').logo,'card.svg');
});

test('invalid logo URLs cannot be applied and image attributes are escaped',()=>{
 const c=harness(),errors=[];c.showToast=message=>errors.push(message);c.escapeAttribute=s=>String(s).replaceAll('"','&quot;');c.icon=()=>'<svg></svg>';load(c,'renderPaymentMethodLogo');
 for(const url of ['http://example.com/a.png','javascript:alert(1)','data:image/png;base64,x','//example.com/a.png','https://user:pass@example.com/a.png']){c.openAdminPaymentMethodEditor('card');c.state.adminPaymentMethodEditor.logoUrl=url;c.applyAdminPaymentMethodEditor();assert.equal(c.settings.paymentMethodText.card,undefined);}
 assert.equal(errors.length,5);const markup=c.renderPaymentMethodLogo({id:'card',logo:'https://example.com/?q=" onerror="alert(1)'});assert.match(markup,/&quot;/);assert.doesNotMatch(markup,/ onerror="/);assert.match(markup,/referrerpolicy="no-referrer"/);
});

test('upload results stay in their editor draft and cannot overwrite another payment method',async()=>{
 const c=harness();let complete,uploads=0;const start=source.indexOf('async function uploadAdminPaymentLogo('),end=start+1+source.slice(start+1).search(/\n(?:async )?function /);vm.runInContext(source.slice(start,end),c);
 c.FormData=class{append(){}};c.postForm=()=>{uploads++;return new Promise(resolve=>{complete=resolve})};
 c.openAdminPaymentMethodEditor('card');const pending=c.uploadAdminPaymentLogo({name:'logo.png',type:'image/png',size:100});assert.equal(c.state.adminPaymentMethodEditor.uploadingLogo,true);c.applyAdminPaymentMethodEditor();assert.equal(c.settings.paymentMethodText.card,undefined);
 c.closeAdminPaymentMethodEditor();c.openAdminPaymentMethodEditor('sbp');complete({data:{url:'/mini-app/uploads/logo-0123456789abcdef.png'}});await pending;assert.equal(c.state.adminPaymentMethodEditor.logoUrl,'');assert.equal(c.settings.paymentMethodText.card,undefined);
 const upload=c.uploadAdminPaymentLogo({name:'logo.png',type:'image/png',size:100});complete({data:{url:'/mini-app/uploads/logo-0123456789abcdef.png'}});await upload;
 assert.equal(c.state.adminPaymentMethodEditor.logoUrl,'/mini-app/uploads/logo-0123456789abcdef.png');assert.equal(c.settings.paymentMethodText.sbp,undefined);c.applyAdminPaymentMethodEditor();assert.equal(c.settings.paymentMethodText.sbp.logoUrl,'/mini-app/uploads/logo-0123456789abcdef.png');
 c.openAdminPaymentMethodEditor('card');await c.uploadAdminPaymentLogo({name:'evil.html',type:'text/html',size:10});await c.uploadAdminPaymentLogo({name:'huge.png',size:3*1024*1024});assert.equal(uploads,2);
});

test('Smerch copies retain independent captions, branding and allowed methods',()=>{
 const c=harness();c.PAYMENT_LOGO_URLS.smerch='payment-smerch.png';
 c.getAdminPaymentMethods=()=>[...new Set(['smerch',...Object.keys(c.settings.paymentMethodVariants||{})])].map(id=>c.paymentMethodMeta(id));
 c.openAdminPaymentMethodEditor('smerch');c.state.adminPaymentMethodEditor.name='Мой СБП';c.state.adminPaymentMethodEditor.channels=['sbp'];c.applyAdminPaymentMethodEditor();
 c.crypto={randomUUID:()=> '11223344-5566-7788-99aa-bbccddeeff00'};load(c,'duplicateAdminPaymentMethod');c.duplicateAdminPaymentMethod('smerch');
 const id=c.settings.paymentMethodOrder.find(id=>id.startsWith('smerch__'));assert.ok(id);
 c.openAdminPaymentMethodEditor(id);c.state.adminPaymentMethodEditor.name='Криптовалюта';c.state.adminPaymentMethodEditor.channels=['crypto'];c.applyAdminPaymentMethodEditor();
 assert.equal(c.paymentMethodMeta(id).logo,'payment-smerch.png');assert.equal(c.paymentMethodMeta(id).label,'Криптовалюта');assert.equal(c.paymentMethodMeta('smerch').label,'Мой СБП');
 assert.deepEqual([...c.settings.paymentMethodVariants.smerch.channels],['sbp']);assert.deepEqual([...c.settings.paymentMethodVariants[id].channels],['crypto']);
});

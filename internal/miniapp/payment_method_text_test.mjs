import test from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';
import vm from 'node:vm';
const source=fs.readFileSync(new URL('./static/app.js',import.meta.url),'utf8');
function load(c,name){const start=source.indexOf(`function ${name}(`),end=source.indexOf('\nfunction ',start+1);assert.ok(start>=0&&end>start,name);vm.runInContext(source.slice(start,end),c);}
function harness(){
 const settings={paymentMethodText:{},paymentMethodOrder:['card','sbp']};
 const state={adminWorkspace:true,adminSection:'plans',adminSettingsDraft:settings,data:{wallet:{balanceCents:500}}};
 const c=vm.createContext({state,settings,render(){},showToast(){},haptic(){},queueMicrotask:fn=>fn(),app:{querySelector:()=>null},requestModalClose:(name,closed)=>closed(),PAYMENT_LOGO_URLS:{card:'card.svg'},localizedText:ru=>ru,formatMoneyCents:cents=>String(cents),getRuntimeSettings:()=>settings,t:()=>({payMethodCard:'Карта',payMethodCardHint:'Системное описание',payMethodSbp:'СБП',payMethodSbpHint:'По номеру телефона'})});
 for(const name of ['paymentMethodMeta','getAdminPaymentMethodEditor','openAdminPaymentMethodEditor','closeAdminPaymentMethodEditor','updateAdminPaymentMethodText','applyAdminPaymentMethodEditor'])load(c,name);
 c.getAdminPaymentMethods=()=>['card','sbp'].map(id=>c.paymentMethodMeta(id));return c;
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

import test from "node:test";
import assert from "node:assert/strict";
import { renderSMTPSettings } from "./static/admin-smtp.mjs";
import fs from "node:fs";
import vm from "node:vm";
const escape = s => String(s).replace(/[&<>"']/g, c => ({ "&":"&amp;", "<":"&lt;", ">":"&gt;", '"':"&quot;", "'":"&#39;" })[c]);
const helpers = { escapeHtml: escape, escapeAttribute: escape, icon: () => "" };
test("SMTP connection editor provides health checks without a duplicate test form", () => {
  const html = renderSMTPSettings({draft:{host:'smtp.example.com',port:'587',enabled:true,passwordConfigured:true,proxyConfigured:true},busy:""},helpers);
  assert.match(html,/data-action="admin-smtp-check"/);
  assert.doesNotMatch(html,/admin-smtp-test|testEmail/);
  assert.match(html,/Сохранён · введите новый/);
  assert.match(html,/type="password" data-input="admin-smtp-password" value=""/);
});
test("SMTP fields escape server input and an operation disables duplicate submissions", () => {
  const html=renderSMTPSettings({draft:{host:'\"><script>alert(1)</script>'},busy:'test'},helpers);
  assert.doesNotMatch(html,/<script>/);
  assert.match(html,/&lt;script&gt;/);
  assert.match(html,/data-action="admin-smtp-check" disabled/);
});

test("SMTP requests send only fields accepted by the strict server decoder", async () => {
  const source=fs.readFileSync(new URL("./static/app.js",import.meta.url),"utf8");
  let payload;
  const state={adminSection:"broadcast",adminBroadcastTab:"email",adminEmailPreviewAddress:"test@example.com",adminEmailBroadcast:{configured:false},adminSMTP:{draft:{host:"smtp.example.com",port:587,user:"sender",password:"",from:"sender@example.com",proxyUrl:"",enabled:true,passwordConfigured:true,proxyConfigured:true,source:"admin"},busy:""}};
  const context=vm.createContext({state,canAdmin:()=>true,render(){},showToast(){},previewMode:false,post:async(path,body)=>{payload={path,body};return{data:{host:"smtp.example.com",enabled:true,passwordConfigured:true}}}});
  vm.runInContext(source.slice(source.indexOf("async function submitAdminSMTP("),source.indexOf("async function checkAdminAI(")),context);
  await context.submitAdminSMTP("save");
  assert.equal(payload.path,"/api/mini-app/admin/smtp/update");
  assert.deepEqual(Object.keys(payload.body).sort(),["clearProxy","email","enabled","from","host","password","port","proxyUrl","user"]);
  assert.equal(payload.body.port,"587");
  assert.equal(state.adminSMTP.draft.password,"");
  assert.equal(state.adminEmailBroadcast.configured,true);
});

test("SMTP settings are inside email broadcast and remain independently restricted", async () => {
  const source=fs.readFileSync(new URL("./static/app.js",import.meta.url),"utf8");
  const state={locale:"ru",adminSMTP:{draft:{host:"smtp.example.com"}},adminEmailBroadcast:{},adminBroadcastTab:"email",adminEmailEditorTab:"message"};
  let permissions=new Set(["smtp"]);
  const context=vm.createContext({state,canAdmin:p=>permissions.has(p),emailAuthText:s=>s,...helpers,pageClass:()=>"",renderSMTPSettings, broadcastStatusLabel:()=>"Пусто"});
  vm.runInContext(source.slice(source.indexOf("function canAdminSection("),source.indexOf("function getBottomNavPages(")),context);
  vm.runInContext(source.slice(source.indexOf("function renderAdminBroadcastTabs("),source.indexOf("function renderAdminBroadcastButton(")),context);
  assert.equal(context.canAdminSection("broadcast"),true);
  const smtpOnly=context.renderAdminEmailBroadcastPage();
  assert.match(smtpOnly,/data-input="admin-smtp-host"/);
  assert.doesNotMatch(smtpOnly,/admin-email-send|admin-email-save|data-value="telegram"/);
  permissions=new Set(["broadcast"]);
  const broadcastOnly=context.renderAdminEmailBroadcastPage();
  assert.match(broadcastOnly,/data-admin-email-subject/);
  assert.doesNotMatch(broadcastOnly,/admin-smtp-host|admin-smtp-toggle/);
  permissions=new Set(["broadcast","smtp"]);
  state.adminEmailEditorTab="connection";
  assert.match(context.renderAdminEmailBroadcastPage(),/admin-smtp-host/);
  state.adminEmailEditorTab="test";
  const testPanel=context.renderAdminEmailBroadcastPage();
  assert.equal((testPanel.match(/data-admin-email-preview/g)||[]).length,1);
  assert.equal((testPanel.match(/data-action="admin-email-test"/g)||[]).length,1);
  assert.doesNotMatch(testPanel,/admin-smtp-test|admin-email-preview-send/);
  assert.doesNotMatch(source,/\[localizedText\("Почта \/ SMTP"/);
});

test("test email saves the latest editor content then previews that same draft", async () => {
  const source=fs.readFileSync(new URL("./static/app.js",import.meta.url),"utf8");
  const requests=[];
  const state={adminEmailDraftDirty:true,adminEmailDraftSubject:"Тема",adminEmailDraftBody:"Новый текст",adminEmailPreviewAddress:"test@example.com",adminEmailBusy:"",adminEmailBroadcast:{}};
  const context=vm.createContext({state,render(){},showToast(){},emailAuthText:s=>s,post:async(path,body)=>{requests.push({path,body});return{data:{subject:body.subject,body:body.body}}}});
  vm.runInContext(source.slice(source.indexOf("async function saveAdminEmailBroadcast("),source.indexOf("async function sendAdminEmailBroadcast(")),context);
  await context.previewAdminEmailBroadcast();
  assert.equal(requests.length,2);
  assert.equal(requests[0].path,"/api/mini-app/admin/broadcast/email/save");
  assert.equal(requests[0].body.body,"Новый текст");
  assert.equal(requests[1].path,"/api/mini-app/admin/broadcast/email/preview");
  assert.equal(requests[1].body.email,"test@example.com");
});

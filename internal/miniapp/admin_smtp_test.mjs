import test from "node:test";
import assert from "node:assert/strict";
import { renderSMTPSettings } from "./static/admin-smtp.mjs";
import fs from "node:fs";
import vm from "node:vm";
const escape = s => String(s).replace(/[&<>"']/g, c => ({ "&":"&amp;", "<":"&lt;", ">":"&gt;", '"':"&quot;", "'":"&#39;" })[c]);
const helpers = { escapeHtml: escape, escapeAttribute: escape, icon: () => "" };
test("SMTP editor provides health checks and test mail with masked saved credentials", () => {
  const html = renderSMTPSettings({draft:{host:'smtp.example.com',port:'587',enabled:true,passwordConfigured:true,proxyConfigured:true},busy:""},helpers);
  assert.match(html,/data-action="admin-smtp-check"/);
  assert.match(html,/data-action="admin-smtp-test"/);
  assert.match(html,/data-input="admin-smtp-testEmail"/);
  assert.match(html,/Сохранён · введите новый/);
  assert.match(html,/type="password" data-input="admin-smtp-password" value=""/);
});
test("SMTP fields escape server input and an operation disables duplicate submissions", () => {
  const html=renderSMTPSettings({draft:{host:'\"><script>alert(1)</script>'},busy:'test'},helpers);
  assert.doesNotMatch(html,/<script>/);
  assert.match(html,/&lt;script&gt;/);
  assert.match(html,/data-action="admin-smtp-test" disabled/);
});

test("SMTP requests send only fields accepted by the strict server decoder", async () => {
  const source=fs.readFileSync(new URL("./static/app.js",import.meta.url),"utf8");
  let payload;
  const state={adminSection:"broadcast",adminBroadcastTab:"email",adminEmailBroadcast:{configured:false},adminSMTP:{draft:{host:"smtp.example.com",port:587,user:"sender",password:"",from:"sender@example.com",proxyUrl:"",enabled:true,passwordConfigured:true,proxyConfigured:true,source:"admin"},busy:"",testEmail:"test@example.com"}};
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
  const state={locale:"ru",adminSMTP:{expanded:true,draft:{host:"smtp.example.com"}},adminEmailBroadcast:{},adminBroadcastTab:"email"};
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
  assert.match(context.renderAdminEmailBroadcastPage(),/admin-smtp-host/);
  assert.doesNotMatch(source,/\[localizedText\("Почта \/ SMTP"/);
});

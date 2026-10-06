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
  const state={adminSection:"smtp",adminSMTP:{draft:{host:"smtp.example.com",port:587,user:"sender",password:"",from:"sender@example.com",proxyUrl:"",enabled:true,passwordConfigured:true,proxyConfigured:true,source:"admin"},busy:"",testEmail:"test@example.com"}};
  const context=vm.createContext({state,canAdmin:()=>true,render(){},showToast(){},previewMode:false,post:async(path,body)=>{payload={path,body};return{data:{host:"smtp.example.com",passwordConfigured:true}}}});
  vm.runInContext(source.slice(source.indexOf("async function submitAdminSMTP("),source.indexOf("async function checkAdminAI(")),context);
  await context.submitAdminSMTP("save");
  assert.equal(payload.path,"/api/mini-app/admin/smtp/update");
  assert.deepEqual(Object.keys(payload.body).sort(),["clearProxy","email","enabled","from","host","password","port","proxyUrl","user"]);
  assert.equal(payload.body.port,"587");
  assert.equal(state.adminSMTP.draft.password,"");
});

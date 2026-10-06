import test from "node:test";
import assert from "node:assert/strict";
import fs from "node:fs";
import vm from "node:vm";
const source=fs.readFileSync(new URL("./static/app.js",import.meta.url),"utf8");
const code=source.slice(source.indexOf("async function changeSupportHandling("),source.indexOf("async function sendSupportMediaMessage("));

test("handoff and operator requests match their strict server schemas", async () => {
  for(const [action,value,keys] of [["support-handoff","",["ticketId"]],["support-operator","claim",["release","ticketId"]],["support-operator","release",["release","ticketId"]]]) {
    let payload;
    const state={supportBusy:"",supportThreadOpen:true,activeSupportTicketId:101};
    const context=vm.createContext({state,closingModalName:"",supportThreadVersion:1,beginSupportOperation:()=>1,finishSupportOperation:()=>true,isCurrentSupportThread:()=>true,render(){},refreshSupport(){},post:async(path,body)=>{payload={path,body};return{data:{ticket:{id:101}}}}});
    vm.runInContext(code,context);await context.changeSupportHandling(action,value);
    assert.deepEqual(Object.keys(payload.body).sort(),keys);
    assert.equal(payload.body.ticketId,101);
    if(action==="support-operator")assert.equal(payload.body.release,value==="release");
  }
});

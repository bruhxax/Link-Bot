import test from "node:test";
import assert from "node:assert/strict";
import fs from "node:fs";
import vm from "node:vm";
const source=fs.readFileSync(new URL("./static/app.js",import.meta.url),"utf8");
const code=source.slice(source.indexOf("async function refreshAdministrators("),source.indexOf("function editAdministrator("));

test("silent unchanged administrator refresh neither remounts nor blocks list interactions", async () => {
  const administrators={items:[{customerId:1,role:"Admin"}],total:1,catalog:[],candidates:[],candidateTotal:0,error:"",busy:"",requestID:0,picking:false,query:""};
  let renders=0,busyDuringRequest;
  const context=vm.createContext({administrators,state:{adminSection:"administrators"},canAdmin:()=>true,renderAdministratorsPreservingFocus(){renders++},post:async()=>{busyDuringRequest=administrators.busy;return{data:{items:[{customerId:1,role:"Admin"}],total:1,permissions:[]}}}});
  vm.runInContext(code,context);await context.refreshAdministrators({silent:true});
  assert.equal(renders,0);
  assert.equal(busyDuringRequest,"");
  context.post=async()=>({data:{items:[{customerId:1,role:"Support"}],total:1,permissions:[]}});
  await context.refreshAdministrators({silent:true});
  assert.equal(renders,1);
  assert.equal(administrators.items[0].role,"Support");
});

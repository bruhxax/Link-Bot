import test from "node:test";
import assert from "node:assert/strict";
import fs from "node:fs";
import vm from "node:vm";

const source=fs.readFileSync(new URL("./static/app.js",import.meta.url),"utf8");
const rows=length=>Array.from({length},(_,i)=>({key:`panel:${i}`,customerId:0,panelId:i+1,usedTrafficBytes:i}));
function setup(state,post) {
 const timers=[];
 const context=vm.createContext({state,post,previewMode:false,window:{clearTimeout(){},setTimeout(fn){timers.push(fn);return timers.length;}},
  render(){},renderRealtime(){},syncAdminUsersBusyDOM(){},captureAdminUsersSearchFocus(){},restoreAdminUsersSearchFocus(){},showToast(){}});
 vm.runInContext('let adminUsersSearchRequestID=0;'+source.slice(source.indexOf("let adminUsersPollTimer"),source.indexOf("function captureAdminUsersSearchFocus")),context);
 return {context,timers};
}

test("background observations preserve loaded panel rows and the pagination revision",async()=>{
 const state={adminWorkspace:true,adminSection:"users",adminUsersQuery:"",adminUsers:{items:rows(90),total:150,snapshot:"old",query:""},adminUsersBusy:""},requests=[];
 const {context}=setup(state,async(url,body)=>{requests.push(body);return {data:{snapshot:"new",items:rows(30).map(row=>({...row,usedTrafficBytes:500})),total:151}}});
 await context.refreshAdminUsers({silent:true});
 assert.equal(state.adminUsers.items.length,90);assert.equal(state.adminUsers.items[0].usedTrafficBytes,500);assert.equal(state.adminUsers.items[80].usedTrafficBytes,80);
 assert.equal(state.adminUsers.snapshot,"old");assert.equal(state.adminUsers.total,150);assert.equal(requests[0].source,"panel");
});

test("append uses panel revision and stable identities for unlinked panel users",async()=>{
 const state={adminWorkspace:true,adminSection:"users",adminUsersQuery:"",adminUsers:{items:rows(30),total:90,snapshot:"same",query:""},adminUsersBusy:""},requests=[];
 const {context}=setup(state,async(url,body)=>{requests.push(body);return {data:{snapshot:"same",items:rows(60).slice(29),total:90}}});
 await context.refreshAdminUsers({append:true});
 assert.equal(requests[0].offset,30);assert.equal(requests[0].snapshot,"same");assert.equal(state.adminUsers.items.length,60);
 assert.equal(new Set(state.adminUsers.items.map(row=>row.key)).size,60);assert.equal(state.adminUsersBusy,"");
});

test("rejected and discarded user searches release busy state so More remains usable",async()=>{
 const state={adminWorkspace:true,adminSection:"users",adminUsersQuery:"",adminUsers:{items:rows(30),query:""},adminUsersBusy:""};
 const {context}=setup(state,async()=>{throw Error("offline")});
 await context.refreshAdminUsers({append:true});assert.equal(state.adminUsersError,"offline");assert.equal(state.adminUsersBusy,"");
 let finish;context.post=()=>new Promise(resolve=>finish=resolve);
 const pending=context.refreshAdminUsers({append:true});state.adminSection="home";finish({data:{items:rows(60),total:60}});await pending;
 assert.equal(state.adminUsersBusy,"");assert.equal(state.adminUsers.items.length,30);
});

test("pending panel catalog polls without erasing previously loaded list",async()=>{
 const state={adminWorkspace:true,adminSection:"users",adminUsersQuery:"",adminUsers:{items:rows(30),total:90,snapshot:"same",query:""},adminUsersBusy:""};
 const {context,timers}=setup(state,async()=>({data:{pending:true,items:[],total:0}}));
 await context.refreshAdminUsers({silent:true});assert.equal(state.adminUsers.items.length,30);assert.equal(state.adminUsers.snapshot,"same");assert.equal(timers.length,1);
});

test("pending overview keeps panel and finance snapshots then schedules bounded polling",async()=>{
 const state={adminWorkspace:true,adminSection:"home",adminOverviewBusy:false,adminOverview:{days:90,users:{source:"panel",total:18},revenue:{allRub:700},subscriptions:[{count:12}]}},timers=[];
 const context=vm.createContext({state,previewMode:false,isAdminUser:()=>true,render(){},post:async()=>({data:{days:90,pending:true,users:null,revenue:null}}),window:{clearTimeout(){},setTimeout(fn){timers.push(fn)}}});
 vm.runInContext(source.slice(source.indexOf("let adminOverviewRequestID"),source.indexOf("async function refreshAdminFinance")),context);
 await context.refreshAdminOverview();assert.equal(state.adminOverview.users.total,18);assert.equal(state.adminOverview.revenue.allRub,700);assert.equal(timers.length,1);assert.equal(state.adminOverviewBusy,false);
});

test("a query changed during append is fetched after the append releases its lock",async()=>{
 const state={adminWorkspace:true,adminSection:"users",adminUsersQuery:"",adminUsers:{items:rows(30),query:"",snapshot:"old"},adminUsersBusy:""},requests=[];
 let finish;
 const {context}=setup(state,async(url,body)=>{requests.push(body);if(requests.length===1)return new Promise(resolve=>finish=resolve);return {data:{items:rows(1),total:1}}});
 const pending=context.refreshAdminUsers({append:true});state.adminUsersQuery="new query";
 finish({data:{items:rows(60),total:60}});await pending;await new Promise(resolve=>setImmediate(resolve));
 assert.equal(requests[1].query,"new query");assert.equal(requests[1].offset,0);assert.equal(state.adminUsers.items.length,1);assert.equal(state.adminUsersBusy,"");
});

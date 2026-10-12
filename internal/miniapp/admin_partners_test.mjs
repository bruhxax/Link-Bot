import test from "node:test";
import assert from "node:assert/strict";
import {partnerRows,partnerMetrics} from "../../admin-ui/partners-data.mjs";

test("partner lists include all dates while search and status filters remain available",()=>{
 const items=[{id:1,username:"one",telegramId:12,isActive:true,createdAt:"2026-09-30T22:00:00Z"},{id:2,username:"two",telegramId:34,isActive:false,createdAt:"2026-09-30T19:00:00Z"}];
 assert.equal(partnerRows(items).length,2);
 assert.equal(partnerRows(items,{query:"34",status:"disabled"})[0].id,2);
 assert.equal(partnerRows(items,{query:"one",status:"disabled"}).length,0);
 assert.equal(partnerRows(items,{descending:false})[0].id,2);
});

test("partner counts always cover all time, including partners from previous months",()=>{
 const data={partners:[{id:1,isActive:true,createdAt:"2026-10-12"},{id:2,isActive:false,createdAt:"2026-09-01"}],applications:[{id:3,status:"pending",createdAt:"2026-10-12"},{id:4,status:"approved",createdAt:"2026-10-11"}]};
 assert.deepEqual(partnerMetrics(data),{total:2,active:1,applications:2,totalShare:100,activeShare:50,applicationsShare:100});
 assert.equal(partnerRows(data.applications,{applications:true,status:"pending"})[0].id,3);
 assert.deepEqual(partnerMetrics({partners:[],applications:[]}),{total:0,active:0,applications:0,totalShare:0,activeShare:0,applicationsShare:0});
});

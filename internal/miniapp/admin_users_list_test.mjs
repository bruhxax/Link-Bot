import test from "node:test";
import assert from "node:assert/strict";
import {userDateKey,userInDateRange,userStatus,userType,usersCSV} from "../../admin-ui/users-list-data.mjs";

test("user creation ranges use Moscow dates, include both bounds, and leave unknown dates explicit",()=>{
 assert.equal(userDateKey("2026-10-11T22:00:00Z"),"2026-10-12");
 assert.equal(userInDateRange("2026-10-11T22:00:00Z",["2026-10-12","2026-10-12"]),true);
 assert.equal(userInDateRange("2026-10-11T20:00:00Z",["2026-10-12",null]),false);
 assert.equal(userInDateRange("2026-10-12T21:00:00Z",[null,"2026-10-12"]),false);
 assert.equal(userInDateRange(undefined,[null,null]),true);
 assert.equal(userInDateRange(undefined,["2026-10-12",null]),false);
});
test("user type filters preserve unknown panel state and separate unlimited from absent subscriptions",()=>{
 assert.equal(userType({subscriptionStatus:"active",trafficLoaded:false,trafficLimitBytes:0}),"unknown");
 assert.equal(userType({subscriptionStatus:"active",trafficLoaded:true,trafficLimitBytes:0}),"unlimited");
 assert.equal(userType({subscriptionStatus:"active",trafficLoaded:true,trafficLimitBytes:100}),"limited");
 assert.equal(userType({subscriptionStatus:"none",trafficLoaded:true}),"none");
 assert.equal(userStatus({isBlocked:true,subscriptionStatus:"ACTIVE"}),"blocked");
});
test("CSV uses actual loaded rows, retains unknown traffic, escapes quotes and neutralizes spreadsheet formulas",()=>{
 const csv=usersCSV([{customerId:12,telegramId:123,username:"=cmd()",description:'hello;"quoted"',trafficLoaded:false},{customerId:13,telegramId:456,username:"normal",trafficLoaded:true,trafficLimitBytes:0,usedTrafficBytes:1024}]);
 assert.equal(csv.split("\r\n").length,3);
 assert.ok(csv.includes('"\'=cmd()"'));
 assert.ok(csv.includes('"hello;""quoted"""'));
 assert.ok(csv.includes('"1.00 KiB";"Безлимит"'));
 assert.ok(!csv.includes("undefined"));
});

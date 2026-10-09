import test from "node:test";
import assert from "node:assert/strict";
import {trafficOverview, expirationText} from "../../admin-ui/user-overview.mjs";

test("unlimited usage shows full progress with current and lifetime bytes",()=>{
  const value=trafficOverview({trafficLoaded:true,trafficLimitBytes:0,usedTrafficBytes:1024,lifetimeUsedTrafficBytes:1024**3,trafficLimitStrategy:"NO_RESET"});
  assert.equal(value.progress,100);assert.equal(value.limit,"∞");assert.equal(value.used,"1.00 KiB");assert.equal(value.lifetime,"1.00 GiB");
  assert.equal(trafficOverview({trafficLoaded:false}),null);
});
test("over-limit traffic clamps only the bar and preserves the actual percentage",()=>{
  const value=trafficOverview({trafficLoaded:true,trafficLimitBytes:100,usedTrafficBytes:150,trafficLimitStrategy:"MONTH"});
  assert.equal(value.percent,150);assert.equal(value.progress,100);assert.equal(value.remaining,0);assert.equal(value.color,"red");assert.equal(value.strategy,"за месяц");
});
test("expiry shows expired, active, unlimited and missing access independently",()=>{
  const now=Date.parse("2026-10-09T00:00:00Z");
  assert.equal(expirationText("2026-10-10T00:00:00Z",now),"Истекает завтра");
  assert.equal(expirationText("2026-10-08T00:00:00Z",now),"Истекла вчера");
  assert.equal(expirationText("2126-10-08T00:00:00Z",now),"∞");
  assert.equal(expirationText("",now),"—");
});

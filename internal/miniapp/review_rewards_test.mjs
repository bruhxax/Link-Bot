import test from "node:test";
import assert from "node:assert/strict";
import { reviewRewardDraft, reviewRewardsFromDraft, reviewRewardSummary } from "./static/review-rewards.mjs";

test("review rewards can combine access, balance and a personal promo", () => {
  const d = reviewRewardDraft({days:7,trafficGb:50,balanceRub:100,promo:{enabled:true,rewardType:"days_traffic",rewardValue:3,rewardTrafficGb:20,expiryDays:30}});
  const r = reviewRewardsFromDraft(d);
  assert.equal(r.days,7); assert.equal(r.trafficGb,50); assert.equal(r.balanceRub,100); assert.equal(r.promo.rewardValue,3); assert.equal(r.promo.rewardTrafficGb,20); assert.equal(r.promo.discountPercent,0);
  assert.match(reviewRewardSummary(r), /\+100 ₽ на баланс.*личный промокод/);
});
test("disabled components are cleared while editing values can be kept", () => {
  const d=reviewRewardDraft(); d.daysEnabled=false; d.trafficEnabled=false; d.promo.rewardValue=999;
  const r=reviewRewardsFromDraft(d);
  assert.equal(r.days,0); assert.equal(r.trafficGb,0); assert.equal(r.promo.rewardValue,0); assert.equal(reviewRewardSummary(r),"");
});
test("reject invalid numeric rewards and promo configuration", () => {
  for (const value of ["", -1, 0, 1.5, 3651, Infinity, "1e100"]) { const d=reviewRewardDraft(); d.days=value; assert.throws(()=>reviewRewardsFromDraft(d)); }
  const d=reviewRewardDraft(); d.promo.enabled=true; d.promo.discountPercent=100;
  assert.throws(()=>reviewRewardsFromDraft(d));
  d.promo.discountPercent=20; d.promo.expiryDays=-1;
  assert.throws(()=>reviewRewardsFromDraft(d));
});

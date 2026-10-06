import test from "node:test";
import assert from "node:assert/strict";
import { reuseAdminUserRows } from "./static/stable-user-rows.mjs";

function row(id, text, avatarText = "avatar") {
  const avatar = { outerHTML: avatarText, replaceWith(node) { this.reused = node; } };
  return { dataset: {action: "admin-user-open", value: String(id)}, outerHTML: text, avatar,
    querySelector: () => avatar, replaceWith(node) { this.reused = node; } };
}
test("unchanged rows reuse their mounted DOM, even when reordered or more users appear", () => {
  const old = [row(1,"first"),row(2,"second")];
  const next = [row(2,"second"),row(3,"new"),row(1,"first")];
  reuseAdminUserRows({querySelectorAll:()=>old},{querySelectorAll:()=>next});
  assert.equal(next[0].reused,old[1]);
  assert.equal(next[2].reused,old[0]);
  assert.equal(next[1].reused,undefined);
});
test("role and balance changes update the row while preserving an unchanged avatar", () => {
  const old=row(1,"old role"), next=row(1,"new role");
  reuseAdminUserRows({querySelectorAll:()=>[old]},{querySelectorAll:()=>[next]});
  assert.equal(next.reused,undefined);
  assert.equal(next.avatar.reused,old.avatar);
  const changedPhoto=row(1,"new role and photo","new-photo");
  reuseAdminUserRows({querySelectorAll:()=>[old]},{querySelectorAll:()=>[changedPhoto]});
  assert.equal(changedPhoto.avatar.reused,undefined);
});

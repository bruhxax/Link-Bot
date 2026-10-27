import test from "node:test";
import assert from "node:assert/strict";
import {cursorValues,safeCursorURL} from "./static/custom-cursor.mjs";
import {renderAudienceEditor,renderLandingEditor,renderCursorEditor} from "./static/surface-editors.mjs";
const escape=value=>String(value).replace(/[&<>"']/g,c=>({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'}[c]));
const helpers={escapeHtml:escape,escapeAttribute:escape,renderAdminColorField:()=>""};
test("custom cursor rejects executable URLs and uses the correct click point",async()=>{
 assert.equal(safeCursorURL('javascript:alert(1)'),"");assert.equal(safeCursorURL('data:text/html,x'),"");
 assert.match(safeCursorURL('/mini-app/uploads/cursor-abc.png'),/^https:\/\/local.invalid\/mini-app/);
 const paw=await cursorValues({mode:'paw',size:48});assert.match(paw.normal,/24 24, auto$/);assert.match(paw.pointer,/24 24, pointer$/);
 const cur=await cursorValues({mode:'custom',url:'https://example.com/test.cur',size:32,hotspotX:5,hotspotY:7});assert.match(cur.normal,/5 7, auto$/);
 assert.deepEqual(await cursorValues({mode:'system'}),{normal:'auto',pointer:'pointer'});
});
test("audience editor preserves squad identities and escapes panel text",()=>{
 const id='11111111-1111-4111-8111-111111111111';
 const html=renderAudienceEditor({category:'trial',language:'ru',tag:'<script>',internalSquads:[id]}, {internal:[{uuid:id,name:'<img onerror="bad">'}]}, 15,false,'telegram',helpers);
 assert.match(html,new RegExp(`value="${id}" checked`));assert.match(html,/15 получателей/);assert.match(html,/value="trial" selected/);assert.doesNotMatch(html,/<script>|<img onerror/);
 const running=renderAudienceEditor({category:'active'},null,null,true,'email',helpers);assert.doesNotMatch(running,/admin-audience-check/);assert.match(running,/data-audience-field="category" data-channel="email" disabled/);
});
test("surface editors expose separate cursors and per-section backgrounds",()=>{
 const a={colors:{accent:'#123456'},landing:{text:{heroTitle:'<script>unsafe</script>'}}};
 const cursors=renderCursorEditor(a,'admin',helpers);assert.match(cursors,/appearance.cursors.admin.url/);assert.match(cursors,/data-value="paw"/);assert.match(cursors,/data-cursor-kind="pointerUrl"/);
 a.landing.perPage=true;const backgrounds=renderLandingEditor(a,'backgrounds',helpers);for(const id of ['home','servers','plans','support'])assert.ok(backgrounds.includes(`appearance.landing.pages.${id}.mode`));
 assert.doesNotMatch(renderLandingEditor(a,'text',helpers),/<script>/);
});

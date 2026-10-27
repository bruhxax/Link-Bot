import test from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';
import vm from 'node:vm';
import {renderAudienceEditor} from './static/surface-editors.mjs';

const source = fs.readFileSync(new URL('./static/app.js', import.meta.url), 'utf8');
function load(context, name) {
  const start = source.indexOf(`function ${name}(`), end = source.indexOf('\nfunction ', start + 1);
  assert.ok(start >= 0 && end > start, name);
  vm.runInContext(source.slice(start, end), context);
}
const escape = value => String(value).replace(/[&<>"']/g, c => ({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'}[c]));

test('unsubscribe override is a boolean switch, isolated per channel and invalidates confirmed audience', () => {
  const state = {adminBroadcastTab:'telegram', adminAudienceDraft:{}, adminAudienceCount:{telegram:40,email:20}, adminBroadcast:{audience:{category:'paid',ignoreUnsubscribe:false}}, adminBroadcastConfirmOpen:true, adminEmailConfirmOpen:true};
  const context = vm.createContext({scheduleAdminBroadcastSave(){},flushAdminBroadcastAutoSave:async()=>{},discardAdminBroadcastAutoSave:async()=>{},adminBroadcastWorkflowMode:()=>"message",state, deepClone:structuredClone, render(){}});
  load(context, 'currentBroadcastAudience'); load(context, 'updateBroadcastAudience'); load(context, 'updateConsoleAudience');
  context.updateBroadcastAudience({type:'checkbox', checked:true, dataset:{channel:'telegram',audienceField:'ignoreUnsubscribe'}, closest(){return null;}});
  assert.equal(state.adminAudienceDraft.telegram.ignoreUnsubscribe,true);
  assert.equal(state.adminAudienceDraft.telegram.category,'paid');
  assert.equal(state.adminAudienceCount.telegram,null); assert.equal(state.adminAudienceCount.email,20);
  assert.equal(state.adminBroadcastConfirmOpen,false); assert.equal(state.adminEmailConfirmOpen,false);
  context.updateConsoleAudience('ignoreUnsubscribe',false);
  assert.equal(state.adminAudienceDraft.telegram.ignoreUnsubscribe,false);
  state.adminBroadcast.status='running';
  context.updateConsoleAudience('ignoreUnsubscribe',true);
  assert.equal(state.adminAudienceDraft.telegram.ignoreUnsubscribe,false);
});

test('audience editor exposes disabled/checked override without changing other filters', () => {
  const helpers = {escapeHtml:escape, escapeAttribute:escape};
  for (const channel of ['telegram','email']) {
    const html = renderAudienceEditor({category:'paid',ignoreUnsubscribe:true}, null, 10, true, channel, helpers);
    if(channel === "email") { assert.doesNotMatch(html, /data-audience-field="ignoreUnsubscribe"/); continue; }
    assert.match(html, /role="switch" data-audience-field="ignoreUnsubscribe"/);
    assert.match(html, new RegExp(`data-audience-field="ignoreUnsubscribe" data-channel="${channel}" checked disabled`));
    assert.doesNotMatch(html, /Их настройки останутся прежними/);
  }
});

test('confirmation explicitly shows when opted-out recipients are included', () => {
  const state = {adminAudienceCount:{telegram:30,email:10}, adminBroadcast:{audience:{ignoreUnsubscribe:true}}, adminEmailBroadcast:{draft:{audience:{ignoreUnsubscribe:false}}}};
  const context = vm.createContext({scheduleAdminBroadcastSave(){},flushAdminBroadcastAutoSave:async()=>{},discardAdminBroadcastAutoSave:async()=>{},adminBroadcastWorkflowMode:()=>"message",state, icon:()=>'', escapeAttribute:escape});
  load(context,'renderAdminBroadcastConfirm'); load(context,'renderAdminEmailBroadcastConfirm');
  assert.match(context.renderAdminBroadcastConfirm(false), /Эта рассылка придёт и отписавшимся/);
  assert.doesNotMatch(context.renderAdminEmailBroadcastConfirm(false), /Эта рассылка придёт и отписавшимся/);
  state.adminEmailBroadcast.draft.audience.ignoreUnsubscribe=true;
  assert.doesNotMatch(context.renderAdminEmailBroadcastConfirm(false), /Эта рассылка придёт и отписавшимся/);
});

test('start content editor contains only visibility and both button forms', () => {
  const context = vm.createContext({scheduleAdminBroadcastSave(){},flushAdminBroadcastAutoSave:async()=>{},discardAdminBroadcastAutoSave:async()=>{},
    renderAdminSettingField:(label,path)=>`<input data-setting-path="${path}">`,
    renderAdminToggle:(label,path)=>`<input data-setting-path="${path}">`,
    renderAdminTelegramButton:(label,path)=>`<input data-setting-path="${path}.text"><input data-setting-path="${path}.iconCustomEmojiId"><input data-setting-path="${path}.style">`,
    renderAdminLogoField:()=>'', renderAdminBannerField:()=>'',
    renderAdminContentTestButton:kind=>`<button data-value="${kind}"></button>`,
    renderAdminPinnedMessage:()=>'',
  });
  load(context,'renderAdminStartContent');
  const html = context.renderAdminStartContent();
  for(const path of ['content.startMenu.notificationsButton.enabled','content.startMenu.notificationsButton.iconCustomEmojiId','content.startMenu.notificationsButton.style','content.startMenu.notificationsOffButton.text','content.startMenu.notificationsOffButton.iconCustomEmojiId','content.startMenu.notificationsOffButton.style']) assert.ok(html.includes(`data-setting-path="${path}"`),path);
  assert.doesNotMatch(html, /content.notifications|notifications-enabled|notifications-disabled|Настройки рассылок пользователя/);
});

import test from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';
import vm from 'node:vm';

const source = fs.readFileSync(new URL('./static/app.js', import.meta.url), 'utf8');
function load(context, name) {
  let start = source.indexOf(`function ${name}(`);
  assert.ok(start >= 0, name);
  if (source.slice(start - 6, start) === 'async ') start -= 6;
  const next = source.slice(start + 1).search(/\n(?:async )?function /);
  vm.runInContext(source.slice(start, start + 1 + next), context);
}

function harness(fields = {}) {
  const draft = { enabled: true, fields: { apiKey: 'live_fixture', ...fields } };
  const item = { id: 'smerch', name: 'Smerch', logo: 'smerch.png', kind: 'payment', fields: [
    { key: 'apiKey', label: 'API-ключ', secret: true, required: true },
    { key: 'paymentMethods', label: 'Способы оплаты' },
    { key: 'paymentMethod', label: 'Ранее выбранный метод' },
    { key: 'webhookSecret', label: 'Секрет вебхука (необязательно)', secret: true },
    { key: 'webhookKeyId', label: 'ID ключа вебхука' },
  ] };
  const state = { adminIntegrationOpen: 'smerch', adminIntegrationDrafts: { smerch: draft }, data: { admin: { integrations: [item] }, paymentMethods: [] } };
  const requests = [], renders = [];
  const context = vm.createContext({ state, draft, item, escapeHtml: String, escapeAttribute: String, icon: () => '', integrationDraft: () => draft,
    render: options => renders.push(options), showToast: () => {},
    post: async (url, body) => {
      requests.push({ url, body });
      return { data: { ...item, enabled: true, fields: item.fields.map(field => ({ ...field, value: field.secret ? undefined : draft.fields[field.key], configured: field.secret })) }, paymentMethods: [{ id: 'smerch' }], paymentChannels: { smerch: { defaultChannels: ['sbp', 'crypto'] } } };
    },
  });
  for (const name of ['smerchIntegrationMethods', 'updateSmerchIntegrationMethod', 'syncSmerchIntegrationDraft', 'renderSmerchIntegrationMethods', 'renderSmerchAdvancedFields', 'renderAdminIntegrationField', 'renderAdminIntegrationRow', 'saveAdminIntegration']) load(context, name);
  return { context, state, draft, item, requests, renders };
}

test('Smerch offers both checkboxes and keeps optional signature fields collapsed', () => {
  const h = harness();
  const html = h.context.renderAdminIntegrationRow(h.item);
  assert.match(html, /data-smerch-method="sbp" checked/);
  assert.match(html, /data-smerch-method="crypto" checked/);
  assert.match(html, /<details class="admin-smerch-advanced"><summary>/);
  assert.match(html, /Заполнять необязательно/);
  assert.doesNotMatch(html, /data-integration-field="paymentMethod"/);
  assert.doesNotMatch(html, /Секрет вебхука \*/);
});

test('legacy choice survives until changed and both rails are saved as a list', async () => {
  const h = harness({ paymentMethod: 'sbp' });
  assert.deepEqual([...h.context.smerchIntegrationMethods(h.draft)], ['sbp']);
  h.state.adminPaymentChannelCatalog = { smerch: { defaultChannels: ['sbp'] } };
  h.context.updateSmerchIntegrationMethod(h.draft, 'crypto', true);
  await h.context.saveAdminIntegration('smerch');
  assert.deepEqual(JSON.parse(h.requests[0].body.fields.paymentMethods), ['sbp', 'crypto']);
  assert.deepEqual(h.state.data.paymentChannels.smerch.defaultChannels, ['sbp', 'crypto']);
  assert.deepEqual(h.state.adminPaymentChannelCatalog.smerch.defaultChannels, ['sbp', 'crypto']);
  assert.equal(h.state.adminIntegrationDrafts.smerch.fields.apiKey, '');
  assert.ok(h.renders.every(options => options.preserveScroll));
});

test('unchecking rails and editing additional methods never restores an unchecked rail', () => {
  const h = harness();
  h.context.updateSmerchIntegrationMethod(h.draft, 'crypto', false);
  h.draft.smerchOtherMethods = 'card_international, card_international';
  h.context.syncSmerchIntegrationDraft(h.draft);
  assert.deepEqual(JSON.parse(h.draft.fields.paymentMethods), ['sbp', 'card_international']);
  h.context.updateSmerchIntegrationMethod(h.draft, 'sbp', false);
  h.draft.smerchOtherMethods = '';
  h.context.syncSmerchIntegrationDraft(h.draft);
  assert.deepEqual(JSON.parse(h.draft.fields.paymentMethods), []);
});

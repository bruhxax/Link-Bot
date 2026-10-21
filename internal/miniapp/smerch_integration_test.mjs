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
  for (const name of ['renderSmerchAdvancedFields', 'renderAdminIntegrationField', 'renderAdminIntegrationRow', 'saveAdminIntegration']) load(context, name);
  return { context, state, draft, item, requests, renders };
}

test('Smerch integration has credentials and no duplicate method controls', () => {
  const h = harness({ paymentMethod: 'sbp', paymentMethods: '["sbp","crypto"]' });
  const html = h.context.renderAdminIntegrationRow(h.item);
  assert.match(html, /data-integration-field="apiKey"/);
  assert.match(html, /<details class="admin-smerch-advanced"><summary>/);
  assert.match(html, /Заполнять необязательно/);
  assert.doesNotMatch(html, /data-smerch-method|data-smerch-other-methods|data-integration-field="paymentMethods?"/);
  assert.doesNotMatch(html, /Секрет вебхука \*/);
});

test('saving Smerch does not introduce method defaults or require a secret', async () => {
  const h = harness();
  await h.context.saveAdminIntegration('smerch');
  assert.equal(h.requests.length, 1);
  assert.equal(h.requests[0].body.fields.paymentMethods, undefined);
  assert.equal(h.requests[0].body.fields.webhookSecret, undefined);
  assert.equal(h.state.adminIntegrationDrafts.smerch.fields.apiKey, '');
  assert.ok(h.renders.every(options => options.preserveScroll));
});

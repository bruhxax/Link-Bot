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
function harness() {
  const fields = [
    { key: 'apiKey', label: 'API-ключ', secret: true, required: true },
    { key: 'webhookSecret', label: 'Секрет вебхука', secret: true, required: true },
    { key: 'rubPerUsd', label: 'Курс ₽/USD', required: true },
  ];
  const item = { id: 'laveshka', name: 'Лавешка', kind: 'payment', logo: 'laveshka.svg', enabled: true, configured: true, fields };
  const draft = { enabled: true, fields: { apiKey: 'LAVE-12-fixture', webhookSecret: 'whsec_fixture', rubPerUsd: '100' } };
  const settings = { features: { stars: false }, paymentMethodVariants: {}, paymentMethodText: {} };
  const state = { adminIntegrationOpen: item.id, adminIntegrationDrafts: { laveshka: draft }, data: { admin: { integrations: [item] }, paymentMethods: [] } };
  const requests = [];
  const c = vm.createContext({ state, settings, previewMode: false, PAYMENT_LOGO_URLS: { laveshka: 'laveshka.svg' }, localizedText: ru => ru, t: () => ({}), getRuntimeSettings: () => settings,
    escapeHtml: String, escapeAttribute: String, icon: () => '', integrationDraft: () => draft, formatMoneyCents: String, render() {}, showToast() {},
    post: async (url, body) => { requests.push({ url, body }); return { data: { ...item, fields: fields.map(field => ({ ...field, value: field.secret ? undefined : draft.fields[field.key], configured: true })) }, paymentMethods: [{ id: item.id }], paymentChannels: { laveshka: { options: [] } } }; },
  });
  for (const name of ['renderAdminIntegrationField', 'renderAdminIntegrationRow', 'saveAdminIntegration', 'paymentMethodBase', 'paymentMethodMeta', 'sortConfiguredPaymentMethods', 'getAdminPaymentMethods', 'paymentHistoryMethodMeta']) load(c, name);
  return { c, item, draft, settings, state, requests };
}

test('Laveshka reuses compact integration fields without a second method selector', () => {
  const { c, item } = harness();
  const html = c.renderAdminIntegrationRow(item);
  for (const key of ['apiKey', 'webhookSecret', 'rubPerUsd']) assert.match(html, new RegExp(`data-integration-field="${key}"`));
  assert.doesNotMatch(html, /data-integration-field="paymentMethod|data-smerch-method/);
});

test('Laveshka is offered only when configured and enabled, including plan copies', () => {
  const { c, item, settings } = harness();
  assert.deepEqual(Array.from(c.getAdminPaymentMethods(), x => x.id), ['laveshka']);
  settings.paymentMethodVariants.laveshka__aabbccdd = { method: 'laveshka', channels: [] };
  assert.deepEqual(Array.from(c.getAdminPaymentMethods(), x => x.id), ['laveshka', 'laveshka__aabbccdd']);
  const meta = c.paymentMethodMeta('laveshka__aabbccdd');
  assert.equal(meta.logo, 'laveshka.svg'); assert.equal(meta.label, 'Лавешка'); assert.match(meta.hint, /USD/);
  assert.equal(c.paymentHistoryMethodMeta({ invoiceType: 'laveshka' }, {}).label, 'Лавешка');
  item.enabled = false; assert.equal(c.getAdminPaymentMethods().length, 0);
  item.enabled = true; item.configured = false; assert.equal(c.getAdminPaymentMethods().length, 0);
});

test('saving Laveshka clears secret inputs while retaining the configured rate', async () => {
  const { c, requests, state } = harness();
  await c.saveAdminIntegration('laveshka');
  assert.equal(requests.length, 1);
  assert.equal(requests[0].body.fields.rubPerUsd, '100');
  assert.equal(state.adminIntegrationDrafts.laveshka.fields.apiKey, '');
  assert.equal(state.adminIntegrationDrafts.laveshka.fields.webhookSecret, '');
  assert.equal(state.adminIntegrationDrafts.laveshka.fields.rubPerUsd, '100');
});

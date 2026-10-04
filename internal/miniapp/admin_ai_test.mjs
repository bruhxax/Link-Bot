import test from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';
import vm from 'node:vm';
import { renderAISettings } from './static/admin-ai.mjs';

const escape = value => String(value ?? '').replace(/[&<>"']/g, value => ({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'}[value]));
const helpers = {escapeHtml:escape, escapeAttribute:escape, icon:() => '<svg></svg>'};

test('provider model identifiers and prompt cannot inject HTML into admin UI', () => {
	const model = '<img src=x onerror=alert(1)>';
	const result = renderAISettings({draft:{apiUrl:'https://provider.example/v1',apiKey:'',model,prompt:'</textarea><script>alert(1)</script>',enabled:true,keyConfigured:true},models:[model],verified:true},helpers);
	assert.ok(!result.includes('<img src=x'));
	assert.ok(!result.includes('<script>'));
	assert.ok(result.includes('aria-checked="true"'));
	assert.ok(result.includes('type="password"'));
	assert.ok(result.includes('Ключ сохранён'));
});

test('settings errors provide retry without losing form availability', () => {
	const result = renderAISettings({draft:null,error:'Сервер недоступен'},helpers);
	assert.ok(result.includes('role="alert"'));
	assert.ok(result.includes('data-action="admin-ai-load"'));
});

test('AI replies remain peer messages for both customer and admin viewers', () => {
	const source = fs.readFileSync(new URL('./static/app.js',import.meta.url),'utf8');
	const start = source.indexOf('function renderSupportMessage(message)');
	const end = source.indexOf('function renderSupportMessageAttachment',start);
	for (const isAdmin of [true,false]) {
		const sandbox = {state:{data:{support:{isAdmin}},activeSupportThread:{ticket:{customerName:'Клиент'}}},supportText:() => ({you:'Вы',admin:'Поддержка'}),localizedText:ru => ru,escapeHtml:escape,renderSupportMessageBody:escape,formatSupportTime:()=> '12:00'};
		vm.createContext(sandbox);
		vm.runInContext(source.slice(start,end),sandbox);
		const result = sandbox.renderSupportMessage({id:1,authorRole:'ai',body:'Здравствуйте',createdAt:''});
		assert.ok(result.includes('ИИ-помощник'));
		assert.ok(result.includes('support-message--peer'));
		assert.ok(!result.includes('support-message--mine'));
	}
});

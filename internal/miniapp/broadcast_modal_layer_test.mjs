import assert from 'node:assert/strict';
import fs from 'node:fs';
import vm from 'node:vm';
import test from 'node:test';

const source = fs.readFileSync(new URL('./static/app.js', import.meta.url), 'utf8');
function functionSource(name) {
  const start = source.indexOf(`function ${name}(`);
  const end = source.indexOf('\nfunction ', start + 1);
  assert.ok(start >= 0 && end > start, `missing ${name}`);
  return source.slice(start, end);
}
const shellStart = source.indexOf('mountCabinetShell(`') + 'mountCabinetShell('.length;
const shellEnd = source.indexOf('\n  `);', shellStart) + '\n  `'.length;
const shellTemplate = source.slice(shellStart, shellEnd);

function setup(channel) {
  const state = {currentPage: 'admin', adminSection: 'broadcast', locale: 'ru',
    adminBroadcastTab: channel, adminBroadcastConfirmOpen: true, adminEmailConfirmOpen: true,
    adminAudienceCount: {telegram: 30, email: 12},
    adminBroadcast: {status: 'idle', sourceKind: 'text', sourcePreview: 'Test message'},
    adminBroadcastButtonsDraft: [], adminSMTP: {}, adminEmailEditorTab: 'message',
    adminEmailBroadcast: {configured: true, draft: {status: 'idle'}},
    adminEmailDraftSubject: 'Test subject', adminEmailDraftBody: 'Test email',
  };
  let allowed = true;
  const context = vm.createContext({state, Number,
    canAdmin: () => allowed, pageClass: () => 'active page--animate',
    escapeHtml: String, escapeAttribute: String, icon: () => '',
    broadcastStatusLabel: () => 'Draft', broadcastKindLabel: () => 'Text',
    renderAdminBroadcastTabs: () => '', renderCurrentAudience: () => '',
    consoleActive: false, dockMode: 'navigation', dockModeChanged: false,
    renderDesktopSidebar: () => '', isModalVisible: () => false,
    renderBottomNav: () => '<nav data-test-bottom-nav></nav>',
  });
  for (const name of ['renderAdminBroadcastAutoReply', 'renderAdminBroadcastExpiration', 'renderAdminBroadcastPage', 'renderAdminEmailBroadcastPage',
    'renderAdminBroadcastConfirmation', 'renderAdminEmailBroadcastConfirm', 'renderAdminBroadcastConfirm']) {
    vm.runInContext(functionSource(name), context);
  }
  context.renderPages = () => context.renderAdminBroadcastPage();
  return {state, context, deny: () => {allowed = false;}};
}

test('Telegram and email confirmation dialogs are outside the animated scrolling page and above navigation', () => {
  for (const channel of ['telegram', 'email']) {
    const {context} = setup(channel);
    const page = context.renderAdminBroadcastPage();
    assert.match(page, /active page--animate/);
    assert.doesNotMatch(page, /role="dialog"|class="modal open"/);
    const shell = vm.runInContext(shellTemplate, context);
    assert.equal((shell.match(/role="dialog"/g) || []).length, 1);
    assert.ok(shell.indexOf('class="modal open"') > shell.indexOf('<nav data-test-bottom-nav>'));
    assert.match(shell, new RegExp(`data-action="admin-${channel === 'email' ? 'email' : 'broadcast'}-close-confirm"`));
    assert.match(shell, new RegExp(`data-action="admin-${channel === 'email' ? 'email' : 'broadcast'}-send"`));
    assert.match(shell, channel === 'email' ? /12/ : /30/);
  }
});

test('closed or stale confirmations cannot cover another channel, page or an unauthorized view', () => {
  const {state, context, deny} = setup('telegram');
  state.adminBroadcastConfirmOpen = false;
  assert.equal(context.renderAdminBroadcastConfirmation(), ''); // email flag belongs to another channel
  state.adminBroadcastConfirmOpen = true;
  state.adminSection = 'home';
  assert.equal(context.renderAdminBroadcastConfirmation(), '');
  state.adminSection = 'broadcast'; state.currentPage = 'dashboard';
  assert.equal(context.renderAdminBroadcastConfirmation(), '');
  state.currentPage = 'admin'; deny();
  assert.equal(context.renderAdminBroadcastConfirmation(), '');
});

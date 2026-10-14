import assert from 'node:assert/strict';
import fs from 'node:fs';
import vm from 'node:vm';
import test from 'node:test';

const source = fs.readFileSync(new URL('./static/app.js', import.meta.url), 'utf8');
const section = (from, to) => source.slice(source.indexOf(from), source.indexOf(to, source.indexOf(from)));
function setup({google = true, clientID = 'configured.apps.googleusercontent.com', email = true} = {}) {
  const context = vm.createContext({state: {data: {user: {}}, locale: 'ru', loginMethodBusy: ''},
    googleMetaClientID: clientID, emailAuth: {stage: 'credentials'},
    featureEnabled: name => name === 'google' ? google : email,
    browserAuthBrand: () => ({name: 'Test VPN', logoUrl: '/logo.png'}), browserAuthCopy: () => ({title: 'Login'}),
    escapeHtml: String, escapeAttribute: String, icon: String, localizedText: x => x, emailAuthText: x => x,
    renderBrowserAuthQR: () => '<div>QR</div>', renderEmailAuth: () => '<form>Email</form>', renderEmailLink: () => '',
    telegramLoginButtonLabel: () => 'Telegram', gmailLoginButtonLabel: () => 'Google',
    gmailLabel: () => 'Google', telegramLabel: () => 'Telegram', gmailLinkHint: () => 'Link Google',
    authLinkedLabel: () => 'Linked', authNotLinkedLabel: () => 'Not linked', gmailLinkButtonLabel: () => 'Link Google',
    pageClass: () => '', showToast() {}, post() {throw new Error('disabled Google must never start linking');},
  });
  vm.runInContext(section('function getGoogleClientID()', 'function googleAuthCopy()') +
    section('function renderStateScreen(', 'function renderBottomNav(') +
    section('function renderLoginMethodsPage()', 'function renderGiftPage()'), context);
  return context;
}

test('disabled Google is absent on the login screen and in account linking even with a configured client ID', () => {
  for (const email of [true, false]) {
    const page = setup({google: false, email});
    assert.equal(page.getGoogleClientID(), '');
    assert.doesNotMatch(page.renderStateScreen('telegram'), /google-browser-login|google-login-widget/);
    assert.doesNotMatch(page.renderLoginMethodsPage(), /google-link-login|Link Google/);
    assert.match(page.renderStateScreen('telegram'), /telegram-browser-login/);
  }
});

test('enabled and configured Google renders login and linking; missing ID never renders a dead login button', () => {
  const enabled = setup();
  assert.match(enabled.renderStateScreen('telegram'), /google-browser-login/);
  assert.match(enabled.renderStateScreen('telegram'), /google-login-widget/);
  assert.match(enabled.renderLoginMethodsPage(), /google-link-login/);
  const missing = setup({clientID: ''});
  assert.doesNotMatch(missing.renderStateScreen('telegram'), /google-browser-login/);
  assert.doesNotMatch(missing.renderLoginMethodsPage(), /google-link-login/);
});

test('disabled Google cannot start linking through a stale button', async () => {
  const page = setup({google: false});
  page.googleAuthCopy = () => ({loginUnavailable: 'disabled'});
  vm.runInContext(section('async function startGoogleLinkRedirect(', 'function handleGoogleCredential('), page);
  await page.startGoogleLinkRedirect();
  assert.equal(page.state.loginMethodBusy, '');
});

test('a failed Google SDK request can be retried without reloading the login form', async () => {
  const scripts = [];
  const page = vm.createContext({window: {}, googleLoginScriptPromise: null,
    document: {querySelector: () => scripts.at(-1),
      createElement: () => ({dataset: {}, remove() {scripts.splice(scripts.indexOf(this), 1);}}),
      head: {appendChild: script => scripts.push(script)}}});
  vm.runInContext(section('function loadGoogleLoginScript()', 'async function ensureGoogleLoginReady()'), page);
  const first = page.loadGoogleLoginScript();
  assert.equal(page.loadGoogleLoginScript(), first);
  scripts[0].onerror();
  await assert.rejects(first, /script failed/);
  assert.equal(scripts.length, 0);
  const retry = page.loadGoogleLoginScript();
  assert.notEqual(retry, first);
  assert.equal(scripts.length, 1);
  scripts[0].onload();
  await retry;
});

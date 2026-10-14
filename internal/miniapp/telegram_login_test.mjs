import assert from 'node:assert/strict';
import fs from 'node:fs';
import vm from 'node:vm';
import test from 'node:test';
import {createTelegramLogin} from './static/telegram-login.mjs';

function setup(path = '/plans?planId=unlimited') {
  const listeners = new Set(), timers = new Map(), popups = [], results = [];
  let nextTimer = 0;
  const browser = {
    location: new URL(path, 'https://my.example.com'),
    open(url) {
      const popup = {url, closed: false, close() {this.closed = true;}, focus() {}};
      popups.push(popup);
      return popup;
    },
    addEventListener: (_, fn) => listeners.add(fn),
    removeEventListener: (_, fn) => listeners.delete(fn),
    setTimeout: fn => {timers.set(++nextTimer, fn); return nextTimer;},
    clearTimeout: id => timers.delete(id),
  };
  const login = createTelegramLogin(browser);
  return {browser, login, listeners, timers, popups, results,
    open: () => login.auth({client_id: '12345678', lang: 'ru'}, data => results.push(data)),
    message(data, overrides = {}) {
      for (const fn of [...listeners]) fn({origin: 'https://oauth.telegram.org', source: popups.at(-1), data, ...overrides});
    },
    tick() {const [id, fn] = [...timers][0]; timers.delete(id); fn();},
  };
}

test('Telegram opens synchronously with a registered redirect URI on all cabinet routes', () => {
  for (const path of ['/dashboard', '/login', '/mini-app/', '/plans?planId=unlimited']) {
    const page = setup(path); page.open();
    assert.equal(page.popups.length, 1);
    const url = new URL(page.popups[0].url);
    assert.equal(url.origin + url.pathname, 'https://oauth.telegram.org/auth');
    assert.equal(url.searchParams.get('response_type'), 'post_message');
    assert.equal(url.searchParams.get('client_id'), '12345678');
    assert.equal(url.searchParams.get('redirect_uri'), 'https://my.example.com/mini-app/');
    assert.equal(url.searchParams.get('scope'), 'openid profile telegram:bot_access');
    assert.equal(url.searchParams.get('lang'), 'ru');
    assert.equal(page.browser.location.pathname + page.browser.location.search, path);
  }
});

test('only the Telegram origin and exact popup can supply a token, with one callback', () => {
  const page = setup(); page.open();
  const payload = {event: 'auth_result', result: 'signed.id.token'};
  page.message(payload, {origin: 'https://evil.example'});
  page.message(payload, {source: {}});
  page.message('broken JSON');
  page.message(null);
  page.message({event: 'other', result: 'wrong'});
  assert.equal(page.results.length, 0);
  page.message(JSON.stringify(payload));
  assert.deepEqual(page.results, [{id_token: 'signed.id.token'}]);
  assert.equal(page.listeners.size, 0);
  assert.equal(page.timers.size, 0);
  assert.ok(page.popups[0].closed);
  page.message(payload);
  assert.equal(page.results.length, 1);
});

test('cancelled, rejected and malformed results clean up so another attempt can work', () => {
  for (const payload of [{event: 'auth_result', error: 'access_denied'}, {event: 'auth_result', result: ''}, null]) {
    const page = setup(); page.open();
    if (payload) page.message(payload);
    else {page.popups[0].closed = true; page.tick();}
    assert.ok(page.results[0].error);
    assert.equal(page.listeners.size, 0);
    assert.equal(page.timers.size, 0);
    page.open();
    page.message({event: 'auth_result', result: 'new.id.token'});
    assert.equal(page.results[1].id_token, 'new.id.token');
  }
});

test('a superseded popup cannot authenticate a newer attempt; blocked popups do not leave listeners', () => {
  const page = setup(); page.open(); const old = page.popups[0]; page.open();
  assert.ok(old.closed);
  assert.equal(page.listeners.size, 1);
  page.message({event: 'auth_result', result: 'old'}, {source: old});
  assert.equal(page.results.length, 0);
  page.message({event: 'auth_result', result: 'current'});
  assert.equal(page.results[0].id_token, 'current');
  page.browser.open = () => null;
  assert.throws(page.open, /blocked/);
  assert.equal(page.listeners.size, 0);
  assert.equal(page.timers.size, 0);
});

test('Telegram attempts expire and invalid bot IDs cannot open a popup', t => {
  const now = Date.now();
  const clock = t.mock.method(Date, 'now', () => now);
  const page = setup(); page.open();
  clock.mock.mockImplementation(() => now + 5 * 60 * 1000);
  page.tick();
  assert.equal(page.results[0].error, 'popup_timeout');
  assert.ok(page.popups[0].closed);
  assert.equal(page.listeners.size, 0);
  assert.throws(() => page.login.auth({client_id: ''}, () => {}), /client ID/);
  assert.equal(page.popups.length, 1);
});

const app = fs.readFileSync(new URL('./static/app.js', import.meta.url), 'utf8');
const actions = app.slice(app.indexOf('function startTelegramBrowserLogin('), app.indexOf('async function completeTelegramLink('));
test('both login and account linking use the fixed popup flow without waiting for a third-party SDK', () => {
  const page = setup();
  const context = vm.createContext({telegramBrowserLogin: page.login, telegramBotID: '12345678', telegramLinkPending: false,
    state: {locale: 'ru', loginMethodBusy: ''}, window: {...page.browser, onTelegramAuth: data => page.results.push(data)},
    browserAuthCopy: () => ({}), emailAuthText: x => x, showToast() {}});
  vm.runInContext(actions, context);
  context.startTelegramBrowserLogin();
  assert.equal(page.popups.length, 1);
  page.message({event: 'auth_result', result: 'login.token'});
  assert.equal(page.results[0].id_token, 'login.token');
  context.startTelegramLink();
  assert.equal(page.popups.length, 2);
  assert.equal(context.telegramLinkPending, true);
  page.message({event: 'auth_result', result: 'link.token'});
  assert.equal(page.results[1].id_token, 'link.token');
});

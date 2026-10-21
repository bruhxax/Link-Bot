import test from 'node:test';
import assert from 'node:assert/strict';
import { landingLegalArticle, renderLandingLegal, mountLandingLegal } from './static/landing-legal.mjs';
import { TERMS_ARTICLE, PRIVACY_ARTICLE } from './static/legal-articles.mjs';

test('landing uses the complete shared cabinet documents and replaces public branding', () => {
  const data = { brand: { name: 'BruhVPN' }, contacts: [{ url: 'https://t.me/example_support' }] };
  for (const [id, fallback] of [['terms', TERMS_ARTICLE.ru], ['privacy', PRIVACY_ARTICLE.ru]]) {
    const article = landingLegalArticle(id, data);
    assert.equal(article.sections.length, fallback.sections.length);
    assert.equal(article.sections.at(-1).title, fallback.sections.at(-1).title);
    const html = renderLandingLegal(id, data);
    assert.ok(html.includes('BruhVPN'));
    assert.ok(html.includes('@example_support'));
    assert.ok(!html.includes('Link-Bot'));
    assert.ok(html.includes(article.footer));
  }
  assert.ok(TERMS_ARTICLE.ru.intro[0].includes('Link-Bot'));
  assert.ok(renderLandingLegal('privacy', { ...data, legalContact: '@custom_contact' }).includes('@custom_contact'));
  assert.equal(landingLegalArticle('unknown', data), null);
});

test('saved legal documents show every section and escape their text', () => {
  const end = 'Последний абзац без обрезания';
  const data = { legalDocuments: { privacy: {
    title: '<img src=x onerror=alert(1)>', jurisdiction: 'Тест & данные', intro: 'Вступление\nВторой абзац',
    sections: [{ title: 'Раздел', body: 'Длинный текст\n'.repeat(500) + end }],
    contactTitle: 'Контакты', contacts: 'Поддержка\nПочта', footer: 'Конец документа',
  } } };
  const html = renderLandingLegal('privacy', data);
  assert.ok(html.includes('&lt;img src=x onerror=alert(1)&gt;'));
  assert.ok(!html.includes('<img'));
  assert.ok(html.includes('Тест &amp; данные'));
  assert.ok(html.includes('<p>Второй абзац</p>'));
  assert.equal(html.match(/<p>Длинный текст<\/p>/g).length, 500);
  assert.ok(html.includes(end));
  assert.ok(html.includes('Конец документа'));
});

function harness(initial = 'https://example.com/?campaign=demo#support') {
  const events = {}, dialogEvents = {}, windowEvents = {};
  const entries = [new URL(initial)]; let index = 0, focus;
  const dialog = { open: false, showModal() { this.open = true; }, close() { this.open = false; }, addEventListener(name, cb) { dialogEvents[name] = cb; } };
  const article = { innerHTML: '' }, scroller = { scrollTop: 0 };
  const heading = { focus(options) { focus = ['heading', options]; } };
  const document = { title: 'BruhVPN — лендинг', body: { style: { overflow: 'auto' } }, activeElement: null,
    getElementById(id) { return { 'landing-legal': dialog, 'landing-legal-article': article, 'landing-legal-scroll': scroller, 'landing-legal-title': heading }[id]; },
    addEventListener(name, cb) { events[name] = cb; },
  };
  const window = { get location() { return entries[index]; }, addEventListener(name, cb) { windowEvents[name] = cb; }, history: {
    pushState(_, __, url) { entries.splice(++index); entries[index] = new URL(url, entries[index - 1]); },
    replaceState(_, __, url) { entries[index] = new URL(url, entries[index]); },
    back() { if (index > 0) { index--; windowEvents.popstate?.(); windowEvents.hashchange?.(); } },
  } };
  const controller = mountLandingLegal({ document, window });
  const click = id => {
    const link = { dataset: { legalDocument: id }, focus(options) { focus = [id, options]; } };
    events.click({ button: 0, target: { closest(selector) { return selector === '[data-legal-document]' ? link : null; } }, preventDefault() {} });
  };
  const close = () => events.click({ target: { closest(selector) { return selector === '[data-legal-close]' ? {} : null; } }, preventDefault() {} });
  return { dialog, article, scroller, document, window, controller, click, close, dialogEvents, get focus() { return focus; } };
}

test('open and close restore the landing route, scrolling container and keyboard focus', () => {
  const h = harness();
  h.click('terms');
  assert.equal(h.dialog.open, true);
  assert.equal(h.document.body.style.overflow, 'hidden');
  assert.match(h.article.innerHTML, /Пользовательское соглашение/);
  h.scroller.scrollTop = 300;
  h.close();
  assert.equal(h.dialog.open, false);
  assert.equal(h.window.location.href, 'https://example.com/?campaign=demo#support');
  assert.equal(h.document.body.style.overflow, 'auto');
  assert.equal(h.document.title, 'BruhVPN — лендинг');
  assert.deepEqual(h.focus, ['terms', { preventScroll: true }]);
  h.click('privacy');
  assert.equal(h.scroller.scrollTop, 0);
  h.window.history.back();
  assert.equal(h.dialog.open, false);
});

test('direct document links close within the landing and Escape closes a pushed entry', () => {
  const direct = harness('https://example.com/?campaign=demo#privacy');
  assert.equal(direct.dialog.open, true);
  direct.close();
  assert.equal(direct.window.location.href, 'https://example.com/?campaign=demo');
  assert.equal(direct.dialog.open, false);
  const h = harness(); h.click('privacy');
  let prevented = false;
  h.dialogEvents.cancel({ preventDefault() { prevented = true; } });
  assert.equal(prevented, true);
  assert.equal(h.dialog.open, false);
});

test('background refresh updates configured text without resetting document scroll or return title', () => {
  const h = harness(); h.click('privacy'); h.scroller.scrollTop = 340;
  h.document.title = 'Новое имя — лендинг';
  h.controller.setData({ brand: { name: 'Новое имя' }, legalDocuments: { privacy: { title: 'Моя политика', intro: 'Сохранённый текст', sections: [] } } });
  assert.ok(h.article.innerHTML.includes('Сохранённый текст'));
  assert.equal(h.scroller.scrollTop, 340);
  h.controller.setData({ brand: { name: 'Новое имя' } });
  h.close();
  assert.equal(h.document.title, 'Новое имя — лендинг');
});

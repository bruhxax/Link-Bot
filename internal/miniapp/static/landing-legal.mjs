import { TERMS_ARTICLE, PRIVACY_ARTICLE } from './legal-articles.mjs';

const labels = { terms: 'Пользовательское соглашение', privacy: 'Политика конфиденциальности' };
const escape = value => String(value ?? '').replace(/[&<>"']/g, char => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' })[char]);
const paragraphs = value => String(value || '').split(/\n\s*\n|\n/).map(part => part.trim()).filter(Boolean);

export function landingLegalArticle(id, data = {}) {
  if (!Object.hasOwn(labels, id)) return null;
  const fallback = id === 'privacy' ? PRIVACY_ARTICLE.ru : TERMS_ARTICLE.ru;
  const document = data.legalDocuments?.[id];
  const article = document?.title ? {
    title: document.title, effectiveLabel: document.effectiveLabel || 'Дата вступления в силу',
    jurisdiction: document.jurisdiction || '', intro: paragraphs(document.intro),
    sections: (document.sections || []).map(section => ({ title: section.title || '', paragraphs: paragraphs(section.body) })),
    contactTitle: document.contactTitle || 'Связь с администрацией', contacts: paragraphs(document.contacts), footer: document.footer || '',
  } : fallback;
  const brand = String(data.brand?.name || 'Link-Bot').trim() || 'Link-Bot';
  const contact = (data.contacts || []).map(item => String(item.url || '').match(/^https?:\/\/t\.me\/([A-Za-z0-9_]{5,32})\/?$/i)?.[1]).find(Boolean);
  const legalContact = String(data.legalContact || (contact ? `@${contact}` : '@your_support_username')).trim();
  const replace = value => {
    if (Array.isArray(value)) return value.map(replace);
    if (value && typeof value === 'object') return Object.fromEntries(Object.entries(value).map(([key, part]) => [key, replace(part)]));
    return typeof value === 'string' ? value.replaceAll('Link-Bot', brand).replaceAll('@your_support_username', legalContact) : value;
  };
  return replace(article);
}

export function renderLandingLegal(id, data = {}, now = new Date()) {
  const article = landingLegalArticle(id, data);
  if (!article) return '';
  const p = text => `<p>${escape(text)}</p>`;
  const list = items => items?.length ? `<ul>${items.map(item => `<li>${escape(item)}</li>`).join('')}</ul>` : '';
  return `<h1 id="landing-legal-title" tabindex="-1">${escape(article.title)}</h1>
    <p class="legal-article__meta">${escape(article.effectiveLabel)} — ${escape(now.toLocaleDateString('ru-RU'))}</p>
    ${article.jurisdiction ? `<p class="legal-article__lead">${escape(article.jurisdiction)}</p>` : ''}
    ${(article.intro || []).map(p).join('')}
    ${(article.sections || []).map(section => `<section>${section.title ? `<h2>${escape(section.title)}</h2>` : ''}${(section.paragraphs || []).map(p).join('')}${list(section.items)}</section>`).join('')}
    ${article.contacts?.length ? `<section><h2>${escape(article.contactTitle)}</h2>${list(article.contacts)}</section>` : ''}
    ${article.footer ? `<p class="legal-article__footer">${escape(article.footer)}</p>` : ''}`;
}

export function mountLandingLegal({ document = globalThis.document, window = globalThis.window } = {}) {
  const dialog = document.getElementById('landing-legal');
  const article = document.getElementById('landing-legal-article');
  const scroller = document.getElementById('landing-legal-scroll');
  let data = {}, active = '', originalTitle = '', originalOverflow = '', returnFocus = null;
  let ownEntry = false, rendered = '';
  const currentID = () => window.location.hash.slice(1);
  const draw = () => {
    const html = renderLandingLegal(active, data);
    if (html !== rendered) { article.innerHTML = html; rendered = html; }
  };
  const sync = () => {
    const id = currentID();
    if (!Object.hasOwn(labels, id)) {
      if (!dialog.open) return;
      dialog.close();
      document.body.style.overflow = originalOverflow;
      document.title = originalTitle;
      active = ''; ownEntry = false;
      returnFocus?.focus({ preventScroll: true });
      returnFocus = null;
      return;
    }
    const opening = !dialog.open;
    if (opening) {
      originalTitle = document.title;
      originalOverflow = document.body.style.overflow;
      returnFocus ||= document.activeElement;
    }
    if (active !== id) { active = id; rendered = ''; scroller.scrollTop = 0; }
    draw();
    document.title = `${labels[id]} — ${data.brand?.name || originalTitle.split(' — ')[0]}`;
    if (opening) {
      document.body.style.overflow = 'hidden';
      dialog.showModal();
      document.getElementById('landing-legal-title')?.focus({ preventScroll: true });
    }
  };
  const close = () => {
    if (ownEntry) window.history.back();
    else { window.history.replaceState(null, '', window.location.pathname + window.location.search); sync(); }
  };
  document.addEventListener('click', event => {
    const link = event.target.closest?.('[data-legal-document]');
    if (link) {
      if (event.button !== 0 || event.ctrlKey || event.metaKey || event.shiftKey || event.altKey) return;
      const id = link.dataset.legalDocument;
      if (!Object.hasOwn(labels, id)) return;
      event.preventDefault();
      returnFocus = link;
      window.history.pushState(null, '', `#${id}`);
      ownEntry = true;
      sync();
    }
    if (event.target.closest?.('[data-legal-close]')) { event.preventDefault(); close(); }
  });
  dialog.addEventListener('cancel', event => { event.preventDefault(); close(); });
  dialog.addEventListener('click', event => { if (event.target === dialog) close(); });
  window.addEventListener('popstate', sync);
  window.addEventListener('hashchange', sync);
  sync();
  return { setData(next) { data = next || {}; if (dialog.open) { if (!document.title.startsWith(labels[active] + " — ")) originalTitle = document.title; sync(); } } };
}

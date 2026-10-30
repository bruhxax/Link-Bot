const escapeHtml = value => String(value ?? '').replace(/[&<>"']/g, char => ({
  '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;'
}[char]));

const formatDate = value => value
  ? new Date(value).toLocaleString('ru-RU', {day: '2-digit', month: 'short', year: 'numeric', hour: '2-digit', minute: '2-digit'})
  : '—';

const labels = {
  sharing: {title: 'Возможный шаринг', words: ['IP-адрес', 'IP-адреса', 'IP-адресов']},
  trial: {title: 'Повторный триал', words: ['пробная подписка', 'пробные подписки', 'пробных подписок']}
};

function plural(count, words) {
  const remainder = count % 100;
  if (remainder >= 11 && remainder <= 14) return words[2];
  switch (count % 10) {
    case 1: return words[0];
    case 2: case 3: case 4: return words[1];
    default: return words[2];
  }
}

export function visibleAbuseItems(model) {
  const query = String(model.query || '').trim().toLowerCase();
  return (model.data?.items || []).filter(item => {
    if (model.filter !== 'all' && model.filter !== item.kind) return false;
    if (!query) return true;
    return [item.username, item.hwid, ...(item.ips || []).map(ip => ip.ip),
      ...(item.subscriptions || []).map(sub => sub.username)]
      .some(value => String(value || '').toLowerCase().includes(query));
  });
}

function emptyCopy(model) {
  if (model.data?.scanning) return ['Проверка выполняется', 'Результаты появятся после ответа панели'];
  if (model.query.trim() || model.filter !== 'all') return ['Ничего не найдено', 'Измените поиск или фильтр'];
  if (model.data?.checkedAt) return ['Подозрительных случаев нет', 'По текущим данным совпадений не обнаружено'];
  return ['Начинаем проверку', 'Получаем данные Remnawave'];
}

// Search and tabs only hide existing rows. Rebuilding the whole admin page on
// every keystroke reset the input and caused visible scroll jumps in WebView.
export function applyAbuseFilters(root, model) {
  if (!root) return;
  const visible = new Set(visibleAbuseItems(model).map(item => item.key));
  for (const card of root.querySelectorAll('[data-abuse-key]')) {
    const hidden = !visible.has(card.dataset.abuseKey);
    if (card.hidden !== hidden) card.hidden = hidden;
  }
  for (const tab of root.querySelectorAll('[data-action="abuse-filter"]')) {
    tab.setAttribute('aria-pressed', String(tab.dataset.value === model.filter));
  }
  const empty = root.querySelector('[data-abuse-empty]');
  if (empty) {
    empty.hidden = visible.size > 0;
    const [title, description] = emptyCopy(model);
    empty.querySelector('h2').textContent = title;
    empty.querySelector('p').textContent = description;
  }
}

// Coalesce rapid typing into a single DOM update before the next paint.
export function createAbuseFilterScheduler(frame = callback => requestAnimationFrame(callback)) {
  let pending = false;
  return (root, model) => {
    if (pending) return;
    pending = true;
    frame(() => {
      pending = false;
      applyAbuseFilters(root, model);
    });
  };
}

export function createAbuseController({post, render, isActive = () => true}) {
  const model = {data: null, busy: false, error: '', filter: 'all', query: '', open: new Set(), confirm: null, acting: '', loaded: false};
  let serial = 0;
  let timer;

  async function load({refresh = false, silent = false} = {}) {
    const request = ++serial;
    clearTimeout(timer);
    const previous = model.data;
    let shouldRender = !silent;
    model.busy = true;
    model.error = '';
    if (shouldRender && isActive()) render();
    try {
      const response = await post('/api/mini-app/admin/abuses/scan', {refresh});
      if (request !== serial || !isActive()) return;
      const incoming = response.data;
      shouldRender ||= !previous || previous.checkedAt !== incoming?.checkedAt ||
        previous.scanning !== incoming?.scanning || previous.error !== incoming?.error ||
        previous.failedChecks !== incoming?.failedChecks;
      model.data = incoming;
      model.loaded = true;
      if (incoming?.scanning) timer = setTimeout(() => {
        if (isActive()) void load({silent: true});
      }, 2200);
    } catch (error) {
      if (request === serial && isActive()) {
        model.error = error.message || 'Не удалось проверить подписки';
        shouldRender = true;
      }
    } finally {
      if (request === serial) {
        model.busy = false;
        if (shouldRender && isActive()) render();
      }
    }
  }

  function toggle(key, root) {
    if (model.open.has(key)) model.open.delete(key);
    else model.open.add(key);
    const card = [...(root?.querySelectorAll('[data-abuse-key]') || [])]
      .find(node => node.dataset.abuseKey === key);
    if (!card) return render();
    const open = model.open.has(key);
    const inside = card.querySelector('.ab-card__inside');
    const template = card.querySelector('.ab-card__template');
    if (open && inside && template) {
      inside.append(template.content.cloneNode(true));
      template.remove();
      // The closed grid row must be measured before its transition starts.
      void inside.offsetHeight;
    }
    card.classList.toggle('is-open', open);
    const head = card.querySelector('.ab-card__head');
    head?.setAttribute('aria-expanded', String(open));
    const currentLabel = head?.getAttribute?.('aria-label');
    if (currentLabel) head.setAttribute('aria-label', currentLabel.replace(/^(Открыть|Свернуть):/, `${open ? 'Свернуть' : 'Открыть'}:`));
    const details = card.querySelector('.ab-card__expand');
    if (open) details?.removeAttribute('inert');
    else details?.setAttribute('inert', '');
  }

  async function act(key, action) {
    const item = model.data?.items?.find(row => row.key === key);
    if (!item || model.acting) return;
    if (model.confirm?.key !== key || model.confirm?.action !== action) {
      model.confirm = {key, action};
      render();
      return;
    }
    model.acting = key;
    model.error = '';
    render();
    try {
      if (action === 'block') {
        await post('/api/mini-app/admin/users/block', {
          customerId: item.customerId, blocked: true,
          reason: item.kind === 'trial' ? 'Повторное использование HWID в пробных подписках' : 'IP-адресов больше лимита устройств'
        });
      } else if (action === 'revoke') {
        await post('/api/mini-app/admin/users/subscription/delete', {customerId: item.customerId, subscriptionId: item.subscriptionId});
      } else if (action === 'latest') {
        const subscription = item.subscriptions?.at(-1);
        if (!subscription) throw Error('Подписка не найдена');
        await post('/api/mini-app/admin/users/subscription/delete', {customerId: subscription.customerId, subscriptionId: subscription.subscriptionId});
      } else if (action === 'all') {
        for (const subscription of item.subscriptions || []) {
          await post('/api/mini-app/admin/users/subscription/delete', {customerId: subscription.customerId, subscriptionId: subscription.subscriptionId});
        }
      }
      model.confirm = null;
      await load({refresh: true});
    } catch (error) {
      model.error = error.message || 'Действие не выполнено';
    } finally {
      model.acting = '';
      render();
    }
  }

  return {model, load, toggle, act, cancel() { model.confirm = null; render(); }};
}

function evidenceRow(main, secondary, trailing = '') {
  return `<div class="ab-evidence__row"><span class="ab-evidence__content"><strong>${main}</strong><small>${secondary}</small></span>${trailing ? `<code>${trailing}</code>` : ''}</div>`;
}

function card(item, model, options, visible) {
  const trial = item.kind === 'trial';
  const open = model.open.has(item.key);
  const confirm = model.confirm?.key === item.key;
  const count = trial ? item.subscriptions?.length || 0 : item.ips?.length || 0;
  const {title, words} = labels[item.kind] || labels.sharing;
  const actions = trial
    ? [['block', 'Заблокировать', options.canBlock], ['latest', 'Отозвать последнюю подписку', options.canRevoke], ['all', 'Отозвать все подписки', options.canRevoke]]
    : [['block', 'Заблокировать', options.canBlock], ['revoke', 'Отозвать подписку', options.canRevoke]];
  const evidence = trial
    ? `<div class="ab-fact"><span>HWID устройства</span><code>${escapeHtml(item.hwid)}</code></div>
       <div class="ab-list-label">Пробные подписки с этим HWID</div>
       <div class="ab-evidence">${(item.subscriptions || []).map(sub => evidenceRow(
         `Подписка #${Number(sub.subscriptionId)}`,
         `${escapeHtml(sub.username)} · ${escapeHtml(formatDate(sub.createdAt))}`
       )).join('')}</div>`
    : `<div class="ab-fact"><span>Лимит устройств</span><strong>${Number(item.deviceLimit)}</strong></div>
       <div class="ab-list-label">Обнаруженные IP-адреса</div>
       <div class="ab-evidence">${(item.ips || []).map(ip => evidenceRow(
         `<code>${escapeHtml(ip.ip)}</code>`,
         `${escapeHtml(ip.node || 'Нода не указана')} · ${escapeHtml(formatDate(ip.lastSeen))}`
       )).join('')}</div>`;

  const details = `${evidence}
    <div class="ab-card__actions">${actions.filter(([, , allowed]) => allowed).map(([action, label]) => `<button type="button" data-action="abuse-act" data-value="${escapeHtml(item.key)}" data-abuse-operation="${action}" class="${action === 'block' ? 'ab-danger' : ''}" ${model.acting ? 'disabled' : ''}>${label}</button>`).join('')}</div>
    ${confirm ? `<div class="ab-confirm" role="group"><strong>${model.confirm.action === 'block' ? 'Заблокировать пользователя?' : model.confirm.action === 'all' ? 'Отозвать все совпадающие подписки?' : 'Отозвать подписку?'}</strong><span>Доступ будет отключён в панели Remnawave.</span><div><button type="button" data-action="abuse-cancel">Отмена</button><button type="button" data-action="abuse-act" data-value="${escapeHtml(item.key)}" data-abuse-operation="${escapeHtml(model.confirm.action)}" class="ab-confirm__yes" ${model.acting ? 'disabled' : ''}>${model.acting ? 'Выполняется…' : 'Подтвердить'}</button></div></div>` : ''}`;

  return `<article class="ab-card ${open ? 'is-open' : ''}" data-abuse-key="${escapeHtml(item.key)}" data-kind="${trial ? 'trial' : 'sharing'}" ${visible ? '' : 'hidden'}>
    <button class="ab-card__head" type="button" data-action="abuse-toggle" data-value="${escapeHtml(item.key)}" aria-expanded="${open}" aria-label="${open ? 'Свернуть' : 'Открыть'}: ${title}, ${escapeHtml(item.username || 'Пользователь')}, ${count} ${plural(count, words)}">
      <span class="ab-card__main"><strong>${title}</strong><small>${escapeHtml(item.username || 'Пользователь')}</small></span>
      <span class="ab-card__count" aria-label="${count} ${plural(count, words)}">${count} <small>${trial ? plural(count, ['подписка', 'подписки', 'подписок']) : 'IP'}</small></span><span class="ab-card__toggle" aria-hidden="true">${options.icons?.expand || ''}</span>
    </button>
    <div class="ab-card__expand" ${open ? '' : 'inert'}><div class="ab-card__inside">${open ? details : ''}</div></div>
    ${open ? '' : `<template class="ab-card__template">${details}</template>`}
  </article>`;
}

export function renderAbuseWorkspace(model, options = {}) {
  const data = model.data || {};
  const items = data.items || [];
  const visible = new Set(visibleAbuseItems(model).map(item => item.key));
  const sharing = items.filter(item => item.kind === 'sharing').length;
  const trial = items.filter(item => item.kind === 'trial').length;
  const [emptyTitle, emptyDescription] = emptyCopy(model);

  return `<div class="ab-workspace">
    <div class="ab-metrics" aria-label="Сводка абузов">
      <div class="ab-metric"><span>Всего</span><strong>${items.length}</strong></div>
      <div class="ab-metric"><span>Шаринг</span><strong>${sharing}</strong></div>
      <div class="ab-metric"><span>Триал</span><strong>${trial}</strong></div>
    </div>
    <div class="ab-toolbar">
      <label class="ab-search"><span class="ab-search__icon" aria-hidden="true">${options.icons?.search || ''}</span><input type="search" data-input="abuse-search" aria-label="Поиск абузов" placeholder="Поиск по пользователю, IP или HWID" autocomplete="off" value="${escapeHtml(model.query)}"></label>
      <div class="ab-tabs" role="group" aria-label="Тип нарушения">${[['all', 'Все'], ['sharing', 'Шаринг'], ['trial', 'Триал']].map(([key, label]) => `<button type="button" data-action="abuse-filter" data-value="${key}" aria-pressed="${model.filter === key}">${label}</button>`).join('')}</div>
    </div>
    ${data.scanning ? '<div class="ab-status is-loading"><span class="ab-pulse"></span><span>Проверяем подключения и устройства…</span></div>' : ''}
    ${data.error ? `<div class="ab-status is-error">${escapeHtml(data.error)}</div>` : ''}
    ${data.failedChecks ? `<div class="ab-status is-warning">Не удалось проверить ${Number(data.failedChecks)} источников данных. Список может быть неполным.</div>` : ''}
    ${model.error ? `<div class="ab-status is-error" role="alert">${escapeHtml(model.error)}</div>` : ''}
    <div class="ab-cards">${items.map(item => card(item, model, options, visible.has(item.key))).join('')}
      <div class="ab-empty" data-abuse-empty ${visible.size ? 'hidden' : ''}><h2>${emptyTitle}</h2><p>${emptyDescription}</p></div>
    </div>
    <footer class="ab-footer">${data.checkedAt ? `Проверено ${escapeHtml(formatDate(data.checkedAt))} · ${Number(data.checkedUsers || 0)} ${plural(Number(data.checkedUsers || 0), ['подписка', 'подписки', 'подписок'])}` : 'Ожидаем первую проверку'}</footer>
  </div>`;
}

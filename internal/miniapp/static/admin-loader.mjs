// Load the administration framework only after the authenticated admin entry.
let administration;
let pending;
let ready;

export function mountRemnaAdmin(element, model, shell, onReady) {
  if (administration) {
    administration.mountRemnaAdmin(element, model, shell);
    return;
  }
  ready = onReady;
  if (pending) return;
  element.innerHTML = '<div class="rn-admin-loader" role="status" aria-busy="true">Загрузка админки…</div>';
  pending = import('./admin-ui.mjs').then(module => {
    administration = module;
    ready?.();
  }).catch(() => {
    pending = null;
    const status = element.querySelector('.rn-admin-loader');
    if (status) {
      status.removeAttribute('aria-busy');
      status.textContent = 'Не удалось загрузить админку. Обновите страницу.';
    }
  });
}

export function unmountRemnaAdmin() {
  administration?.unmountRemnaAdmin();
  ready = undefined;
}

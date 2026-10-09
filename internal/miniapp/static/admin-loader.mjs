// Load the framework once and use the current authenticated page when it arrives.
export function createAdminLoader(download = () => import("./admin-ui.mjs")) {
  let administration;
  let pending;
  let latest;

  function mountRemnaAdmin(element, model, shell, onReady) {
    latest = { element, model, shell, onReady };
    if (administration) {
      administration.mountRemnaAdmin(element, model, shell);
      return;
    }
    if (!element.querySelector(".rn-admin-loader")) {
      element.innerHTML =
        '<div class="rn-admin-loader" role="status" aria-busy="true">Загрузка админки…</div>';
    }
    if (pending) return pending;
    pending = download()
      .then((module) => {
        administration = module;
        const target = latest;
        if (!target || target.element.isConnected === false) return;
        // Rebuild from the current application state, including changes received
        // while downloading. Never mount the section that was left behind.
        if (target.onReady) target.onReady();
        else module.mountRemnaAdmin(target.element, target.model, target.shell);
      })
      .catch(() => {
        const status = latest?.element.querySelector(".rn-admin-loader");
        if (status) {
          status.removeAttribute("aria-busy");
          status.textContent =
            "Не удалось загрузить админку. Обновите страницу.";
        }
      })
      .finally(() => {
        pending = null;
      });
    return pending;
  }

  function unmountRemnaAdmin() {
    latest = undefined;
    administration?.unmountRemnaAdmin();
  }
  return { mountRemnaAdmin, unmountRemnaAdmin };
}

const loader = createAdminLoader();
export const mountRemnaAdmin = loader.mountRemnaAdmin;
export const unmountRemnaAdmin = loader.unmountRemnaAdmin;

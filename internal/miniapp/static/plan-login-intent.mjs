const KEY = 'link-bot-plan-login-intent';
const LIFETIME = 2 * 60 * 60 * 1000;

// Keep this tab's choice across an authentication redirect, without persisting it
// across browsers or sharing it with other tabs/accounts through localStorage.
export function createPlanLoginIntent({getStorage = () => window.sessionStorage, now = Date.now} = {}) {
  let memory = null;
  const valid = value => value && typeof value.id === 'string' && value.id.length > 0 && value.id.length <= 256
    && Number.isFinite(value.createdAt) && value.createdAt <= now() && now() - value.createdAt < LIFETIME;
  function clear() {
    memory = null;
    try { getStorage().removeItem(KEY); } catch {}
  }
  function peek() {
    let value = memory;
    try { value = JSON.parse(getStorage().getItem(KEY) || 'null') || memory; } catch {}
    if (!valid(value)) { clear(); return ''; }
    memory = value;
    return value.id;
  }
  return {
    capture(id) {
      if (typeof id !== 'string' || !id || id.length > 256) return;
      memory = {id, createdAt: now()};
      try { getStorage().setItem(KEY, JSON.stringify(memory)); } catch {}
    },
    peek,
    consume(plans) {
      const id = peek();
      if (!id) return '';
      clear();
      return plans.some(plan => String(plan.id || plan.months || '') === id) ? id : '';
    },
    clear,
  };
}

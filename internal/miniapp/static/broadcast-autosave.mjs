const clone = value => JSON.parse(JSON.stringify(value));
const equal = (a, b) => JSON.stringify(a) === JSON.stringify(b);
const filled = value => Boolean(String(value || '').trim());

function completeButtons(buttons = []) {
  return buttons.every(button => {
    if (!filled(button.text)) return false;
    if (button.type === 'promo') return filled(button.promoCode);
    if (button.type !== 'url') return true;
    try { return Boolean(new URL(button.url).host); } catch { return false; }
  });
}

// Serialize writes to the shared campaign nonce. Acknowledgements may update
// server versions, but must never replace edits made while a request is running.
export function createBroadcastAutosave({ state, post, applyTelegramDraft, onChange = () => {}, onError = () => {}, delay = 350, setTimer = setTimeout, clearTimer = clearTimeout }) {
  const pending = new Set();
  let timer, active;

  function read(key) {
    if (key.startsWith('audience:')) {
      const channel = key.split(':')[1];
      return state.adminAudienceDirty?.[channel] ? clone(state.adminAudienceDraft[channel]) : null;
    }
    if (key === 'survey') return state.adminSurveyDirty ? { ...clone(state.adminSurveyDraft), title: state.adminSurveyDraft.title || 'Опрос' } : null;
    if (key === 'buttons') return state.adminBroadcastButtonsDirty ? clone(state.adminBroadcastButtonsDraft) : null;
    if (key === 'expiration') return state.adminBroadcastExpirationDirty ? state.adminBroadcastExpirationDraft : null;
    if (key === 'auto-reply') return state.adminBroadcastAutoReplyDirty ? clone(state.adminBroadcastAutoReplyDraft) : null;
    if (key === 'email') return state.adminEmailDraftDirty ? { subject: state.adminEmailDraftSubject, body: state.adminEmailDraftBody } : null;
    return null;
  }
  function complete(key, value) {
    if (key === 'survey' && value.enabled) return filled(value.introText) && filled(value.startText) && filled(value.finishText) && value.questions?.length && value.questions.every(q => filled(q.text) && q.options?.length && q.options.every(filled)) && completeButtons(value.finalButtons);
    if (key === 'buttons') return completeButtons(value);
    if (key === 'auto-reply') return filled(value.text) && value.windowHours >= 1 && value.windowHours <= 168;
    if (key === 'email') return String(value.subject || '').trim().length >= 2 && String(value.body || '').trim().length >= 2;
    return true;
  }
  async function save(key, value) {
    const channel = key.startsWith('audience:') ? key.split(':')[1] : key === 'email' ? 'email' : 'telegram';
    if ((channel === 'email' ? state.adminEmailBroadcast?.draft : state.adminBroadcast)?.status === 'running') return;
    let path, body;
    if (key.startsWith('audience:')) { path = 'audience'; body = { channel, audience: value }; }
    else if (key === 'survey') { path = 'survey'; body = { survey: value, updatedAt: state.adminBroadcast?.updatedAt }; }
    else if (key === 'buttons') { path = 'buttons'; body = { buttons: value }; }
    else if (key === 'expiration') { path = 'expiration'; body = { deleteAfterMinutes: value, updatedAt: state.adminBroadcast?.updatedAt }; }
    else if (key === 'email') { path = 'email/save'; body = value; }
    else { path = 'auto-reply'; body = value; }
    const { data } = await post(`/api/mini-app/admin/broadcast/${path}`, body);
    const unchanged = equal(read(key), value);
    if (key.startsWith('audience:')) {
      if (unchanged) { state.adminAudienceDirty[channel] = false; state.adminAudienceCount[channel] = data.count; }
      if (channel === 'email') state.adminEmailBroadcast = { ...state.adminEmailBroadcast, draft: data.draft };
      else applyTelegramDraft(data.draft);
    } else if (key === 'auto-reply') {
      if (unchanged) { state.adminBroadcastAutoReplyDraft = clone(data); state.adminBroadcastAutoReplyDirty = false; }
      else state.adminBroadcastAutoReplyDraft.updatedAt = data.updatedAt;
      if (state.adminBroadcast) state.adminBroadcast.autoReply = data;
    } else if (key === 'email') {
      state.adminEmailBroadcast = { ...state.adminEmailBroadcast, draft: data };
      if (unchanged) state.adminEmailDraftDirty = false;
    } else {
      if (unchanged) state[{ survey: 'adminSurveyDirty', buttons: 'adminBroadcastButtonsDirty', expiration: 'adminBroadcastExpirationDirty' }[key]] = false;
      applyTelegramDraft(data);
    }
    onChange();
  }
  function schedule(key) {
    pending.add(key);
    clearTimer(timer);
    timer = setTimer(() => { timer = undefined; void flush().catch(() => {}); }, delay);
  }
  async function flush() {
    clearTimer(timer); timer = undefined;
    if (active) { await active; if (pending.size) return flush(); return; }
    if (!pending.size) return;
    state.adminBroadcastAutoSaving = true;
    state.adminBroadcastAutoSaveError = '';
    active = (async () => {
      const failed = new Set();
      let failure;
      while ([...pending].some(key => !failed.has(key))) {
        const key = [...pending].find(key => !failed.has(key));
        pending.delete(key);
        const value = read(key);
        if (value === null || !complete(key, value)) continue;
        try { await save(key, value); }
        catch (error) { pending.add(key); failed.add(key); failure ||= error; }
      }
      if (failure) { state.adminBroadcastAutoSaveError = failure?.message || 'Не удалось сохранить изменения'; onError(failure); throw failure; }
    })();
    try { await active; }
    finally { active = undefined; state.adminBroadcastAutoSaving = false; onChange(); }
  }
  async function discard() {
    clearTimer(timer); timer = undefined; pending.clear();
    try { await active; } catch { /* Reset can discard a failed draft. */ }
    pending.clear(); clearTimer(timer); timer = undefined;
  }
  return { schedule, flush, discard };
}

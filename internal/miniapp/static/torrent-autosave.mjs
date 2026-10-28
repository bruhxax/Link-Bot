export function createTorrentAutosave({ save, onState = () => {}, delay = 900, setTimer = setTimeout, clearTimer = clearTimeout }) {
  const state = { phase: 'idle', error: '' };
  let timer = null;
  let revision = 0;
  let savedRevision = 0;
  let latest = null;
  let inFlight = false;

  const emit = (phase, error = '') => {
    state.phase = phase;
    state.error = error;
    onState({ ...state });
  };
  const arm = (wait = delay) => {
    if (timer !== null) clearTimer(timer);
    timer = setTimer(() => { timer = null; void flush(); }, wait);
  };
  async function flush() {
    if (timer !== null) clearTimer(timer);
    timer = null;
    if (inFlight || revision === savedRevision || latest === null) return;
    const currentRevision = revision;
    const payload = latest;
    inFlight = true;
    emit('saving');
    try {
      await save(payload);
      savedRevision = currentRevision;
      if (revision === currentRevision) emit('saved');
    } catch (error) {
      if (revision === currentRevision) emit('error', error?.message || 'Не удалось сохранить изменения');
    } finally {
      inFlight = false;
      if (revision > currentRevision) arm();
    }
  }
  return {
    state,
    schedule(payload) {
      latest = payload;
      revision++;
      emit('pending');
      arm();
    },
    flush,
  };
}

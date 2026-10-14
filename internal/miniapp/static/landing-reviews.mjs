const escapeHTML = (value) => String(value ?? "").replace(/[&<>"']/g, (char) => ({"&":"&amp;","<":"&lt;",">":"&gt;",'"':"&quot;","'":"&#39;"})[char]);
const star = '<svg viewBox="0 0 24 24" aria-hidden="true"><path d="m12 2.8 2.8 5.67 6.26.91-4.53 4.42 1.07 6.24L12 17.1l-5.6 2.94 1.07-6.24L2.94 9.38l6.26-.91Z" fill="currentColor"/></svg>';

export function reviewTickerItem(review) {
  const rating = Number(review?.rating);
  if (!Number.isInteger(rating) || rating < 1 || rating > 5) return "";
  const username = String(review.username || "").trim().replace(/^@+/, "");
  const author = /^[a-z0-9_]{5,32}$/i.test(username) ? `@${username}` : "Пользователь";
  const comment = String(review.comment || "").replace(/\s+/g, " ").trim();
  return `<li class="review-marquee__item"><span class="review-marquee__stars" role="img" aria-label="Оценка ${rating} из 5">${star.repeat(rating)}</span><span class="review-marquee__author">${escapeHTML(author)}</span>${comment ? `<span class="review-marquee__comment">${escapeHTML(comment)}</span>` : ""}</li>`;
}

export function createReviewTicker(section) {
  const viewport = section.querySelector('#reviews-viewport');
  const track = section.querySelector('#reviews-track');
  const reducedMotion = window.matchMedia('(prefers-reduced-motion: reduce)');
  let fingerprint = "";
  let markup = "";
  let width = -1;

  function rebuild() {
    if (!markup || section.hidden) return;
    const group = document.createElement('ul');
    group.className = 'review-marquee__group';
    group.innerHTML = markup;
    track.replaceChildren(group);
    // Fill a whole viewport even with a single short review, then duplicate a
    // complete group. Both halves have exactly the same width for a clean loop.
    const baseWidth = group.getBoundingClientRect().width;
    if (!baseWidth) return;
    const repeats = Math.max(1, Math.ceil(viewport.clientWidth / baseWidth));
    const originals = [...group.children];
    for (let i = 1; i < repeats; i += 1) {
      for (const item of originals) {
        const repeated = item.cloneNode(true);
        repeated.setAttribute('aria-hidden', 'true');
        group.append(repeated);
      }
    }
    const distance = group.getBoundingClientRect().width;
    const duplicate = group.cloneNode(true);
    duplicate.setAttribute('aria-hidden', 'true');
    duplicate.setAttribute('inert', '');
    duplicate.classList.add('review-marquee__duplicate');
    track.append(duplicate);
    track.style.setProperty('--reviews-distance', `${distance}px`);
    track.style.setProperty('--reviews-duration', `${Math.max(12, distance / (viewport.clientWidth < 620 ? 30 : 38))}s`);
    section.classList.toggle('is-reduced-motion', reducedMotion.matches);
  }

  const visibility = () => section.classList.toggle('is-page-hidden', document.hidden);
  document.addEventListener('visibilitychange', visibility);
  visibility();
  if ('IntersectionObserver' in window) {
    const observer = new IntersectionObserver(([entry]) => section.classList.toggle('is-offscreen', !entry.isIntersecting));
    observer.observe(section);
  }
  if ('ResizeObserver' in window) {
    new ResizeObserver(() => {
      if (width === viewport.clientWidth) return;
      width = viewport.clientWidth;
      rebuild();
    }).observe(viewport);
  } else window.addEventListener('resize', rebuild);
  reducedMotion.addEventListener?.('change', rebuild);
  document.fonts?.ready.then(rebuild);

  return {
    setReviews(reviews) {
      const next = (Array.isArray(reviews) ? reviews : []).map(reviewTickerItem).filter(Boolean).join('');
      if (next === fingerprint) return;
      fingerprint = next;
      markup = next;
      section.hidden = !markup;
      if (!markup) { track.replaceChildren(); return; }
      rebuild();
    },
  };
}

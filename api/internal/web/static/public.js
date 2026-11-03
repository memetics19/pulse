(function () {
  const root = document.documentElement;
  const DARK = ['dark', 'default-dark', 'terminal-dark'];
  const isDark = () => DARK.includes(root.getAttribute('data-theme'));

  // A visitor's own choice wins. Without one, a light preset follows the
  // system dark mode, and a dark preset stays as the admin set it.
  let saved = null;
  try { saved = localStorage.getItem('pulse-theme'); } catch (e) { /* storage blocked */ }
  if (saved) {
    root.setAttribute('data-theme', saved);
  } else if (!isDark() && window.matchMedia && matchMedia('(prefers-color-scheme: dark)').matches) {
    root.setAttribute('data-theme', 'dark');
  }
  const btn = document.getElementById('dark-toggle');
  if (btn) btn.addEventListener('click', () => {
    const next = isDark() ? 'light' : 'dark';
    root.setAttribute('data-theme', next);
    try { localStorage.setItem('pulse-theme', next); } catch (e) { /* storage blocked */ }
  });

  // Render server timestamps in the visitor's local time zone.
  function localTimes(scope) {
    scope.querySelectorAll('.localtime').forEach((el) => {
      const d = new Date(el.getAttribute('data-ts') || '');
      if (isNaN(d.getTime())) return;
      el.textContent = el.getAttribute('data-format') === 'time'
        ? d.toLocaleTimeString(undefined, { hour: '2-digit', minute: '2-digit' })
        : d.toLocaleString(undefined, { month: 'short', day: 'numeric', hour: '2-digit', minute: '2-digit' });
      el.setAttribute('title', d.toLocaleString());
    });
  }
  localTimes(document);

  // One tooltip for every uptime bar. It follows hover on desktop and a tap on
  // touch screens, where a title attribute never shows.
  const tip = document.getElementById('bar-tip');
  function showTip(bar) {
    if (!tip) return;
    const r = bar.getBoundingClientRect();
    tip.textContent = bar.getAttribute('data-tip');
    tip.hidden = false;
    const half = tip.offsetWidth / 2 + 8;
    tip.style.left = Math.min(Math.max(r.left + r.width / 2, half), window.innerWidth - half) + 'px';
    tip.style.top = r.top + 'px';
  }
  const hideTip = () => { if (tip) tip.hidden = true; };
  document.addEventListener('pointerover', (e) => {
    const bar = e.target.closest && e.target.closest('.bar[data-tip]');
    if (bar) showTip(bar); else if (e.pointerType === 'mouse') hideTip();
  });
  document.addEventListener('pointerdown', (e) => {
    const bar = e.target.closest && e.target.closest('.bar[data-tip]');
    if (bar) showTip(bar); else hideTip();
  });
  window.addEventListener('scroll', hideTip, { passive: true });

  // Subscribe popover
  const subBtn = document.getElementById('subscribe-btn');
  const subPop = document.getElementById('subscribe-pop');
  const closeSub = () => { if (subPop) { subPop.hidden = true; subBtn.setAttribute('aria-expanded', 'false'); } };
  if (subBtn && subPop) {
    subBtn.addEventListener('click', (e) => {
      e.stopPropagation();
      const open = subPop.hidden;
      subPop.hidden = !open;
      subBtn.setAttribute('aria-expanded', String(open));
    });
    document.addEventListener('click', (e) => {
      if (!subPop.hidden && !subPop.contains(e.target) && e.target !== subBtn) closeSub();
    });
    document.addEventListener('keydown', (e) => { if (e.key === 'Escape') { closeSub(); hideTip(); } });
  }

  // Refresh the live part of the page every 30 seconds without a full reload,
  // so the scroll position, open details and the theme stay as they are. It
  // waits while the tab is hidden and keeps the current page on any error.
  const live = document.getElementById('live');
  async function refresh() {
    if (document.visibilityState !== 'visible' || !live) return;
    try {
      const res = await fetch(location.href, { headers: { Accept: 'text/html' }, cache: 'no-store' });
      if (!res.ok) return;
      const doc = new DOMParser().parseFromString(await res.text(), 'text/html');
      const next = doc.getElementById('live');
      if (!next) return;
      const open = new Set([...live.querySelectorAll('details[open]')].map((d) => d.closest('[data-key]')?.getAttribute('data-key')).filter(Boolean));
      live.innerHTML = next.innerHTML;
      open.forEach((k) => live.querySelector(`[data-key="${CSS.escape(k)}"] details`)?.setAttribute('open', ''));
      document.title = doc.title;
      localTimes(live);
      hideTip();
    } catch (e) {
      // Offline or the server is restarting. Keep the page and try again later.
    }
  }
  setInterval(refresh, 30000);
  document.addEventListener('visibilitychange', () => { if (document.visibilityState === 'visible') refresh(); });
})();

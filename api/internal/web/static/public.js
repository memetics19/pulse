(function () {
  const root = document.documentElement;
  const saved = localStorage.getItem('pulse-theme');
  if (saved) root.setAttribute('data-theme', saved);
  const btn = document.getElementById('dark-toggle');
  if (btn) btn.addEventListener('click', () => {
    const next = root.getAttribute('data-theme') === 'dark' ? 'light' : 'dark';
    root.setAttribute('data-theme', next);
    localStorage.setItem('pulse-theme', next);
  });

  // Render server timestamps in the visitor's local timezone.
  document.querySelectorAll('.localtime').forEach((el) => {
    const ts = el.getAttribute('data-ts');
    if (!ts) return;
    const d = new Date(ts);
    if (!isNaN(d.getTime())) {
      el.textContent = d.toLocaleString(undefined, {
        month: 'short', day: 'numeric', hour: '2-digit', minute: '2-digit',
      });
    }
  });

  // Subscribe popover
  const subBtn = document.getElementById('subscribe-btn');
  const subPop = document.getElementById('subscribe-pop');
  if (subBtn && subPop) {
    subBtn.addEventListener('click', (e) => {
      e.stopPropagation();
      const open = subPop.hidden;
      subPop.hidden = !open;
      subBtn.setAttribute('aria-expanded', String(open));
    });
    document.addEventListener('click', (e) => {
      if (!subPop.hidden && !subPop.contains(e.target) && e.target !== subBtn) {
        subPop.hidden = true;
        subBtn.setAttribute('aria-expanded', 'false');
      }
    });
    document.addEventListener('keydown', (e) => {
      if (e.key === 'Escape') { subPop.hidden = true; subBtn.setAttribute('aria-expanded', 'false'); }
    });
  }

  setInterval(() => location.reload(), 30000);
})();

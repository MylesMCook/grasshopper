// Apply a saved theme before styles or body content can paint.
try {
  const theme = localStorage.getItem('grasshopper-theme');
  if (theme === 'light' || theme === 'dark') document.documentElement.dataset.theme = theme;
} catch {}

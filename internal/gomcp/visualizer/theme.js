const themeButton = document.getElementById('theme-toggle');
const themeKey = 'grasshopper-theme';
const systemTheme = window.matchMedia('(prefers-color-scheme: dark)');

try {
  const savedTheme = localStorage.getItem(themeKey);
  if (savedTheme === 'light' || savedTheme === 'dark') document.documentElement.dataset.theme = savedTheme;
} catch {}

function currentTheme() {
  return document.documentElement.dataset.theme || (systemTheme.matches ? 'dark' : 'light');
}
function updateThemeButton() {
  const next = currentTheme() === 'dark' ? 'Light mode' : 'Dark mode';
  const label = 'Switch to ' + next.toLowerCase();
  themeButton.setAttribute('aria-label', label);
  themeButton.title = label;
}
themeButton.addEventListener('click', () => {
  const next = currentTheme() === 'dark' ? 'light' : 'dark';
  document.documentElement.dataset.theme = next;
  try { localStorage.setItem(themeKey, next); } catch {}
  updateThemeButton();
});
systemTheme.addEventListener('change', updateThemeButton);
updateThemeButton();

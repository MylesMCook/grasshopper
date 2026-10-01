// Theme choice is shared by the public site (a toggle button) and the memory
// view (System, Light and Dark in Settings). Only the choice is stored.
const themeButton = document.getElementById('theme-toggle');
const themeKey = 'grasshopper-theme';
const systemTheme = window.matchMedia('(prefers-color-scheme: dark)');

function currentTheme() {
  return document.documentElement.dataset.theme || (systemTheme.matches ? 'dark' : 'light');
}
function savedTheme() {
  try { const value = localStorage.getItem(themeKey); return value === 'light' || value === 'dark' ? value : 'system'; } catch { return 'system'; }
}
function setTheme(choice) {
  if (choice === 'light' || choice === 'dark') document.documentElement.dataset.theme = choice;
  else delete document.documentElement.dataset.theme;
  try { if (choice === 'light' || choice === 'dark') localStorage.setItem(themeKey, choice); else localStorage.removeItem(themeKey); } catch {}
  updateThemeButton();
}
function updateThemeButton() {
  if (!themeButton) return;
  const label = 'Switch to ' + (currentTheme() === 'dark' ? 'light' : 'dark') + ' mode';
  themeButton.setAttribute('aria-label', label);
  themeButton.title = label;
}
themeButton?.addEventListener('click', () => setTheme(currentTheme() === 'dark' ? 'light' : 'dark'));
systemTheme.addEventListener('change', updateThemeButton);
updateThemeButton();
window.grasshopperTheme = { saved: savedTheme, set: setTheme };

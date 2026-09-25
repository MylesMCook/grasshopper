const form = document.getElementById('server-form');
const address = document.getElementById('server-address');
const error = document.getElementById('server-error');
const storageKey = 'grasshopper-server-url';

try {
  const saved = localStorage.getItem(storageKey);
  if (saved) address.value = saved;
} catch {}

form.addEventListener('submit', event => {
  event.preventDefault();
  error.hidden = true;
  const entered = address.value.trim();
  let url;
  try {
    url = new URL(entered.includes('://') ? entered : `https://${entered}`);
  } catch {
    error.textContent = 'Enter a valid server address.';
    error.hidden = false;
    return;
  }
  const localHost = url.hostname === 'localhost' || url.hostname === '127.0.0.1' || url.hostname === '[::1]';
  if ((url.protocol !== 'https:' && !(url.protocol === 'http:' && localHost)) || url.username || url.password) {
    error.textContent = 'Use an HTTPS address, or HTTP on this device only.';
    error.hidden = false;
    return;
  }
  try { localStorage.setItem(storageKey, url.origin); } catch {}
  window.location.assign(`${url.origin}/visualizer/`);
});

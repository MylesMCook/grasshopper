const form = document.getElementById('server-form');
const address = document.getElementById('server-address');
const error = document.getElementById('server-error');
const storageKey = 'grasshopper-server-url';

try {
  const saved = localStorage.getItem(storageKey);
  if (saved) address.value = saved;
} catch {}

// The message is tied to the field so it is read with it, and focus returns
// to the field to fix it.
function showError(message) {
  error.textContent = message;
  error.hidden = false;
  address.setAttribute('aria-invalid', 'true');
  address.setAttribute('aria-describedby', 'server-error');
  address.focus();
}
function clearError() {
  error.textContent = '';
  error.hidden = true;
  address.removeAttribute('aria-invalid');
  address.removeAttribute('aria-describedby');
}

form.addEventListener('submit', event => {
  event.preventDefault();
  clearError();
  const entered = address.value.trim();
  let url;
  try {
    url = new URL(entered.includes('://') ? entered : `https://${entered}`);
  } catch {
    showError("Enter your server's address, like your-server.tailnet.ts.net.");
    return;
  }
  const localHost = url.hostname === 'localhost' || url.hostname === '127.0.0.1' || url.hostname === '[::1]';
  // Reject ambiguous bare names and malformed DNS labels. An explicit HTTPS
  // link can still use a single-label private host, as the connector can.
  const host = url.hostname.replace(/\.$/, '');
  const ipv6 = /^\[[0-9a-f:]+\]$/i.test(host);
  const dns = host.length <= 253 && (host.includes('.') || /^https:\/\//i.test(entered)) && host.split('.').every(label => label.length <= 63 && /^[a-z0-9](?:[a-z0-9-]*[a-z0-9])?$/i.test(label));
  if (!localHost && !ipv6 && !dns) {
    showError("Enter your server's address, like your-server.tailnet.ts.net.");
    return;
  }
  if (url.username || url.password || entered.includes('?') || entered.includes('#')) {
    showError('Use a server link without credentials, query text, or a fragment.');
    return;
  }
  if (url.protocol !== 'https:' && !(url.protocol === 'http:' && localHost)) {
    showError('Use HTTPS, or local HTTP on this device.');
    return;
  }
  if (!['/', '/mcp', '/visualizer', '/visualizer/'].includes(url.pathname)) {
    showError('Use the server, memory view, or MCP link.');
    return;
  }
  try { localStorage.setItem(storageKey, url.origin); } catch {}
  window.location.assign(`${url.origin}/visualizer/`);
});


document.getElementById('forget-address').addEventListener('click', () => {
  try { localStorage.removeItem(storageKey); } catch {}
  address.value = '';
  clearError();
  address.focus();
});

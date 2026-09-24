const form = document.getElementById('connection-form');
const tokenInput = document.getElementById('token');
const connectButton = document.getElementById('connect');
const disconnectButton = document.getElementById('disconnect');
const status = document.getElementById('status');
const summary = document.getElementById('summary');
const omissions = document.getElementById('omissions');
const records = document.getElementById('records');
const scopeSummary = document.querySelector('.scope-details summary');

let token = null;
let active = false;
let timer = null;
let inFlight = null;
let revisions = new Map();
let hasLoaded = false;

function setStatus(message, kind = '') {
  status.textContent = message;
  status.className = `status ${kind}`;
}

function setConnected(value) {
  active = value;
  tokenInput.required = !value;
  connectButton.textContent = value ? 'Refresh scope' : 'Connect';
  disconnectButton.disabled = !value;
}

function stop(clearRecords = false) {
  setConnected(false);
  token = null;
  clearTimeout(timer);
  timer = null;
  if (inFlight) inFlight.abort();
  inFlight = null;
  if (clearRecords) {
    records.replaceChildren();
    omissions.hidden = true;
    revisions = new Map();
    hasLoaded = false;
    summary.textContent = 'Enter a token to load context.';
  }
}

function scopeInput() {
  const scope = {};
  for (const key of ['project', 'device', 'platform']) {
    const value = document.getElementById(key).value.trim();
    if (value) scope[key] = value;
  }
  scopeSummary.textContent = Object.keys(scope).length ? `Scope · ${Object.keys(scope).join(' + ')}` : 'Scope · global';
  return scope;
}

function label(text) {
  const span = document.createElement('span');
  span.textContent = text;
  return span;
}

function draw(page) {
  const next = new Map();
  const fragment = document.createDocumentFragment();
  const items = page.records || [];
  for (const record of items) {
    const key = `${record.id}`;
    next.set(key, record.revision);
    const article = document.createElement('article');
    article.className = 'record';
    if (hasLoaded && revisions.get(key) !== record.revision) article.classList.add('changed');
    const side = document.createElement('div');
    const kind = document.createElement('p');
    kind.className = 'record-kind';
    kind.textContent = record.purpose || 'Memory';
    side.append(kind);
    const body = document.createElement('div');
    const heading = document.createElement('h3');
    heading.textContent = record.title || `Memory #${record.id}`;
    const content = document.createElement('p');
    content.className = 'record-content';
    content.textContent = record.content;
    const meta = document.createElement('div');
    meta.className = 'record-meta';
    const scope = record.scope || {};
    const dimensions = [scope.project && `Project ${scope.project}`, scope.device && `Device ${scope.device}`, scope.platform && `Platform ${scope.platform}`].filter(Boolean);
    meta.append(
      label(`ID ${record.id} · revision ${record.revision}`),
      label(dimensions.length ? dimensions.join(' · ') : 'Global'),
      label(record.confirmed ? 'Confirmed' : 'Handoff'),
      label(`Source: ${record.provenance?.source || 'Unknown'}`),
      label(`Agent: ${record.provenance?.harness || 'Unknown'}`),
      label(`Source device: ${record.provenance?.device || 'Unknown'}`)
    );
    body.append(heading, content, meta);
    article.append(side, body);
    fragment.append(article);
  }
  if (!items.length) {
    const empty = document.createElement('p');
    empty.className = 'empty';
    empty.textContent = 'No confirmed memories or handoff match this scope.';
    fragment.append(empty);
  }
  records.replaceChildren(fragment);
  revisions = next;
  hasLoaded = true;
  summary.textContent = `${items.length} ${items.length === 1 ? 'record' : 'records'} · updated ${new Date().toLocaleTimeString()}`;
  const omitted = page.omitted || 0;
  omissions.hidden = !omitted;
  if (omitted) {
    const ids = (page.omitted_ids || []).join(', ');
    omissions.textContent = `${omitted} ${omitted === 1 ? 'record was' : 'records were'} omitted by the response limit.${ids ? ` IDs: ${ids}.` : ''} Use Grasshopper get for full content.`;
  }
}

async function refresh() {
  if (!active || inFlight) return;
  const controller = new AbortController();
  inFlight = controller;
  const timeout = setTimeout(() => controller.abort(), 5000);
  try {
    const response = await fetch('/visualizer/api/context', {
      method: 'POST',
      headers: { Authorization: `Bearer ${token}`, 'Content-Type': 'application/json' },
      body: JSON.stringify(scopeInput()),
      cache: 'no-store',
      signal: controller.signal
    });
    if (!response.ok) throw new Error(response.status === 401 ? 'Token rejected' : response.status === 400 ? 'Invalid scope' : 'Service unavailable');
    const page = await response.json();
    if (!active || inFlight !== controller) return;
    draw(page);
    setStatus('Live · refreshes every 3 seconds', 'live');
    timer = setTimeout(refresh, 3000);
  } catch (error) {
    if (inFlight !== controller) return;
    const message = error.name === 'AbortError' ? 'Timed out' : error instanceof TypeError ? 'Service unavailable' : error.message;
    stop(message === 'Token rejected');
    setStatus(message, 'error');
    if (message !== 'Token rejected') summary.textContent = hasLoaded ? `${message}. Showing the last received context; reconnect to resume.` : `${message}. No context was loaded; reconnect to retry.`;
  } finally {
    clearTimeout(timeout);
    if (inFlight === controller) inFlight = null;
  }
}

form.addEventListener('submit', event => {
  event.preventDefault();
  if (tokenInput.value) token = tokenInput.value;
  tokenInput.value = '';
  if (!token) return;
  clearTimeout(timer);
  if (inFlight) inFlight.abort();
  inFlight = null;
  records.replaceChildren();
  revisions = new Map();
  hasLoaded = false;
  omissions.hidden = true;
  setConnected(true);
  setStatus('Connecting');
  summary.textContent = 'Loading context…';
  refresh();
});

disconnectButton.addEventListener('click', () => {
  stop(true);
  setStatus('Not connected');
});

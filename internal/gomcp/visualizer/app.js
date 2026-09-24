const form = document.getElementById('connection-form');
const tokenInput = document.getElementById('token');
const connectButton = document.getElementById('connect');
const disconnectButton = document.getElementById('disconnect');
const status = document.getElementById('status');
const summary = document.getElementById('summary');
const omissions = document.getElementById('omissions');
const records = document.getElementById('records');
const scopeSummary = document.querySelector('.scope-details summary');
const projectSelect = document.getElementById('project');
const manualProjectLabel = document.getElementById('manual-project-label');
const manualProjectInput = document.getElementById('manual-project');
const deviceSelect = document.getElementById('device');
const manualDeviceLabel = document.getElementById('manual-device-label');
const manualDeviceInput = document.getElementById('manual-device');
const manualChoice = '\u0000manual';

let token = null;
let active = false;
let timer = null;
let inFlight = null;
let revisions = new Map();
let hasLoaded = false;
let knownProjects = [];
let knownDevices = [];

function option(value, text) {
  const item = document.createElement('option');
  item.value = value;
  item.textContent = text;
  return item;
}

function updateOptions(select, values, known, emptyLabel, manualLabel, preserveSelection = true) {
  const names = Array.isArray(values) ? values.filter(value => typeof value === 'string') : [];
  if (preserveSelection && JSON.stringify(names) === JSON.stringify(known)) return known;
  const selected = preserveSelection ? select.value : '';
  const choices = [option('', emptyLabel)];
  for (const name of names) choices.push(option(name, name));
  if (selected && selected !== manualChoice && !names.includes(selected)) choices.push(option(selected, `Selected: ${selected}`));
  choices.push(option(manualChoice, manualLabel));
  select.replaceChildren(...choices);
  select.value = selected;
  return names;
}

function updateProjectOptions(projects, preserveSelection = true) {
  knownProjects = updateOptions(projectSelect, projects, knownProjects, 'Global memories', 'Enter another project ID…', preserveSelection);
}

function updateDeviceOptions(devices, preserveSelection = true) {
  knownDevices = updateOptions(deviceSelect, devices, knownDevices, 'Any device', 'Enter another device ID…', preserveSelection);
}

function showManualProject() {
  const manual = projectSelect.value === manualChoice;
  manualProjectLabel.hidden = !manual;
  manualProjectInput.required = manual;
  if (manual) manualProjectInput.focus();
  else manualProjectInput.value = '';
}

function showManualDevice() {
  const manual = deviceSelect.value === manualChoice;
  manualDeviceLabel.hidden = !manual;
  manualDeviceInput.required = manual;
  if (manual) manualDeviceInput.focus();
  else manualDeviceInput.value = '';
}

updateProjectOptions([], false);
updateDeviceOptions([], false);
projectSelect.addEventListener('change', showManualProject);
deviceSelect.addEventListener('change', showManualDevice);

function setStatus(message, kind = '') {
  status.textContent = message;
  status.className = `status ${kind}`;
}

function setConnected(value) {
  active = value;
  tokenInput.required = !value;
  connectButton.textContent = value ? 'Refresh view' : 'Connect';
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
    updateProjectOptions([], false);
    updateDeviceOptions([], false);
    showManualProject();
    showManualDevice();
    scopeInput();
  }
}

function scopeInput() {
  const scope = {};
  const project = projectSelect.value === manualChoice ? manualProjectInput.value.trim() : projectSelect.value;
  const device = deviceSelect.value === manualChoice ? manualDeviceInput.value.trim() : deviceSelect.value;
  const platform = document.getElementById('platform').value;
  if (project) scope.project = project;
  if (device) scope.device = device;
  if (platform) scope.platform = platform;
  scopeSummary.textContent = `View · ${project ? 'one project' : 'global project'} · ${device || 'any device'} · ${platform || 'any platform'}`;
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
    empty.textContent = 'No confirmed memories or handoff match this view.';
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
    updateProjectOptions(page.projects);
    updateDeviceOptions(page.devices);
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

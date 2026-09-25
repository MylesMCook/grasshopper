const form = document.getElementById('connection-form');
const tokenInput = document.getElementById('token');
const tokenField = document.getElementById('token-field');
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
const devicePanel = document.getElementById('device-panel');
const deviceStatus = document.getElementById('device-status');
const pendingDevices = document.getElementById('pending-devices');
const connectedDevices = document.getElementById('connected-devices');
const serverAddress = document.getElementById('server-address');
const copyServerAddress = document.getElementById('copy-server-address');
serverAddress.textContent = `${location.origin}/mcp`;

let active = false;
let loggingIn = false;
let timer = null;
let inFlight = null;
let revisions = new Map();
let hasLoaded = false;
let knownProjects = [];
let knownDevices = [];
let lastSignature = '';
let deviceTimer = null;

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
  if (status.textContent !== message) status.textContent = message;
  const className = `status ${kind}`;
  if (status.className !== className) status.className = className;
}

function setConnected(value) {
  active = value;
  tokenInput.required = !value;
  tokenField.hidden = value;
  connectButton.textContent = value ? 'Refresh' : 'Connect';
  disconnectButton.disabled = !value;
  devicePanel.hidden = !value;
  if (value && devicePanel.open) refreshDevices();
}

function stop(clearRecords = false) {
  setConnected(false);
  clearTimeout(timer);
  timer = null;
  if (inFlight) inFlight.abort();
  inFlight = null;
  clearTimeout(deviceTimer);
  deviceTimer = null;
  devicePanel.hidden = true;
  if (clearRecords) {
    records.replaceChildren();
    omissions.hidden = true;
    revisions = new Map();
    lastSignature = '';
    hasLoaded = false;
    summary.textContent = 'Connect to see what is saved.';
    updateProjectOptions([], false);
    updateDeviceOptions([], false);
    showManualProject();
    showManualDevice();
    scopeInput();
  }
}

copyServerAddress.addEventListener('click', async () => {
  try {
    await navigator.clipboard.writeText(serverAddress.textContent);
    deviceStatus.textContent = 'Server address copied.';
  } catch {
    deviceStatus.textContent = 'Select and copy the address above.';
  }
});

devicePanel.addEventListener('toggle', () => {
  clearTimeout(deviceTimer);
  deviceTimer = null;
  if (devicePanel.open && active) refreshDevices();
});

function deviceRow(text, buttons = []) {
  const row = document.createElement('div');
  row.className = 'device-entry';
  const label = document.createElement('p');
  label.textContent = text;
  const actions = document.createElement('div');
  actions.className = 'device-actions';
  for (const [caption, action] of buttons) {
    const button = document.createElement('button');
    button.type = 'button';
    button.className = 'quiet';
    button.textContent = caption;
    button.addEventListener('click', action);
    actions.append(button);
  }
  row.append(label, actions);
  return row;
}

async function deviceRequest(path, method = 'GET', body) {
  const controller = new AbortController();
  const timeout = setTimeout(() => controller.abort(), 5000);
  try {
    const response = await fetch(path, {
      method, headers: body ? { 'Content-Type': 'application/json' } : {},
      body: body ? JSON.stringify(body) : undefined,
      credentials: 'same-origin', cache: 'no-store', signal: controller.signal
    });
    if (!response.ok) throw new Error('Device controls unavailable');
    return response.status === 204 ? null : response.json();
  } finally {
    clearTimeout(timeout);
  }
}

async function decidePairing(request, decision) {
  try {
    await deviceRequest('/visualizer/api/pairings', 'POST', { request_id: request.request_id, code: request.code, decision });
    deviceStatus.textContent = decision === 'approve' ? `${request.device} approved.` : `${request.device} denied.`;
    refreshDevices();
  } catch {
    deviceStatus.textContent = 'Could not change this request. Try again.';
  }
}

async function revokeDevice(device) {
  if (!window.confirm(`Disconnect ${device.device}? Its agent will lose access immediately.`)) return;
  try {
    await deviceRequest('/visualizer/api/devices', 'DELETE', { id: device.id });
    deviceStatus.textContent = `${device.device} disconnected.`;
    refreshDevices();
  } catch {
    deviceStatus.textContent = 'Could not disconnect this device. Try again.';
  }
}

async function refreshDevices() {
  if (!active || !devicePanel.open) return;
  clearTimeout(deviceTimer);
  try {
    const [pending, devices] = await Promise.all([
      deviceRequest('/visualizer/api/pairings'), deviceRequest('/visualizer/api/devices')
    ]);
    if (!active || !devicePanel.open) return;
    pendingDevices.replaceChildren(...pending.map(request => deviceRow(
      `${request.device} · code ${request.code}`,
      [['Approve', () => decidePairing(request, 'approve')], ['Deny', () => decidePairing(request, 'deny')]]
    )));
    const connected = devices.filter(device => !device.revoked_at);
    connectedDevices.replaceChildren(...connected.map(device => deviceRow(device.device, [['Disconnect', () => revokeDevice(device)]])));
    if (!pending.length) pendingDevices.replaceChildren(deviceRow('No connection requests waiting.'));
    if (!connected.length) connectedDevices.replaceChildren(deviceRow('No other devices connected.'));
  } catch {
    deviceStatus.textContent = 'Device controls unavailable. Memory view still works.';
  }
  if (active && devicePanel.open) deviceTimer = setTimeout(refreshDevices, 5000);
}

function scopeInput() {
  const scope = {};
  const project = projectSelect.value === manualChoice ? manualProjectInput.value.trim() : projectSelect.value;
  const device = deviceSelect.value === manualChoice ? manualDeviceInput.value.trim() : deviceSelect.value;
  const platform = document.getElementById('platform').value;
  if (project) scope.project = project;
  if (device) scope.device = device;
  if (platform) scope.platform = platform;
  const view = [project ? 'one project' : 'global', device ? 'one device' : 'all devices'];
  if (platform) view.push(platform);
  const description = `Scope · ${view.join(' · ')}`;
  if (scopeSummary.textContent !== description) scopeSummary.textContent = description;
  return scope;
}

function label(text) {
  const span = document.createElement('span');
  span.textContent = text;
  return span;
}

function draw(page) {
  const items = page.records || [];
  const omitted = page.omitted || 0;
  omissions.hidden = !omitted;
  if (omitted) {
    const ids = (page.omitted_ids || []).join(', ');
    const message = `${omitted} ${omitted === 1 ? 'memory was' : 'memories were'} left out because this view is full.${ids ? ` IDs: ${ids}.` : ''} Use Grasshopper get to read the full record.`;
    if (omissions.textContent !== message) omissions.textContent = message;
  }
  const signature = JSON.stringify(items.map(record => [record.id, record.revision, record.title, record.content, record.scope, record.provenance, record.confirmed, record.purpose]));
  const selection = window.getSelection();
  const selectingRecord = selection && !selection.isCollapsed && (records.contains(selection.anchorNode) || records.contains(selection.focusNode));
  if (hasLoaded && (signature === lastSignature || selectingRecord)) return;

  const next = new Map();
  const fragment = document.createDocumentFragment();
  for (const record of items) {
    const key = `${record.id}`;
    next.set(key, record.revision);
    const article = document.createElement('article');
    article.className = 'record';
    if (hasLoaded && revisions.get(key) !== record.revision) article.classList.add('changed');
    const top = document.createElement('div');
    top.className = 'record-top';
    const kind = document.createElement('p');
    kind.className = 'record-kind';
    kind.textContent = record.purpose || 'Memory';
    const scope = record.scope || {};
    const scopeLabel = document.createElement('p');
    scopeLabel.className = 'record-scope';
    scopeLabel.textContent = scope.project ? `Project · ${scope.project.split('/').pop()}` : scope.device ? 'Device' : scope.platform || 'Global';
    top.append(kind, scopeLabel);
    const heading = document.createElement('h3');
    heading.textContent = record.title || `Memory #${record.id}`;
    const content = document.createElement('p');
    content.className = 'record-content';
    content.textContent = record.content;
    const details = document.createElement('details');
    details.className = 'record-details';
    const detailsLabel = document.createElement('summary');
    detailsLabel.textContent = `Details · #${record.id} · revision ${record.revision}`;
    const meta = document.createElement('div');
    meta.className = 'record-meta';
    const dimensions = [scope.project && `Project ${scope.project}`, scope.device && `Device ${scope.device}`, scope.platform && `Platform ${scope.platform}`].filter(Boolean);
    meta.append(
      label(dimensions.length ? dimensions.join(' · ') : 'Global'),
      label(record.confirmed ? 'Confirmed' : 'Handoff'),
      label(`Source: ${record.provenance?.source || 'Unknown'}`),
      label(`Agent: ${record.provenance?.harness || 'Unknown'}`),
      label(`Source device: ${record.provenance?.device || 'Unknown'}`)
    );
    details.append(detailsLabel, meta);
    article.append(top, heading, content, details);
    fragment.append(article);
  }
  if (!items.length) {
    const empty = document.createElement('p');
    empty.className = 'empty';
    empty.textContent = 'No memories here yet. Ask an agent to save one, then check back.';
    fragment.append(empty);
  }
  records.replaceChildren(fragment);
  revisions = next;
  lastSignature = signature;
  hasLoaded = true;
  summary.textContent = `${items.length} ${items.length === 1 ? 'memory' : 'memories'}`;
}

async function refresh() {
  if (!active || inFlight) return;
  const controller = new AbortController();
  inFlight = controller;
  const timeout = setTimeout(() => controller.abort(), 5000);
  try {
    const response = await fetch('/visualizer/api/context', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(scopeInput()),
      cache: 'no-store',
      credentials: 'same-origin',
      signal: controller.signal
    });
    if (!response.ok) throw new Error(response.status === 401 ? 'Connection expired' : response.status === 400 ? 'Scope not recognized' : 'Service unavailable');
    const page = await response.json();
    if (!active || inFlight !== controller) return;
    updateProjectOptions(page.projects);
    updateDeviceOptions(page.devices);
    draw(page);
    setStatus('Live · remembered here', 'live');
    timer = setTimeout(refresh, 3000);
  } catch (error) {
    if (inFlight !== controller) return;
    const message = error.name === 'AbortError' ? 'Request timed out' : error instanceof TypeError ? 'Service unavailable' : error.message;
    stop(message === 'Connection expired');
    setStatus(message, 'error');
    summary.textContent = message === 'Connection expired' ? 'Connect again to see your memories.' : hasLoaded ? 'Showing the last results. Reconnect to refresh.' : 'No memories loaded. Reconnect to retry.';
  } finally {
    clearTimeout(timeout);
    if (inFlight === controller) inFlight = null;
  }
}

async function sessionRequest(method, bearer) {
  const controller = new AbortController();
  const timeout = setTimeout(() => controller.abort(), 5000);
  try {
    return await fetch('/visualizer/api/session', {
      method,
      headers: bearer ? { Authorization: `Bearer ${bearer}` } : {},
      credentials: 'same-origin',
      cache: 'no-store',
      signal: controller.signal
    });
  } finally {
    clearTimeout(timeout);
  }
}

form.addEventListener('submit', async event => {
  event.preventDefault();
  if (active) {
    clearTimeout(timer);
    refresh();
    return;
  }
  if (loggingIn || !tokenInput.value) return;
  const bearer = tokenInput.value;
  tokenInput.value = '';
  loggingIn = true;
  connectButton.disabled = true;
  setStatus('Connecting');
  try {
    const response = await sessionRequest('POST', bearer);
    if (!response.ok) throw new Error(response.status === 401 ? 'Access token rejected' : 'Could not connect');
    stop(true);
    setConnected(true);
    summary.textContent = 'Loading memories…';
    refresh();
  } catch (error) {
    setStatus(error.name === 'AbortError' ? 'Request timed out' : error instanceof TypeError ? 'Service unavailable' : error.message, 'error');
  } finally {
    loggingIn = false;
    connectButton.disabled = false;
  }
});

disconnectButton.addEventListener('click', async () => {
  disconnectButton.disabled = true;
  try {
    const response = await sessionRequest('DELETE');
    if (!response.ok) throw new Error('Could not disconnect');
    stop(true);
    setStatus('Not connected');
  } catch (error) {
    disconnectButton.disabled = false;
    setStatus('Could not disconnect. Try again when the service is available.', 'error');
  }
});

(async () => {
  try {
    const response = await sessionRequest('GET');
    if (!response.ok) throw new Error('Service unavailable');
    const session = await response.json();
    if (session.connected && !loggingIn && !active) {
      setConnected(true);
      setStatus('Connecting');
      summary.textContent = 'Loading memories…';
      refresh();
    }
  } catch {
    if (!loggingIn && !active) setStatus('Service unavailable', 'error');
  }
})();

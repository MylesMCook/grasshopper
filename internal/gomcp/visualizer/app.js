const form = document.getElementById('connection-form');
const tokenInput = document.getElementById('token');
const tokenField = document.getElementById('token-field');
const connectButton = document.getElementById('connect');
const disconnectButton = document.getElementById('disconnect');
const status = document.getElementById('status');
const summary = document.getElementById('summary');
const omissions = document.getElementById('omissions');
const records = document.getElementById('records');
const searchForm = document.getElementById('search-form');
const searchInput = document.getElementById('search-query');
const clearSearch = document.getElementById('clear-search');
const memoryDialog = document.getElementById('memory-dialog');
const detailTitle = document.getElementById('memory-detail-title');
const detailStatus = document.getElementById('memory-detail-status');
const detailContent = document.getElementById('memory-detail-content');
const detailMeta = document.getElementById('memory-detail-meta');
const history = document.getElementById('memory-history');
const revisionLabel = document.getElementById('memory-revision');
const previousRevision = document.getElementById('previous-revision');
const nextRevision = document.getElementById('next-revision');
const latestRevision = document.getElementById('latest-revision');
const scopeSummary = document.getElementById('scope-summary');
const projectSelect = document.getElementById('project');
const manualProjectLabel = document.getElementById('manual-project-label');
const manualProjectInput = document.getElementById('manual-project');
const deviceSelect = document.getElementById('device');
const manualDeviceLabel = document.getElementById('manual-device-label');
const manualDeviceInput = document.getElementById('manual-device');
const manualChoice = '\u0000manual';
const globalChoice = '\u0000global';
const devicePanel = document.getElementById('device-panel');
const browseControls = document.getElementById('browse-controls');
const refreshButton = document.getElementById('refresh');
const approvalNotice = document.getElementById('approval-notice');
const deviceStatus = document.getElementById('device-status');
const pendingDevices = document.getElementById('pending-devices');
const connectedDevices = document.getElementById('connected-devices');
const connectionPrompt = document.getElementById('connection-prompt');
const copyConnectionPrompt = document.getElementById('copy-connection-prompt');
connectionPrompt.textContent = `Connect Grasshopper to ${location.origin}/visualizer/`;
function requestFromHash() { return /^#connect=([0-9a-f]{64})$/.exec(location.hash)?.[1] || ''; }
let approvalID = requestFromHash();
let approvalFocused = false;

let active = false;
let loggingIn = false;
let timer = null;
let inFlight = null;
let revisions = new Map();
let hasLoaded = false;
let knownProjects = [];
let knownDevices = [];
let lastSignature = '';
let lastOmissionSignature = '';
let deviceTimer = null;
let deviceInFlight = null;
let searchQuery = '';
let detailState = null;
let detailInFlight = null;
let openButtons = new Map();

function option(value, text) {
  const item = document.createElement('option');
  item.value = value;
  item.textContent = text;
  return item;
}

function updateOptions(select, values, known, emptyLabel, manualLabel, preserveSelection = true, extra = []) {
  const names = Array.isArray(values) ? values.filter(value => typeof value === 'string') : [];
  if (preserveSelection && JSON.stringify(names) === JSON.stringify(known)) return known;
  const selected = preserveSelection ? select.value : '';
  const choices = [option('', emptyLabel), ...extra];
  for (const name of names) choices.push(option(name, name));
  if (selected && selected !== manualChoice && selected !== globalChoice && !names.includes(selected)) choices.push(option(selected, `Selected: ${selected}`));
  choices.push(option(manualChoice, manualLabel));
  select.replaceChildren(...choices);
  select.value = selected;
  return names;
}

function updateProjectOptions(projects, preserveSelection = true) {
  knownProjects = updateOptions(projectSelect, projects, knownProjects, 'All projects', 'Enter another project ID…', preserveSelection, [option(globalChoice, 'Global memories only')]);
}

function updateDeviceOptions(devices, preserveSelection = true) {
  knownDevices = updateOptions(deviceSelect, devices, knownDevices, 'All devices', 'Enter another device ID…', preserveSelection);
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
projectSelect.addEventListener('change', () => { showManualProject(); restartMemoryView(); });
deviceSelect.addEventListener('change', () => { showManualDevice(); restartMemoryView(); });
for (const input of [manualProjectInput, manualDeviceInput, document.getElementById('platform')]) input.addEventListener('change', restartMemoryView);

function setStatus(message, kind = '') {
  if (status.textContent !== message) status.textContent = message;
  const className = `status ${kind}`;
  if (status.className !== className) status.className = className;
}

function setConnected(value) {
  active = value;
  tokenInput.required = !value;
  tokenField.hidden = value;
  form.hidden = value;
  refreshButton.hidden = !value;
  disconnectButton.disabled = !value;
  disconnectButton.hidden = !value;
  browseControls.hidden = !value;
  scopeSummary.hidden = !value;
  devicePanel.hidden = !value;
  searchForm.hidden = !value;
  if (value && approvalID) openDevicePanel();
  else if (value && devicePanel.open) refreshDevices();
}

function stop(clearRecords = false) {
  setConnected(false);
  clearTimeout(timer);
  timer = null;
  if (inFlight) inFlight.abort();
  inFlight = null;
  stopDeviceRefresh();
  closeMemory();
  devicePanel.hidden = true;
  if (clearRecords) {
    records.replaceChildren();
    omissions.hidden = true;
    revisions = new Map();
    lastSignature = '';
    lastOmissionSignature = '';
    hasLoaded = false;
    searchQuery = '';
    searchInput.value = '';
    clearSearch.hidden = true;
    summary.textContent = 'Connect to see what is saved.';
    updateProjectOptions([], false);
    updateDeviceOptions([], false);
    showManualProject();
    showManualDevice();
    scopeInput();
  }
}

function openDevicePanel() {
  if (devicePanel.open) refreshDevices();
  else devicePanel.open = true; // The toggle event starts its first refresh.
}

copyConnectionPrompt.addEventListener('click', async () => {
  try {
    await navigator.clipboard.writeText(connectionPrompt.textContent);
    deviceStatus.textContent = 'Connection prompt copied.';
  } catch {
    deviceStatus.textContent = 'Select and copy the prompt above.';
  }
});

window.addEventListener('hashchange', () => {
  approvalID = requestFromHash();
  approvalFocused = false;
  if (approvalID && active) {
    openDevicePanel();
  }
});

devicePanel.addEventListener('toggle', refreshDevices);

function stopDeviceRefresh() {
  clearTimeout(deviceTimer);
  deviceTimer = null;
  if (deviceInFlight) deviceInFlight.abort();
  deviceInFlight = null;
}

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

async function deviceRequest(path, method = 'GET', body, controller = new AbortController()) {
  const timeout = setTimeout(() => controller.abort(), 5000);
  try {
    const response = await fetch(path, {
      method, headers: body ? { 'Content-Type': 'application/json' } : {},
      body: body ? JSON.stringify(body) : undefined,
      credentials: 'same-origin', cache: 'no-store', signal: controller.signal
    });
    if (!response.ok) throw new Error('Device controls unavailable');
    return response.status === 204 ? null : await response.json();
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
  stopDeviceRefresh();
  if (!active || !devicePanel.open) return;
  const controller = new AbortController();
  deviceInFlight = controller;
  try {
    const [pending, devices] = await Promise.all([
      deviceRequest('/visualizer/api/pairings', 'GET', undefined, controller),
      deviceRequest('/visualizer/api/devices', 'GET', undefined, controller)
    ]);
    if (!active || !devicePanel.open || deviceInFlight !== controller) return;
    pendingDevices.replaceChildren(...pending.map(request => {
      const row = deviceRow(`${request.device} · code ${request.code} · ${request.status}`,
        request.status === 'pending' ? [['Approve', () => decidePairing(request, 'approve')], ['Deny', () => decidePairing(request, 'deny')]] : []);
      if (request.request_id === approvalID) {
        row.classList.add('focused-request');
        row.tabIndex = -1;
        deviceStatus.textContent = request.status === 'pending' ? `Check ${request.device} and code ${request.code} before approving.` : `${request.device} · ${request.status}.`;
      }
      return row;
    }));
    if (approvalID && !approvalFocused) {
      const focused = pendingDevices.querySelector('.focused-request');
      if (focused) {
        approvalFocused = true;
        focused.focus();
        focused.scrollIntoView({ block: 'center' });
      }
    }
    const connected = devices.filter(device => !device.revoked_at);
    connectedDevices.replaceChildren(...connected.map(device => deviceRow(device.device, [['Disconnect', () => revokeDevice(device)]])));
    if (approvalID && !pending.some(request => request.request_id === approvalID)) {
      deviceStatus.textContent = 'This request is no longer available. Ask the agent to connect again.';
    }
    if (!pending.length) pendingDevices.replaceChildren(deviceRow('No connection requests waiting.'));
    if (!connected.length) connectedDevices.replaceChildren(deviceRow('No other devices connected.'));
    if (deviceStatus.textContent === 'Device controls unavailable. Memory view still works.') deviceStatus.textContent = '';
  } catch {
    if (deviceInFlight === controller) deviceStatus.textContent = 'Device controls unavailable. Memory view still works.';
  } finally {
    // A failed fetch also cancels its sibling. Superseded completions may not
    // render or schedule another polling loop.
    controller.abort();
    if (deviceInFlight === controller) {
      deviceInFlight = null;
      if (active && devicePanel.open) deviceTimer = setTimeout(refreshDevices, 5000);
    }
  }
}

function scopeInput() {
  const scope = {};
  const allProjects = projectSelect.value === '';
  const project = projectSelect.value === manualChoice ? manualProjectInput.value.trim() : projectSelect.value === globalChoice ? '' : projectSelect.value;
  const device = deviceSelect.value === manualChoice ? manualDeviceInput.value.trim() : deviceSelect.value;
  const platform = document.getElementById('platform').value;
  if (allProjects) scope.all_projects = true;
  if (project) scope.project = project;
  if (device) scope.device = device;
  if (platform) scope.platform = platform;
  const platforms = { macos: 'macOS', windows: 'Windows', linux: 'Linux' };
  const projectView = allProjects ? 'All projects' : project ? `${project} + global memories` : 'Global memories only';
  const view = [projectView, device || 'All devices', platforms[platform] || 'All platforms'];
  const description = view.join(' · ');
  if (scopeSummary.textContent !== description) scopeSummary.textContent = description;
  if (projectSelect.value === manualChoice && !project) return null;
  if (deviceSelect.value === manualChoice && !device) return null;
  return scope;
}

function label(text) {
  const span = document.createElement('span');
  span.textContent = text;
  return span;
}

function updatedLabel(record) {
  const date = new Date(record.updated_at);
  return Number.isNaN(date.getTime()) ? 'Update date unavailable' : `Updated ${date.toLocaleString()}`;
}

const platformNames = { macos: 'macOS', windows: 'Windows', linux: 'Linux' };

// Cards name the exact scope in words. A Git project and a project ID are
// different scopes even when the rest of their text matches, so the label says which.
function projectLabel(project) {
  if (project.startsWith('git:')) return `Git project ${project.slice(4)}`;
  if (project.startsWith('id:')) return `Project ID ${project.slice(3)}`;
  return `Project ${project}`;
}

function scopeLabelText(scope) {
  const project = scope.project && projectLabel(scope.project);
  const parts = [project, scope.device && `device ${scope.device}`, scope.platform && (platformNames[scope.platform] || scope.platform)].filter(Boolean);
  return parts.length ? parts.join(' · ') : 'Global';
}

// Only confirmed records and handoffs load into an agent's startup context.
function startupNote(record) {
  if (record.confirmed) return 'Confirmed';
  return record.purpose === 'handoff' ? 'Unconfirmed handoff · loads at startup' : 'Unconfirmed · not loaded at agent startup';
}

function recordMetadata(record) {
  const scope = record.scope || {};
  const dimensions = [scope.project && `Project ${scope.project}`, scope.device && `Device ${scope.device}`, scope.platform && `Platform ${scope.platform}`].filter(Boolean);
  return [
    dimensions.length ? dimensions.join(' · ') : 'Global',
    startupNote(record),
    updatedLabel(record),
    record.archived ? 'Archived' : 'Active',
    `Source: ${record.provenance?.source || 'Unknown'}`,
    `Agent: ${record.provenance?.harness || 'Unknown'}`,
    `Source device: ${record.provenance?.device || 'Unknown'}`
  ];
}

function cancelDetail() {
  if (detailInFlight) detailInFlight.abort();
  detailInFlight = null;
  detailState = null;
  detailTitle.textContent = 'Memory';
  detailStatus.textContent = '';
  detailContent.textContent = '';
  detailMeta.replaceChildren();
  history.hidden = true;
}

function closeMemory() {
  cancelDetail();
  if (memoryDialog.open) memoryDialog.close();
}

function openMemory(record) {
  cancelDetail();
  detailState = { id: record.id, scope: scopeInput(), revision: 0, maximumRevision: 0 };
  if (!detailState.scope || !active) { cancelDetail(); return; }
  if (!memoryDialog.open) memoryDialog.showModal();
  loadRecord();
}

async function loadRecord(revision = null) {
  if (!active || !detailState) return;
  const selected = detailState;
  if (detailInFlight) detailInFlight.abort();
  const controller = new AbortController();
  detailInFlight = controller;
  const timeout = setTimeout(() => controller.abort(), 5000);
  detailStatus.textContent = 'Loading memory…';
  for (const button of [previousRevision, nextRevision, latestRevision]) button.disabled = true;
  try {
    const response = await fetch('/visualizer/api/record', {
      method: 'POST', headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ scope: selected.scope, id: selected.id, revision }),
      cache: 'no-store', credentials: 'same-origin', signal: controller.signal
    });
    if (!response.ok) throw new Error(response.status === 401 ? 'Connection expired' : response.status === 404 ? 'Memory or revision not found' : 'Could not load memory');
    const record = await response.json();
    if (detailState !== selected || detailInFlight !== controller || !active) return;
    selected.revision = record.revision;
    if (revision === null) selected.maximumRevision = record.revision;
    detailTitle.textContent = record.title || `Memory #${record.id}`;
    detailContent.textContent = record.content;
    detailMeta.replaceChildren(...recordMetadata(record).map(label));
    revisionLabel.textContent = `Memory #${record.id} · Revision ${record.revision} of ${selected.maximumRevision}`;
    history.hidden = false;
    detailStatus.textContent = record.revision === selected.maximumRevision ? 'Latest saved revision' : 'Earlier revision · current memory is unchanged';
  } catch (error) {
    if (detailState !== selected || detailInFlight !== controller) return;
    if (error.message === 'Connection expired') { stop(true); setStatus('Connection expired', 'error'); return; }
    detailStatus.textContent = error.name === 'AbortError' ? 'Request timed out. Close and open this memory to retry.' : error.message;
  } finally {
    clearTimeout(timeout);
    if (detailInFlight === controller) {
      detailInFlight = null;
      previousRevision.disabled = selected.revision <= 1;
      nextRevision.disabled = selected.revision >= selected.maximumRevision;
      latestRevision.disabled = false;
    }
  }
}

document.getElementById('close-memory').addEventListener('click', closeMemory);
memoryDialog.addEventListener('close', () => { if (!memoryDialog.open) cancelDetail(); });
previousRevision.addEventListener('click', () => { if (detailState?.revision > 1) loadRecord(detailState.revision - 1); });
nextRevision.addEventListener('click', () => { if (detailState?.revision < detailState?.maximumRevision) loadRecord(detailState.revision + 1); });
latestRevision.addEventListener('click', () => loadRecord());

function restartMemoryView() {
  clearTimeout(timer);
  if (inFlight) inFlight.abort();
  inFlight = null;
  closeMemory();
  records.replaceChildren();
  omissions.hidden = true;
  lastSignature = '';
  lastOmissionSignature = '';
  hasLoaded = false;
  revisions = new Map();
  clearSearch.hidden = !searchQuery;
  if (active) { summary.textContent = searchQuery ? 'Searching…' : 'Loading memories…'; refresh(); }
}

searchForm.addEventListener('submit', event => {
  event.preventDefault();
  searchQuery = searchInput.value.trim();
  restartMemoryView();
});
clearSearch.addEventListener('click', () => { searchQuery = ''; searchInput.value = ''; restartMemoryView(); });

function emptyState(omitted) {
  const empty = document.createElement('div');
  empty.className = 'empty';
  const message = document.createElement('p');
  empty.append(message);
  if (omitted) { message.textContent = 'Open an omitted memory above to read its full content.'; return empty; }
  if (searchQuery) { message.textContent = 'No matching memories. Try fewer words, or search all projects.'; return empty; }
  if (projectSelect.value !== '' || deviceSelect.value !== '' || document.getElementById('platform').value !== '') {
    message.textContent = 'Nothing saved in this view. Choose All projects, All devices and All platforms to see everything.';
    return empty;
  }
  message.textContent = 'Nothing is saved yet. To try it:';
  const steps = document.createElement('ol');
  for (const text of [
    'Connect an agent: open Devices & connections below and give the agent the prompt.',
    'Ask it to save a preference, such as “Remember that I prefer short commit messages.”',
    'Start a fresh session and ask what it remembers about your preferences.',
    'Check that the memory appears here. Memories that are not confirmed yet are not loaded at startup.'
  ]) {
    const step = document.createElement('li');
    step.textContent = text;
    steps.append(step);
  }
  empty.append(steps);
  return empty;
}

function draw(page) {
  const items = page.records || [];
  const omitted = page.omitted || 0;
  omissions.hidden = !omitted;
  const omissionSignature = JSON.stringify([omitted, page.omitted_records, page.omitted_titles]);
  if (omitted && omissionSignature !== lastOmissionSignature) {
    const text = document.createElement('p');
    const listed = (page.omitted_records || []).length;
    text.textContent = `${omitted} ${omitted === 1 ? 'memory is' : 'memories are'} outside this list's size limit. ${listed < omitted ? `The first ${listed} are named below; narrow the list with filters or search to reach the other ${omitted - listed}.` : 'Open one below, or search to narrow the list.'}`;
    const links = document.createElement('div');
    links.className = 'omitted-links';
    for (const reference of page.omitted_records || []) {
      const button = document.createElement('button');
      button.type = 'button'; button.className = 'quiet';
      const title = page.omitted_titles?.[reference.id];
      button.textContent = title ? `Open “${title}” (#${reference.id})` : `Open memory #${reference.id}`;
      button.addEventListener('click', () => openMemory(reference));
      links.append(button);
    }
    omissions.replaceChildren(text, links);
  }
  lastOmissionSignature = omissionSignature;
  const signature = JSON.stringify(items.map(record => [record.id, record.revision, record.title, record.content, record.scope, record.provenance, record.confirmed, record.purpose, record.updated_at]));
  summary.textContent = `${items.length} ${items.length === 1 ? 'memory' : 'memories'}${searchQuery ? ' in search results' : ''}`;
  const selection = window.getSelection();
  const selectingRecord = selection && !selection.isCollapsed && (records.contains(selection.anchorNode) || records.contains(selection.focusNode));
  // Defer replacement while someone is selecting text; the next poll can draw it.
  if (hasLoaded && (signature === lastSignature || selectingRecord)) return;

  const next = new Map();
  // A poll redraw must not strand keyboard focus on a removed button.
  const focusedID = document.activeElement?.memoryID;
  openButtons = new Map();
  const fragment = document.createDocumentFragment();
  for (const record of items) {
    const key = `${record.id}`;
    next.set(key, record.revision);
    const article = document.createElement('article');
    article.className = 'record';
    if (hasLoaded && revisions.get(key) !== record.revision) article.classList.add('changed');
    const top = document.createElement('div');
    top.className = 'record-top';
    // Memory fields stay text nodes, so stored content is never interpreted as HTML.
    const kind = document.createElement('p');
    kind.className = 'record-kind';
    kind.textContent = record.purpose || 'Memory';
    const scope = record.scope || {};
    const scopeLabel = document.createElement('p');
    scopeLabel.className = 'record-scope';
    scopeLabel.textContent = scopeLabelText(scope);
    const number = document.createElement('p');
    number.textContent = `#${record.id}`;
    top.append(kind, scopeLabel, number);
    const heading = document.createElement('h3');
    heading.textContent = record.title || `Memory #${record.id}`;
    const content = document.createElement('p');
    content.className = 'record-content';
    const preview = Array.from(record.content || '');
    content.textContent = record.content_truncated || preview.length > 240 ? preview.slice(0, 240).join('') + '…' : record.content;
    const state = document.createElement('p');
    state.className = 'record-state';
    state.textContent = `${startupNote(record)} · ${updatedLabel(record)}`;
    const open = document.createElement('button');
    open.type = 'button'; open.className = 'quiet record-open';
    open.textContent = 'Open memory';
    open.setAttribute('aria-label', `Open memory: ${record.title || `#${record.id}`}`);
    open.memoryID = record.id;
    openButtons.set(record.id, open);
    open.addEventListener('click', () => openMemory(record));
    article.append(top, heading, state, content, open);
    fragment.append(article);
  }
  if (!items.length) {
    fragment.append(emptyState(omitted));
  }
  records.replaceChildren(fragment);
  if (focusedID !== undefined) openButtons.get(focusedID)?.focus();
  revisions = next;
  lastSignature = signature;
  hasLoaded = true;
}

async function refresh() {
  if (!active || inFlight) return;
  const controller = new AbortController();
  inFlight = controller;
  // Search can wait for the server's bounded inference and wording fallback.
  const timeout = setTimeout(() => controller.abort(), searchQuery ? 20000 : 5000);
  try {
    const scope = scopeInput();
    if (!scope) {
      setStatus('Enter the selected project or device ID', 'error');
      return;
    }
    const response = await fetch(searchQuery ? '/visualizer/api/search' : '/visualizer/api/context', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(searchQuery ? { scope, query: searchQuery } : scope),
      cache: 'no-store',
      credentials: 'same-origin',
      signal: controller.signal
    });
    if (!response.ok) throw new Error(response.status === 401 ? 'Connection expired' : response.status === 400 ? 'Scope not recognized' : 'Service unavailable');
    const page = await response.json();
    // A disconnected or replaced request must not redraw an older session.
    if (!active || inFlight !== controller) return;
    updateProjectOptions(page.projects);
    updateDeviceOptions(page.devices);
    draw(page);
    setStatus(searchQuery ? page.semantic_ready ? 'Search results' : 'Wording matches only · meaning search unavailable' : 'Live', 'live');
    // Browsing follows live writes. Search runs on submission or explicit refresh,
    // rather than repeating query inference while someone reads the results.
    if (!searchQuery) timer = setTimeout(refresh, 3000);
  } catch (error) {
    if (inFlight !== controller) return;
    const message = error.name === 'AbortError' ? 'Request timed out' : error instanceof TypeError ? 'Service unavailable' : error.message;
    if (message === 'Connection expired') stop(true);
    setStatus(message, 'error');
    summary.textContent = message === 'Connection expired' ? 'Connect again to see your memories.' : hasLoaded ? 'Showing the last results. Use Refresh to try again.' : 'No memories loaded. Use Refresh to try again.';
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
      approvalNotice.hidden = true;
      setConnected(true);
      setStatus('Connecting');
      summary.textContent = 'Loading memories…';
      refresh();
    } else if (approvalID && !session.connected) {
      tokenField.hidden = true;
      connectButton.hidden = true;
      approvalNotice.hidden = false;
      approvalNotice.textContent = 'Open this link in your already-connected memory view to approve the agent. Do not give the agent your access token.';
      setStatus('Approval needs your connected browser');
    }
  } catch {
    if (!loggingIn && !active) setStatus('Service unavailable', 'error');
  }
})();

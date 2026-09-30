const purposeSelect = document.getElementById('purpose');
const disconnectAll = document.getElementById('disconnect-all');
const exportControls = document.getElementById('export-controls');
let pageETag = '';
let moreInFlight = null;
let moreRequested = false;
let undoState = null;
let undoTimer = null;
const connectionSection = document.getElementById('connection-section');
const editTitleCount = document.getElementById('edit-title-count');
const editContentCount = document.getElementById('edit-content-count');
const searchHint = document.getElementById('search-hint');
const discardChanges = document.getElementById('discard-changes');
const keepEditing = document.getElementById('keep-editing');
const discardDraft = document.getElementById('discard-draft');
const restoreRevisionButton = document.getElementById('restore-revision');
const form = document.getElementById('connection-form');
const tokenInput = document.getElementById('token');
const tokenError = document.getElementById('token-error');
const devicesLink = document.getElementById('devices-link');
const showMore = document.getElementById('show-more');
const editActions = document.getElementById('edit-actions');
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
const viewTabs = document.getElementById('view-tabs');
const memoryActions = document.getElementById('memory-actions');
const editButton = document.getElementById('edit-memory');
const confirmButton = document.getElementById('confirm-memory');
const archiveButton = document.getElementById('archive-memory');
const restoreButton = document.getElementById('restore-memory');
const editForm = document.getElementById('edit-form');
const editTitle = document.getElementById('edit-title');
const editPurpose = document.getElementById('edit-purpose');
const editContent = document.getElementById('edit-content');
const editNote = document.getElementById('edit-note');
const saveEdit = document.getElementById('save-edit');
const conflictPanel = document.getElementById('conflict');
const conflictNote = document.getElementById('conflict-note');
const conflictText = document.getElementById('conflict-text');
const serverStatus = document.getElementById('server-status');
const startupControls = document.getElementById('startup-controls');
const startupAgent = document.getElementById('startup-agent');
const notLoaded = document.getElementById('not-loaded');
connectionPrompt.textContent = `Connect Grasshopper to ${location.origin}/visualizer/`;
function requestFromHash() { const id = new URLSearchParams(location.hash.slice(1)).get('connect') || ''; return /^[0-9a-f]{64}$/.test(id) ? id : ''; }
let approvalID = requestFromHash();
let approvalFocused = false;

let active = false;
let sessionGeneration = 0;
let loggingIn = false;
let timer = null;
let pollFailures = 0;
let wasHidden = Boolean(document.hidden);
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
let listView = '';
let pendingWrite = null;
let saving = false;
let visibleLimit = 8;
let latestPage = null;
const inlineWrites = new Map();
const inlineBusy = new Set();
let inlineActionRows = new Map();

const filterToggle = document.getElementById('toggle-filters');
filterToggle.addEventListener('click', () => {
  const expanded = filterToggle.getAttribute('aria-expanded') !== 'true';
  filterToggle.setAttribute('aria-expanded', String(expanded));
  browseControls.classList.toggle('filters-expanded', expanded);
});
showMore.addEventListener('click', () => {
  visibleLimit += 8;
  lastSignature = '';
  if (latestPage) { draw(latestPage); if (visibleLimit > latestPage.records.length && latestPage.next) loadMore(); }
});
devicesLink.addEventListener('click', event => {
  event.preventDefault();
  openDevicePanel();
  devicePanel.scrollIntoView({ block: 'start' });
  devicePanel.querySelector('summary')?.focus();
});
let leaveDraft = null;
let appliedRoute = null;
let linkedMemory = null;

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
for (const input of [manualProjectInput, manualDeviceInput, document.getElementById('platform'), startupAgent, purposeSelect]) input.addEventListener('change', restartMemoryView);

function setStatus(message, kind = '') {
  if (status.textContent !== message) status.textContent = message;
  const className = `status ${kind}`;
  if (status.className !== className) status.className = className;
}

function setConnected(value) {
  if (active !== value) sessionGeneration++;
  active = value;
  if (!value) { inlineWrites.clear(); inlineBusy.clear(); }
  devicesLink.hidden = !value;
  tokenError.hidden = true;
  tokenInput.setAttribute('aria-invalid', 'false');
  tokenInput.required = !value;
  tokenField.hidden = value;
  form.hidden = value;
  connectionSection.hidden = value;
  refreshButton.hidden = !value;
  disconnectButton.disabled = !value;
  disconnectButton.hidden = !value;
  disconnectAll.hidden = !value;
  exportControls.hidden = !value;
  browseControls.hidden = !value;
  scopeSummary.hidden = !value;
  devicePanel.hidden = !value;
  searchForm.hidden = !value || listView !== '';
  startupControls.hidden = !value || listView !== 'startup';
  viewTabs.hidden = !value;
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
  pageETag = "";
  moreInFlight?.abort();
  moreInFlight = null;
  moreRequested = false;
  clearUndo();
  pollFailures = 0;
  resetDeviceRows();
  closeMemory(true);
  devicePanel.hidden = true;
  serverStatus.hidden = true;
  if (clearRecords) {
    records.replaceChildren();
    showMore.hidden = true;
    latestPage = null;
    visibleLimit = 8;
    inlineWrites.clear();
    omissions.hidden = true;
    revisions = new Map();
    lastSignature = '';
    lastOmissionSignature = '';
    hasLoaded = false;
    searchQuery = '';
    searchInput.value = '';
    clearSearch.hidden = true;
    listView = '';
    startupControls.hidden = true;
    notLoaded.hidden = true;
    delete viewCounts.review;
    delete viewCounts.archived;
    showViewTabs();
    summary.textContent = 'Sign in to see what is saved.';
    updateProjectOptions([], false);
    updateDeviceOptions([], false);
    showManualProject();
    showManualDevice();
    scopeInput();
  }
}

// A connection request is time-limited, so its panel moves above the memory
// list instead of waiting at the bottom of a long page. Without a request it
// returns to its usual place after the list.
function placeDevicePanel() {
  if (approvalID) document.getElementById('memory-section')?.before?.(devicePanel);
  else serverStatus.before?.(devicePanel);
}

function openDevicePanel() {
  placeDevicePanel();
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
  const route = routeFromHash();
  if (dirtyDraft()) {
    replaceRouteURL(appliedRoute);
    askToLeaveDraft(() => applyRoute(route, true));
  } else applyRoute(route, false);
});

devicePanel.addEventListener('toggle', () => { if (!devicePanel.open) resetDeviceRows(); refreshDevices(); });

function resetDeviceRows() {
  deviceSignature = '';
  confirmingDevice = null;
}

function stopDeviceRefresh() {
  clearTimeout(deviceTimer);
  deviceTimer = null;
  if (deviceInFlight) deviceInFlight.abort();
  deviceInFlight = null;
}

// Buttons are [caption, action, primary]. Only a primary button is filled;
// approving a connection is the one decision that should stand out.
function deviceRow(text, buttons = [], detail = null) {
  const row = document.createElement('div');
  row.className = 'device-entry';
  const label = document.createElement('div');
  label.className = 'device-label';
  const name = document.createElement('p');
  name.textContent = text;
  label.append(name);
  if (detail) label.append(detail);
  row.append(label, deviceActions(buttons));
  return row;
}

function deviceActions(buttons) {
  const actions = document.createElement('div');
  actions.className = 'device-actions';
  for (const [caption, action, primary] of buttons) {
    const button = document.createElement('button');
    button.type = 'button';
    button.className = primary ? '' : 'quiet';
    button.textContent = caption;
    button.addEventListener('click', action);
    actions.append(button);
  }
  return actions;
}

function detailLine(...parts) {
  const line = document.createElement('p');
  line.className = 'device-detail';
  line.append(...parts);
  return line;
}

function timeLeft(seconds) {
  if (typeof seconds !== 'number') return '';
  if (seconds <= 0) return 'Expiring now';
  return seconds >= 90 ? `Expires in ${Math.ceil(seconds / 60)} min` : `Expires in ${seconds} s`;
}

// Devices are polled, so a row must not be rebuilt (and lose keyboard focus)
// unless something visible changed. Only the countdown updates in place.
let deviceSignature = '';
let confirmingDevice = null;
let expiryNodes = new Map();

function pendingRow(request) {
  const isPending = request.status === 'pending';
  const code = document.createElement('code');
  code.className = 'pairing-code';
  code.textContent = request.code;
  const expiry = document.createElement('span');
  expiry.textContent = isPending ? ` · ${timeLeft(request.expires_in)}` : ` · ${request.status}`;
  expiryNodes.set(request.request_id, expiry);
  return deviceRow(isPending ? `${request.device} wants to connect` : request.device,
    isPending ? [['Approve', () => decidePairing(request, 'approve'), true], ['Deny', () => decidePairing(request, 'deny')]] : [],
    detailLine('Code ', code, expiry));
}

function connectedRow(device) {
  const created = new Date(device.created_at);
  const detail = Number.isNaN(created.getTime()) ? null : detailLine(`Connected ${created.toLocaleDateString()}`);
  if (confirmingDevice === device.id) {
    const row = deviceRow(`Disconnect ${device.device}?`, [['Disconnect now', () => revokeDevice(device), true], ['Keep', () => { confirmingDevice = null; deviceSignature = ''; refreshDevices(); }]], detailLine('Its agent loses access immediately.'));
    return row;
  }
  return deviceRow(device.device, [['Disconnect', () => { confirmingDevice = device.id; deviceSignature = ''; refreshDevices(); }]], detail);
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
  confirmingDevice = null;
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
  if (!active || !devicePanel.open || document.hidden) return;
  const controller = new AbortController();
  deviceInFlight = controller;
  try {
    const [pending, devices] = await Promise.all([
      deviceRequest('/visualizer/api/pairings', 'GET', undefined, controller),
      deviceRequest('/visualizer/api/devices', 'GET', undefined, controller)
    ]);
    if (!active || !devicePanel.open || deviceInFlight !== controller) return;
    const connected = devices.filter(device => !device.revoked_at);
    const signature = JSON.stringify([pending.map(request => [request.request_id, request.code, request.device, request.status]), connected.map(device => [device.id, device.device, device.created_at]), confirmingDevice, approvalID]);
    if (signature !== deviceSignature) {
      deviceSignature = signature;
      expiryNodes = new Map();
      pendingDevices.replaceChildren(...pending.map(request => {
        const row = pendingRow(request);
        if (request.request_id === approvalID) {
          row.classList.add('focused-request');
          row.tabIndex = -1;
        }
        return row;
      }));
      connectedDevices.replaceChildren(...connected.map(connectedRow));
      if (!pending.length) pendingDevices.replaceChildren(deviceRow('No connection requests waiting.'));
      if (!connected.length) connectedDevices.replaceChildren(deviceRow('No other devices connected.'));
    }
    for (const request of pending) {
      const node = expiryNodes.get(request.request_id);
      if (node && request.status === 'pending') node.textContent = ` · ${timeLeft(request.expires_in)}`;
      if (request.request_id === approvalID) {
        deviceStatus.textContent = request.status === 'pending' ? `Check ${request.device} and code ${request.code} before approving.` : `${request.device} · ${request.status}.`;
      }
    }
    if (approvalID && !approvalFocused) {
      const focused = pendingDevices.querySelector('.focused-request');
      if (focused) {
        approvalFocused = true;
        focused.focus();
        focused.scrollIntoView({ block: 'center' });
      }
    }
    if (approvalID && !pending.some(request => request.request_id === approvalID)) {
      deviceStatus.textContent = 'This request is no longer available. Ask the agent to connect again.';
    }
    if (deviceStatus.textContent === 'Device controls unavailable. Memory view still works.') deviceStatus.textContent = '';
  } catch {
    if (deviceInFlight === controller) deviceStatus.textContent = 'Device controls unavailable. Memory view still works.';
  } finally {
    // A failed fetch also cancels its sibling. Superseded completions may not
    // render or schedule another polling loop.
    controller.abort();
    if (deviceInFlight === controller) {
      deviceInFlight = null;
      if (active && devicePanel.open && !document.hidden) deviceTimer = setTimeout(refreshDevices, 5000);
    }
  }
}

function scopeInput() {
  const scope = {};
  // An agent sends one project, so the preview never means "every project".
  const allProjects = projectSelect.value === '' && listView !== 'startup';
  const project = projectSelect.value === manualChoice ? manualProjectInput.value.trim() : projectSelect.value === globalChoice ? '' : projectSelect.value;
  const device = deviceSelect.value === manualChoice ? manualDeviceInput.value.trim() : deviceSelect.value;
  const platform = document.getElementById('platform').value;
  if (allProjects) scope.all_projects = true;
  if (listView === 'review' || listView === 'archived') scope.view = listView;
  if (project) scope.project = project;
  if (device) scope.device = device;
  if (platform) scope.platform = platform;
  if (purposeSelect.value && listView !== "startup") scope.purpose = purposeSelect.value;
  const platforms = { macos: 'macOS', windows: 'Windows', linux: 'Linux' };
  const projectView = allProjects ? 'All projects' : project ? `${project} + global memories` : listView === 'startup' && projectSelect.value === '' ? 'No project chosen · global memories only' : 'Global memories only';
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
  return Number.isNaN(date.getTime()) ? 'Update date unavailable' : `Updated ${date.toLocaleString(undefined, { year: 'numeric', month: 'short', day: 'numeric', hour: 'numeric', minute: '2-digit', timeZoneName: 'short' })}`;
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

function dirtyDraft() {
  return Boolean(detailState?.editing && detailState.draftBase !== JSON.stringify([editTitle.value, editPurpose.value, editContent.value]));
}

function askToLeaveDraft(action) {
  if (!dirtyDraft()) { action(); return; }
  leaveDraft = action;
  discardChanges.hidden = false;
  keepEditing.focus();
  discardChanges.scrollIntoView({ block: 'nearest' });
}

keepEditing.addEventListener('click', () => {
  leaveDraft = null;
  discardChanges.hidden = true;
  editContent.focus();
});
discardDraft.addEventListener('click', () => {
  const action = leaveDraft;
  leaveDraft = null;
  discardChanges.hidden = true;
  if (detailState) detailState.editing = false;
  action?.();
});

function cancelDetail() {
  if (detailInFlight) detailInFlight.abort();
  detailInFlight = null;
  detailState = null;
  leaveDraft = null;
  discardChanges.hidden = true;
  pendingWrite = null;
  // `saving` belongs to the request in flight, not to the dialog: it stays set
  // until that request ends, so closing the dialog cannot allow a second write.
  editForm.hidden = true;
  editActions.hidden = true;
  conflictPanel.hidden = true;
  detailContent.hidden = false;
  memoryActions.hidden = true;
  detailTitle.textContent = 'Memory';
  detailStatus.textContent = '';
  detailContent.textContent = '';
  detailMeta.replaceChildren();
  history.hidden = true;
}

function closeMemory(force = false) {
  const close = () => {
    cancelDetail();
    if (memoryDialog.open) memoryDialog.close();
    if (!force) { linkedMemory = null; saveRoute(); }
  };
  if (force) close();
  else askToLeaveDraft(close);
}

function openMemory(record, updateURL = true) {
  askToLeaveDraft(() => {
    cancelDetail();
    detailState = { id: record.id, scope: scopeInput(), revision: 0, maximumRevision: 0, record: null, editing: false };
    if (!detailState.scope || !active) { cancelDetail(); return; }
    linkedMemory = record.id;
    if (updateURL) saveRoute();
    if (!memoryDialog.open) memoryDialog.showModal();
    detailTitle.focus();
    loadRecord();
  });
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
    if (!response.ok) throw new Error(response.status === 401 ? 'Session expired' : response.status === 404 ? 'Memory or revision not found' : 'Could not load memory');
    const record = await response.json();
    if (detailState !== selected || detailInFlight !== controller || !active) return;
    selected.revision = record.revision;
    if (revision === null) { selected.maximumRevision = record.revision; selected.record = record; }
    detailTitle.textContent = record.title || `Memory #${record.id}`;
    detailContent.textContent = record.content;
    detailContent.className = record.purpose === "handoff" ? "record-content handoff-content" : "record-content";
    detailMeta.replaceChildren(...recordMetadata(record).map(label));
    if (record.revision > 1 && record.provenance?.harness === "memory-view") showOriginalSource(selected, record);
    revisionLabel.textContent = `Memory #${record.id} · Revision ${record.revision} of ${selected.maximumRevision}`;
    history.hidden = false;
    detailStatus.textContent = record.revision === selected.maximumRevision ? 'Latest saved revision' : 'Earlier revision · current memory is unchanged';
    showActions();
  } catch (error) {
    if (detailState !== selected || detailInFlight !== controller) return;
    if (error.message === 'Session expired') { stop(true); setStatus('Session expired', 'error'); return; }
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

document.getElementById('close-memory').addEventListener('click', () => closeMemory());
memoryDialog.addEventListener('cancel', event => { event.preventDefault(); closeMemory(); });
memoryDialog.addEventListener('click', event => {
  if (event.target !== memoryDialog) return;
  const box = memoryDialog.getBoundingClientRect?.();
  if (!box || event.clientX < box.left || event.clientX > box.right || event.clientY < box.top || event.clientY > box.bottom) closeMemory();
});
memoryDialog.addEventListener('close', () => { if (!memoryDialog.open) cancelDetail(); });
previousRevision.addEventListener('click', () => { if (detailState?.revision > 1) loadRecord(detailState.revision - 1); });
nextRevision.addEventListener('click', () => { if (detailState?.revision < detailState?.maximumRevision) loadRecord(detailState.revision + 1); });
latestRevision.addEventListener('click', () => loadRecord());


function newRequestID() {
  const id = globalThis.crypto?.randomUUID?.();
  return id || `mv-${Date.now().toString(36)}-${Math.random().toString(36).slice(2, 12)}`;
}

// Owner actions post one JSON body and return the status with any JSON reply.
async function postOwner(path, body) {
  const controller = new AbortController();
  const timeout = setTimeout(() => controller.abort(), 20000);
  try {
    const response = await fetch(path, {
      method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(body),
      cache: 'no-store', credentials: 'same-origin', signal: controller.signal
    });
    const text = await response.text();
    let data;
    try { data = JSON.parse(text); } catch { data = { error: text.trim() }; }
    return { status: response.status, data };
  } finally {
    clearTimeout(timeout);
  }
}

// A retry of the same change reuses its request ID, so an uncertain first
// attempt cannot save twice; any different change gets a new ID.
function requestIDFor(body) {
  const fingerprint = JSON.stringify(body);
  if (!pendingWrite || pendingWrite.fingerprint !== fingerprint) pendingWrite = { fingerprint, id: newRequestID() };
  return pendingWrite.id;
}

function showActions() {
  const state = detailState;
  const record = state?.record;
  const latest = Boolean(record) && state.revision === state.maximumRevision && record.revision === state.maximumRevision;
  const earlier = Boolean(record) && state.revision > 0 && state.revision < state.maximumRevision && !record.archived;
  memoryActions.hidden = (!latest && !earlier) || Boolean(state?.editing);
  if (record) history.hidden = Boolean(state.editing);
  const archived = Boolean(record?.archived);
  editButton.hidden = !latest || archived;
  confirmButton.hidden = !latest || archived || Boolean(record.confirmed);
  archiveButton.hidden = !latest || archived;
  restoreButton.hidden = !latest || !archived;
  restoreRevisionButton.hidden = !earlier;
  for (const button of [editButton, confirmButton, archiveButton, restoreButton, restoreRevisionButton]) button.disabled = saving;
}

function updateEditLimits() {
  const titleBytes = new TextEncoder().encode(editTitle.value.trim()).length;
  const contentBytes = new TextEncoder().encode(editContent.value).length;
  editTitleCount.textContent = `${titleBytes.toLocaleString()} / 512 bytes`;
  editContentCount.textContent = `${contentBytes.toLocaleString()} / 32,768 bytes`;
  const valid = titleBytes <= 512 && contentBytes <= 32768 && Boolean(editContent.value.trim());
  editTitle.setAttribute('aria-invalid', String(titleBytes > 512));
  editContent.setAttribute('aria-invalid', String(contentBytes > 32768));
  saveEdit.disabled = saving || !valid;
  return valid;
}

function invalidChangeMessage(reason) {
  if (reason === 'content must be 1-32768 bytes') return 'Text must contain 1 to 32,768 bytes. Shorten it and try again.';
  if (reason === 'metadata too large') return 'Title or tags are too long. Keep the title within 512 bytes.';
  if (reason === 'invalid purpose') return 'Choose a valid memory kind and try again.';
  return 'This change is not valid. Check the text and try again.';
}

editTitle.addEventListener('input', updateEditLimits);
editContent.addEventListener('input', updateEditLimits);

// Highlight only literal query terms; memory text never becomes HTML.
function highlightMatches(node, text) {
  node.textContent = text;
  if (!searchQuery || !text) return;
  const terms = [...new Set(searchQuery.split(/\s+/).filter(Boolean))].sort((a, b) => b.length - a.length);
  const pattern = terms.map(term => term.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')).join('|');
  if (!pattern) return;
  const matcher = new RegExp(pattern, 'giu');
  let start = 0;
  const parts = [];
  for (const match of text.matchAll(matcher)) {
    parts.push(text.slice(start, match.index));
    const strong = document.createElement('strong');
    strong.textContent = match[0];
    parts.push(strong);
    start = match.index + match[0].length;
  }
  if (!parts.length) return;
  parts.push(text.slice(start));
  node.replaceChildren(...parts);
}

function startEdit() {
  const state = detailState;
  if (!state?.record || state.record.archived) return;
  state.editing = true;
  editTitle.value = state.record.title || '';
  editPurpose.value = state.record.purpose || 'observation';
  editContent.value = state.record.content;
  state.draftBase = JSON.stringify([editTitle.value, editPurpose.value, editContent.value]);
  conflictPanel.hidden = true;
  editForm.hidden = false;
  editActions.hidden = false;
  detailContent.hidden = true;
  showActions();
  updateEditLimits();
  editContent.focus();
}

function endEdit() {
  if (!detailState) return;
  detailState.editing = false;
  detailState.conflicted = false;
  pendingWrite = null;
  editForm.hidden = true;
  editActions.hidden = true;
  conflictPanel.hidden = true;
  detailContent.hidden = false;
  showActions();
}

function showConflict(current, reason) {
  const state = detailState;
  if (reason === 'archived') {
    detailStatus.textContent = 'This memory is archived. Restore it before editing.';
    endEdit();
    loadRecord();
    return;
  }
  if (!current) { detailStatus.textContent = 'This memory changed. Close and reopen it to see the newer text.'; return; }
  state.record = current;
  state.conflicted = true;
  state.revision = state.maximumRevision = current.revision;
  conflictNote.textContent = `This memory changed after you opened it (now revision ${current.revision}, ${updatedLabel(current)}). Nothing was overwritten. Newer text:`;
  conflictText.textContent = current.content;
  conflictPanel.hidden = false;
  conflictPanel.scrollIntoView({ block: 'nearest' });
  editNote.textContent = 'Saving again replaces the newer text with yours. Earlier revisions stay in history.';
  detailStatus.textContent = 'Review the newer text, then save again or cancel.';
}

// Returns true when the owner's change was stored.
async function changeMemory(path, fields, done) {
  const state = detailState;
  if (!state?.record || !active) return false;
  if (saving) { detailStatus.textContent = 'Another save is still finishing. Try again in a moment.'; return false; }
  const body = { id: state.id, expected_revision: state.record.revision, ...fields };
  body.request_id = requestIDFor(body);
  saving = true;
  saveEdit.disabled = true;
  for (const button of [editButton, confirmButton, archiveButton, restoreButton, restoreRevisionButton]) button.disabled = true;
  detailStatus.textContent = 'Saving…';
  try {
    const { status, data } = await postOwner(path, body);
    if (detailState !== state) {
      // The dialog closed while the request ran; the list still needs the result.
      if (status === 200 && active) refreshList();
      return false;
    }
    if (status === 200) {
      pendingWrite = null;
      await done(data);
      return true;
    }
    if (status === 401) { stop(true); setStatus('Session expired', 'error'); return false; }
    if (status === 409) {
      pendingWrite = null;
      if (state.editing) showConflict(data?.current, data?.error);
      else {
        // Confirm, archive and restore have no draft to keep: show the latest.
        await loadRecord();
        if (detailState === state) detailStatus.textContent = 'This memory changed while you were looking at it. Showing the latest version; review it and try again.';
      }
      return false;
    }
    detailStatus.textContent = status === 400 ? invalidChangeMessage(data?.error) : 'Could not save. Your text is still here; try again.';
    return false;
  } catch (error) {
    if (detailState === state) detailStatus.textContent = error.name === 'AbortError' ? 'The save timed out. Try again; a repeat cannot save twice.' : 'Could not save. Your text is still here; try again.';
    return false;
  } finally {
    saving = false;
    updateEditLimits();
    showActions();
  }
}

// List previews may be truncated. Read full text before confirmation and refuse
// a changed revision, so one-click review cannot overwrite a concurrent edit.
async function reviewInline(reference, archived, actions) {
  if (!active || inlineBusy.has(reference.id)) return;
  const generation = sessionGeneration;
  const currentSession = () => active && sessionGeneration === generation;
  inlineBusy.add(reference.id);
  for (const button of actions.children) button.disabled = true;
  const key = `${reference.id}:${archived}`;
  setStatus(archived ? 'Archiving…' : 'Confirming…');
  try {
    let pending = inlineWrites.get(key);
    if (!pending) {
      const { status: readStatus, data: record } = await postOwner('/visualizer/api/record', { scope: scopeInput(), id: reference.id });
      if (!currentSession()) return;
      if (readStatus === 401) { stop(true); setStatus('Session expired', 'error'); return; }
      if (readStatus !== 200) throw new Error('Could not read the memory. Try again.');
      if (record.revision !== reference.revision || record.archived) {
        setStatus('This memory changed. Review the latest text before trying again.', 'error');
        refreshList();
        return;
      }
      pending = { path: archived ? '/visualizer/api/archive' : '/visualizer/api/update', body: {
        id: record.id, expected_revision: record.revision,
        ...(archived ? { archived: true } : { action: 'confirm' }),
        request_id: newRequestID()
      } };
      inlineWrites.set(key, pending);
    }
    const { status: writeStatus, data: receipt } = await postOwner(pending.path, pending.body);
    if (!currentSession()) return;
    if (writeStatus === 200) {
      inlineWrites.delete(key);
      if (archived) offerUndo(reference, receipt.revision);
      setStatus(archived ? 'Memory archived. Restore it from Archived.' : 'Memory confirmed.', 'live');
      refreshList();
    } else if (writeStatus === 401) { stop(true); setStatus('Session expired', 'error'); }
    else if (writeStatus === 409) {
      inlineWrites.delete(key);
      setStatus('This memory changed. Review the latest text before trying again.', 'error');
      refreshList();
    } else throw new Error('Could not save. Try again; a repeat cannot save twice.');
  } catch (error) {
    if (currentSession()) setStatus(error.name === 'AbortError' ? 'The request timed out. Try again; a repeat cannot save twice.' : error.message, 'error');
  } finally {
    if (currentSession()) {
      inlineBusy.delete(reference.id);
      for (const row of [actions, inlineActionRows.get(reference.id)]) {
        for (const button of row?.children || []) button.disabled = false;
      }
    }
  }
}

function refreshList() {
  clearTimeout(timer);
  refresh();
}

async function saveMemoryEdit(event) {
  event.preventDefault();
  const content = editContent.value;
  if (!content.trim()) { detailStatus.textContent = 'Write some text before saving.'; return; }
  if (!updateEditLimits()) { detailStatus.textContent = 'Keep the title within 512 bytes and text within 32,768 bytes.'; return; }
  await changeMemory('/visualizer/api/update', { title: editTitle.value.trim(), content, purpose: editPurpose.value }, async receipt => {
    endEdit();
    await loadRecord();
    detailStatus.textContent = `Saved as revision ${receipt.revision} and confirmed. Agents read this text from their next session.`;
    refreshList();
  });
}

async function confirmMemory() {
  const record = detailState?.record;
  if (!record) return;
  await changeMemory('/visualizer/api/update', { action: 'confirm' }, async receipt => {
    await loadRecord();
    detailStatus.textContent = `Confirmed as revision ${receipt.revision}.`;
    refreshList();
  });
}

async function setArchived(archived) {
  const record = detailState?.record;
  if (!record) return;
  await changeMemory('/visualizer/api/archive', { archived }, async receipt => {
    if (archived) offerUndo(record, receipt.revision);
    const name = record.title || `Memory #${record.id}`;
    closeMemory();
    setStatus(archived ? `Archived “${name}”. Restore it from Archived.` : `Restored “${name}”.`, 'live');
    refreshList();
  });
}

async function restoreEarlierRevision() {
  const revision = detailState?.revision;
  if (!revision || revision >= detailState.maximumRevision) return;
  await changeMemory('/visualizer/api/update', { restore_revision: revision }, async receipt => {
    await loadRecord();
    detailStatus.textContent = `Restored revision ${revision} as revision ${receipt.revision} and confirmed. Agents read this text from their next session.`;
    refreshList();
  });
}
restoreRevisionButton.addEventListener('click', restoreEarlierRevision);

editButton.addEventListener('click', startEdit);
confirmButton.addEventListener('click', confirmMemory);
archiveButton.addEventListener('click', () => setArchived(true));
restoreButton.addEventListener('click', () => setArchived(false));
editForm.addEventListener('submit', saveMemoryEdit);
document.getElementById('cancel-edit').addEventListener('click', () => askToLeaveDraft(() => {
  // After a conflict the page still shows the text from before it; reload the latest.
  const stale = detailState?.conflicted;
  endEdit();
  if (stale) loadRecord();
  else detailStatus.textContent = '';
}));

const viewNames = { '': 'Saved', review: 'Needs review', archived: 'Archived', startup: 'Startup preview' };
const viewCounts = {};

function showViewTabs() {
  for (const tab of viewTabs.children || []) {
    const name = tab.dataset?.view ?? '';
    const count = viewCounts[name];
    // An unknown count shows no number; zero is a real count.
    tab.textContent = count === undefined ? viewNames[name] : `${viewNames[name]} (${count})`;
    tab.setAttribute('aria-pressed', String(name === listView));
  }
}

function chooseView(name) {
  if (name === listView) return;
  askToLeaveDraft(() => {
  listView = name;
  searchQuery = '';
  searchInput.value = '';
  searchForm.hidden = !active || Boolean(name);
  startupControls.hidden = !active || name !== 'startup';
  notLoaded.hidden = true;
  showViewTabs();
  restartMemoryView();
  });
}

function updateServerStatus(info) {
  if (!info) return;
  const search = info.model ? `meaning search ${info.model}` : 'wording search only';
  const text = `Server ${info.version} · ${search} · ${info.memories} ${info.memories === 1 ? 'memory' : 'memories'}, ${info.archived} archived`;
  if (serverStatus.textContent !== text) serverStatus.textContent = text;
  serverStatus.hidden = !active;
}

function updateViewCounts(page) {
  if (page.review_count === undefined) return;
  viewCounts.review = page.review_count;
  viewCounts.archived = page.archived_count;
  showViewTabs();
}

for (const tab of viewTabs.children || []) tab.addEventListener('click', () => chooseView(tab.dataset?.view ?? ''));

function restartMemoryView() {
  const desired = routeFromFields();
  if (dirtyDraft()) {
    setRouteFields(appliedRoute);
    askToLeaveDraft(() => { setRouteFields(desired); restartMemoryView(); });
    return;
  }
  clearUndo();
  linkedMemory = null;
  saveRoute();
  restartList();
}

function restartList() {
  clearTimeout(timer);
  if (inFlight) inFlight.abort();
  inFlight = null;
  closeMemory(true);
  records.replaceChildren();
  showMore.hidden = true;
  latestPage = null;
  pageETag = "";
  moreInFlight?.abort();
  moreInFlight = null;
  moreRequested = false;
  showMore.disabled = false;
  visibleLimit = 8;
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

function routeFromHash() {
  const params = new URLSearchParams(location.hash.slice(1));
  const bounded = (key, size) => {
    const text = params.get(key) || '';
    return text.length <= size && !/[\r\n]/.test(text) ? text : '';
  };
  const id = Number(params.get('memory'));
  const view = params.get('view') || '';
  const connect = params.get('connect') || '';
  return { view: Object.hasOwn(viewNames, view) ? view : '', project: bounded('project', 512),
    device: bounded('device', 256), platform: ['macos', 'windows', 'linux'].includes(params.get('platform')) ? params.get('platform') : '',
    purpose: ['preference', 'decision', 'lesson', 'handoff', 'observation'].includes(params.get('purpose')) ? params.get('purpose') : '', query: bounded('query', 4096), memory: Number.isSafeInteger(id) && id > 0 ? id : null,
    connect: /^[0-9a-f]{64}$/.test(connect) ? connect : '' };
}

function routeFromFields() {
  return { view: listView, project: projectSelect.value === manualChoice ? manualProjectInput.value.trim() : projectSelect.value,
    device: deviceSelect.value === manualChoice ? manualDeviceInput.value.trim() : deviceSelect.value,
    purpose: purposeSelect.value, platform: document.getElementById('platform').value, query: searchQuery, memory: linkedMemory, connect: approvalID };
}

function routeHash(route) {
  const params = new URLSearchParams();
  for (const key of ['connect', 'view', 'project', 'device', 'platform', 'purpose', 'query', 'memory']) {
    if (route?.[key]) params.set(key, String(route[key]));
  }
  const hash = params.toString();
  return hash ? `#${hash}` : '';
}

function replaceRouteURL(route) { window.history.replaceState(null, '', routeHash(route) || location.pathname || '/visualizer/'); }

function saveRoute() {
  const route = routeFromFields();
  const hash = routeHash(route);
  if (location.hash !== hash) window.history.pushState(null, '', hash || location.pathname || '/visualizer/');
  appliedRoute = route;
}

function setRouteFields(route) {
  if (!route) return;
  purposeSelect.value = route.purpose || "";
  listView = route.view;
  searchQuery = route.query;
  searchInput.value = searchQuery;
  for (const [select, value] of [[projectSelect, route.project], [deviceSelect, route.device]]) {
    if (value && ![...(select.options || [])].some(item => item.value === value)) select.append(option(value, value));
    select.value = value;
  }
  manualProjectInput.value = '';
  manualDeviceInput.value = '';
  manualProjectLabel.hidden = true;
  manualDeviceLabel.hidden = true;
  manualProjectInput.required = false;
  manualDeviceInput.required = false;
  document.getElementById('platform').value = route.platform;
  searchForm.hidden = !active || Boolean(listView);
  startupControls.hidden = !active || listView !== 'startup';
  showViewTabs();
}

function applyRoute(route, push) {
  setRouteFields(route);
  linkedMemory = route.memory;
  approvalID = route.connect;
  approvalFocused = false;
  appliedRoute = route;
  if (push) saveRoute();
  else replaceRouteURL(route);
  if (active) {
    restartList();
    if (route.memory) openMemory({ id: route.memory }, false);
    if (approvalID) openDevicePanel();
    else placeDevicePanel();
  }
}

function emptyState(omitted) {
  const empty = document.createElement('div');
  empty.className = 'empty';
  const message = document.createElement('p');
  empty.append(message);
  if (omitted) { message.textContent = 'Open an omitted memory above to read its full content.'; return empty; }
  if (listView === 'review') {
    message.textContent = 'Nothing needs review. Memories that agents save without your confirmation appear here. Handoffs are not listed: they load at startup without confirmation.';
    return empty;
  }
  if (listView === 'startup') {
    message.textContent = 'Nothing would load at startup for this scope. Confirm memories, or choose the project, device and platform your agent uses.';
    return empty;
  }
  if (listView === 'archived') { message.textContent = 'No archived memories in this view. Archived memories no longer load for agents and can be restored.'; return empty; }
  if (searchQuery) { message.textContent = `No matching memories. Try fewer words${projectSelect.value ? ', or search all projects' : ''}.`; return empty; }
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

// The preview asks for one exact scope, as an agent would send it.
function startupScope(scope) {
  const { all_projects, view, ...exact } = scope;
  return exact;
}

const notLoadedReasons = {
  unconfirmed: 'Not confirmed. Confirm it to load at startup.',
  older_handoff: 'An older handoff. Only the latest handoff loads.',
  over_budget: 'Over the startup size budget.'
};

let lastNotLoaded = '';

function drawNotLoaded(page) {
  const items = page.not_loaded || [];
  const count = page.records?.length || 0;
  summary.textContent = `${count} ${count === 1 ? 'memory loads' : 'memories load'} at startup within ${Number(page.budget).toLocaleString()} bytes${items.length ? ` · ${page.not_loaded_total || items.length} not loaded` : ''}`;
  notLoaded.hidden = !items.length;
  // A memory's title and scope change only with its revision, so this identifies what is drawn.
  const signature = JSON.stringify(items.map(item => [item.id, item.revision, item.reason]));
  if (signature === lastNotLoaded) return;
  lastNotLoaded = signature;
  const heading = document.createElement('h2');
  heading.textContent = 'Not loaded at startup';
  const list = document.createElement('div');
  list.className = 'device-list';
  for (const item of items) {
    const title = item.title || `Memory #${item.id}`;
    const reason = document.createElement('p');
    reason.className = 'device-detail';
    reason.textContent = `${notLoadedReasons[item.reason] || item.reason} · ${scopeLabelText(item.scope || {})}`;
    const name = document.createElement('p');
    name.textContent = `${title} (#${item.id})`;
    const label = document.createElement('div');
    label.className = 'device-label';
    label.append(name, reason);
    const row = document.createElement('div');
    row.className = 'device-entry';
    const actions = deviceActions([['Open memory', () => openMemory(item)]]);
    actions.children[0]?.setAttribute?.('aria-label', `Open memory: ${title}`);
    row.append(label, actions);
    list.append(row);
  }
  notLoaded.replaceChildren(heading, list);
}

function draw(page) {
  latestPage = page;
  const items = page.records || [];
  const omitted = page.omitted || 0;
  showMore.hidden = items.length <= visibleLimit && !page.next;
  showMore.textContent = `Show more memories (${Math.max(0, (page.total || items.length) - visibleLimit)} remaining)`;
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
  const total = !searchQuery && Number.isSafeInteger(page.total) ? page.total : items.length;
  summary.textContent = searchQuery && !items.length ? 'No matching memories' : `${total} ${total === 1 ? 'memory' : 'memories'}${searchQuery ? ' in search results' : ''}`;
  const selection = window.getSelection();
  const selectingRecord = selection && !selection.isCollapsed && (records.contains(selection.anchorNode) || records.contains(selection.focusNode));
  // Defer replacement while someone is selecting text; the next poll can draw it.
  if (hasLoaded && (signature === lastSignature || selectingRecord)) return;

  const next = new Map();
  // A poll redraw must not strand keyboard focus on a removed button.
  const focusedID = document.activeElement?.memoryID;
  const focusedAction = document.activeElement?.memoryAction;
  openButtons = new Map();
  inlineActionRows = new Map();
  const fragment = document.createDocumentFragment();
  for (const record of items.slice(0, visibleLimit)) {
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
    highlightMatches(heading, record.title || `Memory #${record.id}`);
    const content = document.createElement('p');
    content.className = record.purpose === 'handoff' ? 'record-content handoff-content' : 'record-content';
    const preview = Array.from(record.content || '');
    highlightMatches(content, record.content_truncated || preview.length > 240 ? preview.slice(0, 240).join('') + '…' : record.content);
    const state = document.createElement('p');
    state.className = 'record-state';
    state.textContent = `${startupNote(record)} · ${updatedLabel(record)}`;
    if (listView === 'review') state.textContent += ` · Saved by ${record.provenance?.harness || 'an agent'} on ${record.provenance?.device || 'an unknown device'}${record.provenance?.source ? `: ${record.provenance.source}` : ''}`;
    const open = document.createElement('button');
    open.type = 'button'; open.className = 'quiet record-open';
    open.textContent = 'Open memory';
    open.setAttribute('aria-label', `Open memory: ${record.title || `#${record.id}`}`);
    open.memoryID = record.id;
    openButtons.set(record.id, open);
    open.addEventListener('click', () => openMemory(record));
    article.append(top, heading, state, content, open);
    if (listView === 'review') {
      const actions = document.createElement('div');
      actions.className = 'actions inline-review';
      inlineActionRows.set(record.id, actions);
      for (const [name, archived] of [['Confirm', false], ['Archive', true]]) {
        const button = document.createElement('button');
        button.type = 'button';
        button.textContent = name;
        button.memoryID = record.id;
        button.memoryAction = archived ? 'archive' : 'confirm';
        button.setAttribute('aria-label', `${name} memory: ${record.title || `#${record.id}`}`);
        if (archived) button.className = 'quiet';
        button.disabled = inlineBusy.has(record.id);
        button.addEventListener('click', () => reviewInline(record, archived, actions));
        actions.append(button);
      }
      article.append(actions);
    }
    fragment.append(article);
  }
  if (!items.length) {
    fragment.append(emptyState(omitted));
  }
  records.replaceChildren(fragment);
  if (focusedID !== undefined) {
    const action = Array.from(inlineActionRows.get(focusedID)?.children || []).find(button => button.memoryAction === focusedAction && !button.disabled);
    (action || openButtons.get(focusedID) || openButtons.values().next().value || refreshButton).focus();
  }
  revisions = next;
  lastSignature = signature;
  hasLoaded = true;
}

async function refresh() {
  if (!active || inFlight || moreInFlight || document.hidden) return;
  clearTimeout(timer);
  timer = null;
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
    const startup = listView === 'startup';
    const response = await fetch(startup ? '/visualizer/api/startup' : searchQuery ? '/visualizer/api/search' : '/visualizer/api/context', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json', ...(!searchQuery && pageETag ? { 'If-None-Match': pageETag } : {}) },
      body: JSON.stringify(startup ? { scope: startupScope(scope), budget: Number(startupAgent.value) || 12000 } : searchQuery ? { scope, query: searchQuery } : scope),
      cache: 'no-store',
      credentials: 'same-origin',
      signal: controller.signal
    });
    if (response.status === 304) {
      if (!active || inFlight !== controller) return;
      const recovered = pollFailures > 0;
      pollFailures = 0;
      // A changed snapshot may have deferred its redraw during text selection.
      if (latestPage) { draw(latestPage); if (startup) drawNotLoaded(latestPage); }
      if (recovered) setStatus('Live', 'live');
      return;
    }
    if (!response.ok) throw new Error(response.status === 401 ? 'Session expired' : response.status === 400 ? 'Scope not recognized' : 'Service unavailable');
    const page = await response.json();
    // A disconnected or replaced request must not redraw an older session.
    if (!active || inFlight !== controller) return;
    pollFailures = 0;
    const responseETag = response.headers?.get('ETag') || '';
    // If the snapshot changed, rebuild the loaded range from its new cursor.
    // Retaining old tail records would leave archived or edited memories stale.
    if (!startup && !searchQuery && latestPage?.records.length > (page.records || []).length) {
      const target = latestPage.records.length;
      const seen = new Set();
      while (page.next && page.records.length < target && !seen.has(page.next)) {
        seen.add(page.next);
        const nextResponse = await fetch('/visualizer/api/context', {
          method: 'POST', headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ ...scope, after: page.next }), cache: 'no-store', credentials: 'same-origin', signal: controller.signal
        });
        if (!nextResponse.ok) throw new Error('Could not refresh loaded memories');
        const nextPage = await nextResponse.json();
        if (!active || inFlight !== controller) return;
        const ids = new Set(page.records.map(record => record.id));
        page.records.push(...(nextPage.records || []).filter(record => !ids.has(record.id)));
        page.next = nextPage.next;
      }
    }
    pageETag = responseETag;
    if (startup) {
      draw(page);
      drawNotLoaded(page);
      setStatus('Live', 'live');
      return;
    }
    updateProjectOptions(page.projects);
    updateDeviceOptions(page.devices);
    updateViewCounts(page);
    updateServerStatus(page.server);
    notLoaded.hidden = true;
    draw(page);
    setStatus(searchQuery ? page.semantic_ready ? 'Search results' : 'Wording matches only · meaning search unavailable' : 'Live', 'live');
    searchHint.textContent = page.semantic_ready ? 'Meaning search can match related wording. Bold text marks exact words from your query.' : 'Wording search matches any query word. Bold text marks exact words from your query.';
    searchHint.hidden = !searchQuery;
  } catch (error) {
    if (inFlight !== controller) return;
    const message = error.name === 'AbortError' ? 'Request timed out' : error instanceof TypeError ? 'Service unavailable' : error.message;
    pollFailures++;
    if (message === 'Session expired') stop(true);
    setStatus(message, 'error');
    summary.textContent = message === 'Session expired' ? 'Sign in again to see your memories.' : hasLoaded ? `Showing the last results. ${searchQuery ? 'Use Refresh to try again.' : 'Trying again automatically.'}` : `No memories loaded. ${searchQuery ? 'Use Refresh to try again.' : 'Trying again automatically.'}`;
  } finally {
    clearTimeout(timeout);
    if (inFlight === controller) {
      inFlight = null;
      // Retry live views after failures too. Search remains explicit to avoid
      // repeating inference while someone reads the results.
      if (active && !searchQuery && !document.hidden && scopeInput()) {
        timer = setTimeout(refresh, pollFailures < 2 ? 3000 : pollFailures === 2 ? 10000 : 30000);
      }
      if (moreRequested) loadMore();
    }
  }
}

document.addEventListener('visibilitychange', () => {
  const hidden = Boolean(document.hidden);
  if (hidden === wasHidden) return;
  wasHidden = hidden;
  if (hidden) {
    clearTimeout(timer);
    timer = null;
    if (inFlight) inFlight.abort();
    inFlight = null;
    stopDeviceRefresh();
  } else if (active) {
    refresh();
    refreshDevices();
  }
});

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
  tokenError.hidden = true;
  tokenInput.setAttribute('aria-invalid', 'false');
  setStatus('Signing in');
  try {
    const response = await sessionRequest('POST', bearer);
    if (!response.ok) throw new Error(response.status === 401 ? 'Access token rejected' : 'Could not sign in');
    const route = routeFromHash();
    stop(true);
    approvalNotice.hidden = true;
    setConnected(true);
    summary.textContent = 'Loading memories…';
    applyRoute(route, false);
  } catch (error) {
    const message = error.message === 'Access token rejected' ? 'That owner access token was rejected. Check it with the person who runs your server, then try again.' : error.name === 'AbortError' ? 'Sign-in timed out. Try again.' : 'Could not reach the server to sign in. Try again when it is available.';
    tokenError.textContent = message;
    tokenError.hidden = false;
    tokenInput.setAttribute('aria-invalid', 'true');
    tokenInput.focus();
    summary.textContent = 'Sign-in failed. Check the token field above.';
    setStatus('Sign-in failed', 'error');
  } finally {
    loggingIn = false;
    connectButton.disabled = false;
  }
});

disconnectButton.addEventListener('click', () => askToLeaveDraft(async () => {
  disconnectButton.disabled = true;
  try {
    const response = await sessionRequest('DELETE');
    if (!response.ok) throw new Error('Could not sign out');
    stop(true);
    setStatus('Signed out');
  } catch (error) {
    disconnectButton.disabled = false;
    setStatus('Could not sign out. Try again when the service is available.', 'error');
  }
}));

applyRoute(routeFromHash(), false);

(async () => {
  try {
    const response = await sessionRequest('GET');
    if (!response.ok) throw new Error('Service unavailable');
    const session = await response.json();
    if (session.connected && !loggingIn && !active) {
      approvalNotice.hidden = true;
      setConnected(true);
      setStatus('Signing in');
      summary.textContent = 'Loading memories…';
      applyRoute(routeFromHash(), false);
    } else if (approvalID && !session.connected) {
      approvalNotice.hidden = false;
      approvalNotice.textContent = 'An agent is asking to connect. Sign in as the owner to review the device and code, then approve or deny. Never give the agent your access token.';
      setStatus('Sign in to review the connection request');
    }
  } catch {
    if (!loggingIn && !active) setStatus('Service unavailable', 'error');
  }
})();

function clearUndo() {
  clearTimeout(undoTimer);
  undoState = null;
  document.getElementById('archive-undo').hidden = true;
}
function offerUndo(record, revision) {
  clearUndo();
  undoState = { id: record.id, expected_revision: revision, archived: false, request_id: newRequestID() };
  document.getElementById('archive-undo-label').textContent = `Archived “${record.title || `Memory #${record.id}`}”.`;
  document.getElementById('archive-undo').hidden = false;
  undoTimer = setTimeout(clearUndo, 10000);
}
document.getElementById('undo-archive').addEventListener('click', async () => {
  const pending = undoState;
  if (!pending) return;
  try {
    const { status } = await postOwner('/visualizer/api/archive', pending);
    if (pending !== undoState) return;
    if (status === 200) { clearUndo(); pageETag = ''; setStatus('Memory restored.', 'live'); refreshList(); }
    else if (status === 409) { clearUndo(); setStatus('Memory changed. Open Archived to review it.', 'error'); }
    else { clearTimeout(undoTimer); undoTimer = setTimeout(clearUndo, 10000); setStatus('Could not restore. Try Undo again.', 'error'); }
  } catch { if (pending === undoState) { clearTimeout(undoTimer); undoTimer = setTimeout(clearUndo, 10000); setStatus('Could not restore. Try Undo again.', 'error'); } }
});
disconnectAll.addEventListener('click', () => {
  askToLeaveDraft(signOutEverywhere);
});
async function signOutEverywhere() {
  const generation = sessionGeneration;
  try {
    const { status } = await postOwner('/visualizer/api/session/revoke-all', {});
    if (generation !== sessionGeneration) return;
    if (status >= 200 && status < 300) { stop(true); setStatus('Signed out everywhere'); }
    else setStatus('Could not sign out everywhere. Try again.', 'error');
  } catch { if (generation === sessionGeneration) setStatus('Could not sign out everywhere. Try again.', 'error'); }
}
document.getElementById('export-memory').addEventListener('click', async () => {
  const generation = sessionGeneration;
  try {
    const { status, data } = await postOwner('/visualizer/api/export', { scope: scopeInput(), all: document.getElementById('export-mode').value === 'all', include_archived: document.getElementById('export-archived').checked === true });
    if (!active || generation !== sessionGeneration) return;
    if (status !== 200 || !Array.isArray(data?.records) || data.error) throw new Error();
    const url = URL.createObjectURL(new Blob([JSON.stringify(data, null, 2)], { type: 'application/json' }));
    const link = document.createElement('a'); link.href = url; link.download = 'grasshopper-memories.json'; link.click();
    setTimeout(() => URL.revokeObjectURL(url), 1000);
    setStatus('JSON downloaded.', 'live');
  } catch { if (active && generation === sessionGeneration) setStatus('Could not export memories. Try again.', 'error'); }
});
async function loadMore() {
  if (!active || document.hidden || moreInFlight || !latestPage?.next || searchQuery || listView === 'startup') { moreRequested = false; return; }
  // Polling can replace the cursor snapshot; finish it before starting a page.
  if (inFlight) { moreRequested = true; return; }
  moreRequested = false;
  clearTimeout(timer);
  timer = null;
  const selected = latestPage;
  const generation = sessionGeneration;
  const controller = new AbortController(); moreInFlight = controller;
  showMore.disabled = true;
  const timeout = setTimeout(() => controller.abort(), 5000);
  try {
    const response = await fetch('/visualizer/api/context', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ ...scopeInput(), after: selected.next }), credentials: 'same-origin', cache: 'no-store', signal: controller.signal });
    if (response.status === 401) { stop(true); setStatus('Session expired', 'error'); return; }
    if (!response.ok) throw new Error();
    const page = await response.json();
    if (!active || generation !== sessionGeneration || moreInFlight !== controller) return;
    const existing = latestPage.records;
    const ids = new Set(existing.map(record => record.id));
    draw({ ...latestPage, records: [...existing, ...(page.records || []).filter(record => !ids.has(record.id))], next: page.next, total: page.total });
  } catch { if (moreInFlight === controller) setStatus('Could not load more. Try again, or Refresh to reload an expired list.', 'error'); }
  finally { clearTimeout(timeout); if (moreInFlight === controller) { moreInFlight = null; showMore.disabled = false; if (active && !document.hidden) { clearTimeout(timer); timer = setTimeout(refresh, 3000); } } }
}

async function showOriginalSource(selected, shown) {
  try {
    const { status, data } = await postOwner('/visualizer/api/record', { scope: selected.scope, id: selected.id, revision: 1 });
    if (status === 200 && active && detailState === selected && selected.revision === shown.revision) {
      detailMeta.append(label(`Originally saved by ${data.provenance?.harness || 'an unknown agent'} on ${data.provenance?.device || 'an unknown device'}.`));
    }
  } catch { /* Current provenance and revision history remain available. */ }
}

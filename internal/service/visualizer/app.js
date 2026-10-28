'use strict';
// Grasshopper memory view. Stored memory text is only ever placed with
// textContent, so it can never become markup. Credentials stay in an HttpOnly
// session cookie set by the server; this script never stores them.

const root = document.getElementById('app');
const announcer = document.getElementById('announce');
const BUDGETS = { Codex: 12000, Cursor: 12000, 'Claude Code': 3000 };
const HARNESS = { 'codex-desktop': 'Codex', codex: 'Codex', claude: 'Claude Code', 'claude-code': 'Claude Code', cursor: 'Cursor', 'memory-view': 'you' };
const PLATFORM = { macos: 'macOS', windows: 'Windows', linux: 'Linux' };
const POLL = { page: 5000, index: 15000, devices: 5000, devicesIdle: 30000 };

const state = {
  checking: true, active: false, generation: 0, signingIn: false, signInError: '', signedOutNote: '',
  route: routeFromHash(),
  index: null, page: null, pageKey: '', etags: {}, offline: null, loading: false,
  devices: [], pairings: [], devicesLoaded: false,
  panel: null, expanded: new Set(), showGlobal: false, moreProjects: false, menu: false,
  undo: null, openVersions: new Set(), startupAgent: 'Codex', startupProject: '', exportArchived: false, exportStatus: ''
};
const timers = { page: null, index: null, devices: null, undo: null };
const inFlight = { page: null, index: null, devices: null };
const writes = new Map();
let saving = false;

// ---------- small helpers ----------

// A busy control uses aria-disabled rather than disabled: it keeps keyboard
// focus across redraws and simply gets no click handler.
function h(tag, props, ...children) {
  const node = document.createElement(tag);
  const busy = props?.['aria-disabled'] === true;
  for (const [key, value] of Object.entries(props || {})) {
    if (value === undefined || value === null || value === false) continue;
    if (key === 'class') node.className = value;
    else if (key === 'text') node.textContent = value;
    else if (key === 'onclick' && busy) continue;
    else if (key.startsWith('on')) node.addEventListener(key.slice(2), value);
    else if (key === 'value' || key === 'checked' || key === 'disabled' || key === 'hidden') node[key] = value;
    else node.setAttribute(key, value === true ? (key.startsWith('aria-') ? 'true' : '') : String(value));
  }
  for (const child of children.flat(Infinity)) if (child !== null && child !== undefined && child !== false) node.append(child);
  return node;
}

// Routes and memories are links so Cmd/Ctrl-click and middle-click open a new
// tab; a plain left click stays in place and keeps the unsaved-changes check.
function inPlace(action) {
  return event => {
    if (event.button || event.metaKey || event.ctrlKey || event.shiftKey || event.altKey) return;
    event.preventDefault();
    action(event);
  };
}
function routeLink(route) { return routeHash({ page: 'project', ...route }) || location.pathname; }

function say(message) { if (announcer) announcer.textContent = message; }

// Projects are named by the last part of their repository or ID; two that end
// the same way also show their owner.
function projectName(key) {
  if (!key) return 'All projects';
  const tail = text => text.replace(/^(git|id):/, '').split('/').filter(Boolean);
  const parts = tail(key);
  const short = parts[parts.length - 1] || key;
  const clash = (state.index?.projects || []).some(other => other !== key && tail(other).pop() === short);
  return clash && parts.length > 1 ? parts.slice(-2).join('/') : short;
}

// A device already implies its platform, so only one restriction shows.
function onlyLabel(scope = {}) {
  return scope.device ? `${deviceName(scope.device)} only` : scope.platform ? `${PLATFORM[scope.platform] || scope.platform} only` : '';
}
// New memories and moves need a project ID that survives clones and machines.
function durable(project) { return /^id:.+|^git:[^/]+\/.+/.test(project); }
function byName(projects) { return [...projects].sort((a, b) => projectName(a).localeCompare(projectName(b))); }
// The places these records apply, in the given order, then any others by
// name: the index leaves out projects with only archived memories, and a
// brand-new project can appear on a page before it reaches the index.
function places(records, order) {
  const present = new Set(records.map(record => record.scope?.project || null));
  return [...new Set([...order, ...byName([...present].filter(Boolean))])].filter(project => present.has(project));
}
// Where a memory can apply: every project, then durable projects by name, and
// always the selected one, even before the project list loads. Add memory and
// Move to share it.
function placeSelect(id, selected, onchange) {
  const projects = (state.index?.projects || []).filter(durable);
  if (selected && !projects.includes(selected)) projects.push(selected);
  return h('select', { id, onchange }, [null, ...byName(projects)].map(project =>
    h('option', { value: project || '', selected: (selected || '') === (project || '') ? true : null }, projectName(project))));
}
// A select of [value, label] pairs with one selected.
function optionSelect(id, options, selected, onchange) {
  return h('select', { id, onchange }, options.map(([value, label]) => h('option', { value, selected: value === (selected || '') ? true : null }, label)));
}
function whereLabel(scope = {}) { return [projectName(scope.project), onlyLabel(scope)].filter(Boolean).join(' · '); }

// The owner's name for a device, or its ID. The ID itself never changes.
function deviceName(id) { return state.devices.find(device => device.device === id && device.label)?.label || id; }
function by(record) { const harness = record?.provenance?.harness || ''; return HARNESS[harness] || HARNESS[harness.split('-')[0]] || harness || 'unknown'; }

function day(value) { const date = new Date(value || NaN); return Number.isNaN(date.getTime()) ? null : date; }
function daysAgo(value, now = new Date()) {
  const date = day(value);
  if (!date) return null;
  const local = d => Date.UTC(d.getFullYear(), d.getMonth(), d.getDate());
  return Math.round((local(now) - local(date)) / 86400000);
}
function excerpt(text, size = 160) { return text.length > size ? `${text.slice(0, size).trimEnd()}…` : text; }
// One earlier version: who saved it and when, Restore, its title if it
// differed, and its text, three lines until opened. Open ones survive redraws.
function versionBlock(record, older) {
  const key = `${record.id}:${older.revision}`;
  const open = state.openVersions.has(key);
  const long = older.content.length > 160 || older.content.split('\n').length > 3;
  const toggle = () => { if (open) state.openVersions.delete(key); else state.openVersions.add(key); render(); };
  return h('div', { class: 'version' },
    h('div', { class: 'version-head' }, h('span', { class: 'meta', text: `${by(older)} · ${fullDate(older.updated_at)}` }),
      record.archived ? null : h('button', { type: 'button', class: 'quiet', 'aria-disabled': saving, 'data-key': `restore-version:${older.revision}`, 'aria-label': `Restore version from ${relDate(older.updated_at)}`, onclick: () => restoreRevision(record, older) }, 'Restore')),
    older.title !== record.title ? h('p', { class: 'version-title', text: older.title }) : null,
    h('p', { class: `version-text${long && !open ? ' clamp' : ''}`, text: older.content }),
    long ? h('button', { type: 'button', class: 'quiet muted more', 'data-key': `version:${key}`, 'aria-expanded': String(open), onclick: toggle }, open ? 'Show less' : 'Show all') : null);
}
function shortDate(value) { const date = day(value); return date ? date.toLocaleDateString(undefined, { month: 'short', day: 'numeric' }) : ''; }
function relDate(value, now = new Date()) {
  const days = daysAgo(value, now);
  return days === null ? '' : days <= 0 ? 'today' : days === 1 ? 'yesterday' : shortDate(value);
}
function fullDate(value) { const date = day(value); return date ? date.toLocaleString(undefined, { dateStyle: 'medium', timeStyle: 'short' }) : ''; }
function ago(value, now = Date.now()) {
  const date = day(value);
  if (!date) return 'not seen yet';
  const minutes = Math.max(0, Math.round((now - date.getTime()) / 60000));
  if (minutes < 2) return 'active now';
  if (minutes < 60) return `${minutes} min ago`;
  if (minutes < 1440) return `${Math.round(minutes / 60)} h ago`;
  const days = Math.round(minutes / 1440);
  return `${days} ${days === 1 ? 'day' : 'days'} ago`;
}
function recentlySeen(device, now = Date.now()) { const date = day(device.last_seen_at); return Boolean(date) && now - date.getTime() < 3600000; }

// Groups memories by when they were last saved: this week, this month by
// name, then Earlier. Newest first within each group.
function recencyGroups(records, now = new Date()) {
  const sorted = [...records].sort((a, b) => String(b.updated_at).localeCompare(String(a.updated_at)) || b.id - a.id);
  const groups = [];
  for (const record of sorted) {
    const days = daysAgo(record.updated_at, now);
    const date = day(record.updated_at);
    const label = days !== null && days < 7 ? 'This week'
      : date && date.getFullYear() === now.getFullYear() && date.getMonth() === now.getMonth() ? date.toLocaleDateString(undefined, { month: 'long' })
      : 'Earlier';
    let group = groups.find(item => item.label === label);
    if (!group) groups.push(group = { label, records: [] });
    group.records.push(record);
  }
  return groups;
}

const STOP = new Set('the a an and or of to for in on at is are be by with from it this that not never use keep one most'.split(' '));
function words(text) { return new Set((String(text).toLowerCase().match(/[\p{L}\p{N}]+/gu) || []).filter(word => word.length > 2 && !STOP.has(word))); }
// A kept memory that shares at least three meaningful words with a suggestion.
function similarTo(record, kept) {
  const mine = words(`${record.title} ${record.content}`);
  let best = null, score = 0;
  for (const other of kept) {
    if (other.id === record.id || !other.confirmed || other.purpose === 'handoff') continue;
    if (other.scope?.project && other.scope.project !== record.scope?.project) continue;
    let shared = 0;
    for (const word of words(`${other.title} ${other.content}`)) if (mine.has(word)) shared++;
    if (shared > score) { score = shared; best = other; }
  }
  return score >= 3 ? best : null;
}

function newRequestID() {
  return globalThis.crypto?.randomUUID?.() || `mv-${Date.now().toString(36)}-${Math.random().toString(36).slice(2, 12)}`;
}
// A retry of the same change reuses its request ID, so an uncertain attempt
// can never save twice. A different change gets a new ID.
function requestIDFor(path, body) {
  const key = JSON.stringify([path, body]);
  if (!writes.has(key)) writes.set(key, newRequestID());
  return { key, id: writes.get(key) };
}

// ---------- routes ----------

function routeFromHash() {
  const params = new URLSearchParams(location.hash.slice(1));
  const pages = ['project', 'all', 'new', 'agents', 'archived', 'settings', 'search'];
  const bounded = (key, size) => { const text = params.get(key) || ''; return text.length <= size && !/[\r\n]/.test(text) ? text : ''; };
  const memory = Number(params.get('memory'));
  const connect = /^[0-9a-f]{64}$/.test(params.get('connect') || '') ? params.get('connect') : '';
  return {
    page: connect ? 'agents' : pages.includes(params.get('page')) ? params.get('page') : 'project',
    project: bounded('project', 512), query: bounded('query', 4096),
    memory: Number.isSafeInteger(memory) && memory > 0 ? memory : null, connect
  };
}
function routeHash(route) {
  const params = new URLSearchParams();
  for (const key of ['page', 'project', 'query', 'memory', 'connect']) if (route[key]) params.set(key, String(route[key]));
  const text = params.toString();
  return text ? `#${text}` : '';
}
function setURL(push) {
  const hash = routeHash(state.route) || location.pathname;
  if (push) window.history.pushState(null, '', hash); else window.history.replaceState(null, '', hash);
}
function go(route, push = true) {
  if (dirty() && !confirmLeave()) return false;
  state.route = { page: 'project', project: '', query: '', memory: null, connect: '', ...route };
  Object.assign(state, { panel: null, menu: false, expanded: new Set(), showGlobal: false, page: null, pageKey: '', approvalFocused: false, searchDraft: null });
  setURL(push);
  chooseDefaultProject();
  if (state.route.memory) openMemory(state.route.memory, false);
  restartPolling();
  render();
  document.getElementById('main')?.focus?.();
  return true;
}
const ROUTE_KEYS = ['page', 'project', 'query', 'memory', 'connect'];
window.addEventListener('hashchange', () => {
  // In-page anchors such as the skip link are not routes.
  if (![...new URLSearchParams(location.hash.slice(1)).keys()].some(key => ROUTE_KEYS.includes(key)) && location.hash) return;
  const route = routeFromHash();
  if (routeHash(route) !== routeHash(state.route) && !go(route, false)) setURL(false);
});
// Closing or reloading the tab asks before an unsaved edit is lost.
window.addEventListener('beforeunload', event => { if (dirty()) event.preventDefault(); });

// ---------- network ----------

async function api(path, body, { method = 'POST', etag = '', timeout = 8000, signal } = {}) {
  const controller = new AbortController();
  signal?.addEventListener?.('abort', () => controller.abort());
  const timer = setTimeout(() => controller.abort(), timeout);
  try {
    const headers = {};
    if (body !== undefined) headers['Content-Type'] = 'application/json';
    if (etag) headers['If-None-Match'] = etag;
    const response = await fetch(path, { method, headers, body: body === undefined ? undefined : JSON.stringify(body), credentials: 'same-origin', cache: 'no-store', signal: controller.signal });
    if (response.status === 401) { sessionEnded(); throw Object.assign(new Error('Session ended'), { session: true }); }
    if (response.status === 304) return { status: 304, data: null, etag };
    const text = response.status === 204 ? '' : await response.text();
    let data = null;
    try { data = text ? JSON.parse(text) : null; } catch { data = { error: text.trim() }; }
    return { status: response.status, data, etag: response.headers?.get?.('ETag') || '' };
  } finally { clearTimeout(timer); }
}

function sessionEnded() {
  if (!state.active) return;
  stopPolling();
  const draft = draftOf(state.panel);
  if (draft) state.keptDraft = draft;
  Object.assign(state, { active: false, generation: state.generation + 1, index: null, projectOrder: null, page: null, panel: null, devices: [], pairings: [], undo: null, signedOutNote: state.keptDraft ? 'Your session ended. Sign in again; your unsaved text is kept.' : 'Your session ended. Sign in again.' });
  writes.clear();
  render();
}

function listFor(route) {
  switch (route.page) {
    case 'all': return { url: '/visualizer/api/context', body: {} };
    case 'new': return { url: '/visualizer/api/context', body: { all_projects: true, view: 'review' } };
    case 'archived': return { url: '/visualizer/api/context', body: { all_projects: true, view: 'archived' } };
    case 'search': return route.query ? { url: '/visualizer/api/search', body: { scope: route.project ? { project: route.project } : { all_projects: true }, query: route.query } } : null;
    case 'project': return route.project ? { url: '/visualizer/api/context', body: { project: route.project } } : null;
    default: return null;
  }
}

// Loads every page of a list (bounded) so grouping and "Show N more" work
// locally. Polling asks for the first page with its ETag; a 304 keeps what is
// drawn, and any change reloads the whole list.
async function loadList(target, generation, signal, cached = true) {
  const key = target.url + JSON.stringify(target.body);
  // Only a list already on screen may be answered with 304.
  const first = await api(target.url, target.body, { etag: cached ? state.etags[key] || '' : '', signal, timeout: target.url.endsWith('search') ? 20000 : 8000 });
  if (first.status === 304) return null;
  if (first.status !== 200) throw new Error('unavailable');
  const page = first.data;
  page.records = page.records || [];
  const seen = new Set();
  while (page.next && !seen.has(page.next) && target.url.endsWith('context')) {
    seen.add(page.next);
    const more = await api(target.url, { ...target.body, after: page.next }, { signal });
    if (more.status !== 200 || generation !== state.generation) throw new Error('unavailable');
    const ids = new Set(page.records.map(record => record.id));
    page.records.push(...(more.data.records || []).filter(record => !ids.has(record.id)));
    page.next = more.data.next;
  }
  state.etags[key] = first.etag;
  return page;
}

async function refreshPage() {
  clearTimeout(timers.page);
  if (!state.active || document.hidden) return;
  const target = listFor(state.route);
  if (!target) return;
  const key = JSON.stringify(target);
  inFlight.page?.abort();
  const controller = new AbortController();
  inFlight.page = controller;
  const generation = state.generation;
  if (state.pageKey !== key) { state.loading = true; render(); }
  try {
    const page = await loadList(target, generation, controller.signal, Boolean(state.page) && state.pageKey === key);
    if (generation !== state.generation || inFlight.page !== controller) return;
    const changed = Boolean(page) || Boolean(state.offline) || state.loading;
    state.offline = null;
    if (page) { state.page = page; state.pageKey = key; }
    state.loading = false;
    redraw(changed);
  } catch (error) {
    if (error.session || generation !== state.generation || inFlight.page !== controller) return;
    state.loading = false;
    state.offline = state.offline || new Date();
    redraw(true);
  } finally {
    if (inFlight.page === controller) {
      inFlight.page = null;
      // Search is explicit; lists stay live.
      if (state.active && !document.hidden && state.route.page !== 'search') timers.page = setTimeout(refreshPage, state.offline ? 15000 : POLL.page);
    }
  }
}

// With no project chosen, open the most recently used one once the index is known.
function chooseDefaultProject() {
  if (state.route.page !== 'project' || state.route.project || !state.index) return false;
  state.route.project = sortedProjects()[0] || '';
  if (!state.route.project) state.route.page = 'all';
  setURL(false);
  return true;
}

// Most recent first, fixed when the view loads: the owner's own keeps and
// moves never reshuffle the list under the pointer. New projects join the end.
function sortedProjects() {
  const recency = state.index?.recency || new Map();
  const live = [...(state.index?.projects || [])].sort((a, b) => String(recency.get(b) || '').localeCompare(String(recency.get(a) || '')) || a.localeCompare(b));
  if (!live.length) return live;
  const added = live.filter(project => !state.projectOrder?.includes(project));
  if (added.length) state.projectOrder = [...(state.projectOrder || []), ...added];
  return state.projectOrder.filter(project => live.includes(project));
}

async function refreshIndex() {
  clearTimeout(timers.index);
  if (!state.active || document.hidden) return;
  inFlight.index?.abort();
  const controller = new AbortController();
  inFlight.index = controller;
  const generation = state.generation;
  try {
    const page = await loadList({ url: '/visualizer/api/context', body: { all_projects: true } }, generation, controller.signal, Boolean(state.index));
    if (generation !== state.generation || inFlight.index !== controller) return;
    const wasOffline = Boolean(state.offline);
    state.offline = null;
    if (page) {
      const recency = new Map();
      for (const record of page.records) {
        const project = record.scope?.project;
        if (project && !(recency.get(project) >= record.updated_at)) recency.set(project, record.updated_at);
      }
      state.index = { projects: page.projects || [], recency, records: page.records, review: page.review_count || 0, server: page.server, total: page.total ?? page.records.length };
    }
    if (chooseDefaultProject()) refreshPage();
    redraw(Boolean(page || wasOffline));
  } catch (error) {
    if (!error.session && generation === state.generation && inFlight.index === controller) { state.offline = state.offline || new Date(); redraw(true); }
  } finally {
    if (inFlight.index === controller) { inFlight.index = null; if (state.active && !document.hidden) timers.index = setTimeout(refreshIndex, POLL.index); }
  }
}

async function refreshDevices() {
  clearTimeout(timers.devices);
  if (!state.active || document.hidden) return;
  inFlight.devices?.abort();
  const controller = new AbortController();
  inFlight.devices = controller;
  const generation = state.generation;
  try {
    const [pairings, devices] = await Promise.all([
      api('/visualizer/api/pairings', undefined, { method: 'GET', signal: controller.signal }),
      api('/visualizer/api/devices', undefined, { method: 'GET', signal: controller.signal })
    ]);
    if (generation !== state.generation || inFlight.devices !== controller) return;
    if (pairings.status === 200 && devices.status === 200) {
      const changed = !state.devicesLoaded || JSON.stringify([pairings.data, devices.data]) !== JSON.stringify([state.pairings, state.devices]);
      Object.assign(state, { pairings: pairings.data || [], devices: devices.data || [], devicesLoaded: true });
      redraw(changed);
    }
  } catch { /* Agents are secondary; memories still work without them. */ }
  finally {
    // A failed request also cancels its sibling. Only the latest schedules.
    controller.abort();
    if (inFlight.devices === controller) {
      inFlight.devices = null;
      if (state.active && !document.hidden) timers.devices = setTimeout(refreshDevices, state.route.page === 'agents' ? POLL.devices : POLL.devicesIdle);
    }
  }
}

function stopPolling() {
  for (const name of ['page', 'index', 'devices']) { clearTimeout(timers[name]); timers[name] = null; inFlight[name]?.abort(); inFlight[name] = null; }
}
function restartPolling() {
  stopPolling();
  if (!state.active || document.hidden) return;
  refreshIndex(); refreshPage(); refreshDevices();
}
document.addEventListener('visibilitychange', () => { if (document.hidden) stopPolling(); else restartPolling(); });

// ---------- session ----------

async function checkSession() {
  try {
    const response = await fetch('/visualizer/api/session', { method: 'GET', credentials: 'same-origin', cache: 'no-store' });
    if (!response.ok) throw new Error('unavailable');
    const data = await response.json();
    state.checking = false;
    if (data.connected) { Object.assign(state, { active: true, generation: state.generation + 1 }); restartPolling(); if (state.route.memory) openMemory(state.route.memory, false); }
  } catch { state.checking = false; state.signInError = "Can't reach your server. Check it's running, then reload."; }
  render();
}

async function signIn(token) {
  if (state.signingIn) return;
  if (!token) { state.signInError = 'Paste your access token.'; render(); return; }
  state.signingIn = true; state.signInError = ''; render();
  try {
    const response = await fetch('/visualizer/api/session', { method: 'POST', headers: { Authorization: `Bearer ${token}` }, credentials: 'same-origin', cache: 'no-store' });
    if (!response.ok) throw new Error(response.status === 401 ? 'rejected' : response.status === 429 ? 'limited' : 'unavailable');
    Object.assign(state, { active: true, generation: state.generation + 1, signedOutNote: '' });
    restartPolling();
    const kept = state.keptDraft;
    state.keptDraft = null;
    if (kept) restoreDraft(kept);
    else if (state.route.memory) openMemory(state.route.memory, false);
  } catch (error) {
    state.signInError = error.message === 'rejected' ? 'That token was rejected. Check the file on your server and try again.'
      : error.message === 'limited' ? 'Too many attempts. Wait five minutes, then try again.' : "Can't reach your server. Try again when it's running.";
  } finally { state.signingIn = false; render(); }
}

// Reopens a memory with text that was being edited when the session ended.
// If an agent saved a newer revision meanwhile, both show as a conflict, so
// saving the draft never overwrites it silently.
async function restoreDraft(kept) {
  if (!kept.id) { startCreate(kept.project, { title: kept.title, content: kept.content }); return; }
  await openMemory(kept.id, false);
  const panel = state.panel;
  if (panel?.id === kept.id && panel.record) {
    const draft = { title: kept.title, content: kept.content };
    Object.assign(panel, panel.record.revision > kept.revision ? { mode: 'conflict', draft, theirs: panel.record } : { mode: 'edit', draft });
    render();
  }
}

async function signOut(everywhere) {
  if (dirty() && !confirmLeave()) return;
  try {
    const result = everywhere ? await api('/visualizer/api/session/revoke-all', {}) : await api('/visualizer/api/session', undefined, { method: 'DELETE' });
    if (result.status >= 300) throw new Error();
    stopPolling();
    Object.assign(state, { active: false, generation: state.generation + 1, index: null, projectOrder: null, page: null, panel: null, undo: null, signedOutNote: everywhere ? 'Signed out on every browser.' : 'Signed out.' });
    writes.clear();
  } catch (error) { if (!error.session) say("Couldn't sign out. Try again."); }
  render();
}

// ---------- memory panel ----------

async function openMemory(id, push = true) {
  if (dirty() && !confirmLeave()) return;
  const panel = { type: 'memory', id, key: newRequestID(), record: null, history: null, mode: 'view', error: '', draft: null, theirs: null };
  state.panel = panel;
  if (push) { state.route.memory = id; setURL(true); }
  render();
  await loadMemory(panel);
  document.getElementById('panel-title')?.focus?.();
}

async function loadMemory(panel, revision) {
  const generation = state.generation;
  try {
    const { status, data } = await api('/visualizer/api/record', { scope: { all_projects: true }, id: panel.id, ...(revision ? { revision } : {}) });
    if (state.panel !== panel || generation !== state.generation) return null;
    if (status !== 200) { if (!revision) { panel.error = status === 404 ? 'This memory no longer exists.' : "Couldn't load this memory."; render(); } return null; }
    if (!revision) { panel.record = data; panel.history = null; render(); loadHistory(panel); }
    return data;
  } catch (error) { if (!error.session && state.panel === panel && !revision) { panel.error = "Couldn't load this memory."; render(); } return null; }
}

// Earlier versions load newest first, five at a time, back to the first.
async function loadHistory(panel, more = false) {
  const record = panel.record;
  if (!record || record.revision <= 1) { panel.history = []; panel.olderFrom = 0; return; }
  const items = more ? [...(panel.history || [])] : [];
  const start = more ? panel.olderFrom : record.revision - 1;
  const stop = Math.max(1, start - 4);
  for (let revision = start; revision >= stop; revision--) {
    const older = await loadMemory(panel, revision);
    if (!older || state.panel !== panel || panel.record !== record) return;
    // An archive or restore revision repeats the text; show each text once.
    if (!items.some(item => item.content === older.content && item.title === older.title) && !(older.content === record.content && older.title === record.title)) items.push(older);
  }
  panel.history = items;
  panel.olderFrom = stop - 1;
  redraw(true);
}

function closePanel() {
  if (dirty() && !confirmLeave()) return;
  // Focus goes back to the row that opened the panel, so a keyboard user keeps their place.
  const opener = { memory: 'open:', device: 'device:' }[state.panel?.type];
  const id = state.panel?.id;
  state.panel = null;
  if (state.route.memory) { state.route.memory = null; setURL(true); }
  render();
  if (opener) focusByKey(opener + id);
}
// Escape closes an open panel. Fields and the edit form keep their own Escape.
document.addEventListener('keydown', event => {
  if (event.key !== 'Escape' || event.defaultPrevented || !state.panel || editing() || /^(INPUT|TEXTAREA|SELECT)$/.test(event.target?.tagName || '')) return;
  event.preventDefault();
  closePanel();
});

// Typing in a memory or a device name, or choosing where to move one; routine refreshes wait so text, caret and choice stay put.
function editing() { return state.panel?.mode === 'edit' || state.panel?.mode === 'move' || document.activeElement?.id === 'device-name'; }
// The text being edited, if it differs from the saved memory.
function draftOf(panel) {
  if (!panel?.record || (panel.mode !== 'edit' && panel.mode !== 'conflict')) return null;
  const title = document.getElementById('edit-title')?.value ?? panel.draft?.title ?? panel.record.title;
  const content = document.getElementById('edit-text')?.value ?? panel.draft?.content ?? panel.record.content;
  return title !== panel.record.title || content !== panel.record.content ? { id: panel.record.id, revision: panel.record.revision, title, content, project: panel.project } : null;
}
function dirty() {
  const panel = state.panel;
  if (panel?.mode === 'conflict') return Boolean(draftOf(panel));
  if (!panel || panel.mode !== 'edit' || !panel.record) return false;
  const title = document.getElementById('edit-title')?.value ?? panel.draft?.title ?? panel.record.title;
  const text = document.getElementById('edit-text')?.value ?? panel.draft?.content ?? panel.record.content;
  return title !== panel.record.title || text !== panel.record.content;
}
function confirmLeave() { return window.confirm('Discard your unsaved changes?'); }
// Background refreshes draw only when nothing is being typed or selected. A
// change held back meanwhile stays pending, so the next refresh draws it even
// when that refresh itself brings nothing new.
function redraw(changed) {
  if (!changed && !state.redrawPending) return;
  state.redrawPending = editing() || selecting();
  if (!state.redrawPending) render();
}
function selecting() { const selection = window.getSelection?.(); return Boolean(selection && !selection.isCollapsed); }

// One owner write at a time. A 409 shows the newer version; nothing is overwritten.
async function write(path, body, { onDone, onConflict } = {}) {
  if (saving) { say('Another change is still saving.'); return false; }
  const { key, id } = requestIDFor(path, body);
  saving = true; render();
  const generation = state.generation;
  try {
    const { status, data } = await api(path, { ...body, request_id: id }, { timeout: 20000 });
    if (generation !== state.generation) return false;
    if (status === 200) { writes.delete(key); await onDone?.(data); refreshAfterWrite(); return true; }
    // Only a definite answer retires the request ID; a retry after 5xx reuses it.
    if ([400, 404, 409].includes(status)) writes.delete(key);
    if (status === 409 && data?.error === 'duplicate_conflict') { say('Not saved. Another memory already has this text.'); return false; }
    if (status === 409) {
      if (onConflict) onConflict(data?.current);
      else {
        say('This memory changed. Showing the latest.');
        if (state.panel?.type === 'memory' && state.panel.id === body.id && data?.current) { state.panel.record = data.current; state.panel.history = null; loadHistory(state.panel); }
      }
      refreshAfterWrite();
      return false;
    }
    say(status === 400 ? 'That change is not valid. Check the text and try again.' : "Couldn't save. Try again; it won't save twice.");
    return false;
  } catch (error) {
    if (!error.session) say("Couldn't save. Try again; it won't save twice.");
    return false;
  } finally { saving = false; render(); }
}
function refreshAfterWrite() { state.etags = {}; refreshIndex(); refreshPage(); }

function keep(record) {
  return write('/visualizer/api/update', { id: record.id, expected_revision: record.revision, action: 'confirm' }, {
    onDone: () => { say(`Kept “${record.title}”.`); advanceReview(record.id); }
  });
}
function discard(record) {
  return write('/visualizer/api/archive', { id: record.id, expected_revision: record.revision, archived: true }, {
    onDone: receipt => { offerUndo(record, receipt.revision, 'Discarded'); advanceReview(record.id); }
  });
}
async function keepAndReplace(record, old) {
  if (await keep(record)) await write('/visualizer/api/archive', { id: old.id, expected_revision: old.revision, archived: true }, { onDone: () => say(`Kept “${record.title}” and archived “${old.title}”.`) });
}
// After deciding on a suggestion in its panel, open the next one on New.
function advanceReview(id) {
  if (state.panel?.id !== id) return;
  const next = state.route.page === 'new' && (state.page?.records || []).find(record => record.id !== id);
  if (next) openMemory(next.id); else { state.panel = null; state.route.memory = null; setURL(false); }
}
function archive(record) {
  return write('/visualizer/api/archive', { id: record.id, expected_revision: record.revision, archived: true }, {
    onDone: receipt => { offerUndo(record, receipt.revision, 'Archived'); state.panel = null; state.route.memory = null; setURL(false); }
  });
}
function restore(record) {
  return write('/visualizer/api/archive', { id: record.id, expected_revision: record.revision, archived: false }, {
    onDone: () => { say(`Restored “${record.title}”.`); if (state.panel?.id === record.id) loadMemory(state.panel); }
  });
}
function restoreRevision(record, older) {
  return write('/visualizer/api/update', { id: record.id, expected_revision: record.revision, restore_revision: older.revision }, {
    onDone: () => { say('Restored that version.'); if (state.panel) loadMemory(state.panel); }
  });
}
function saveEdit(panel) {
  const record = panel.record;
  const title = (document.getElementById('edit-title')?.value ?? panel.draft?.title ?? record.title).trim();
  const content = document.getElementById('edit-text')?.value ?? panel.draft?.content ?? record.content;
  if (saving) { say('Another change is still saving.'); return false; }
  panel.draft = { title, content };
  const bytes = text => new TextEncoder().encode(text).length;
  const invalid = !content.trim() ? ['edit-text', 'Write some text before saving.']
    : bytes(title) > 512 ? ['edit-title', 'Keep the title under 512 bytes.']
    : bytes(content) > 32768 ? ['edit-text', 'Keep the text under 32,768 bytes.'] : null;
  if (invalid) {
    [panel.errorField, panel.error] = invalid;
    render();
    document.getElementById(panel.errorField)?.focus?.();
    return false;
  }
  panel.error = ''; panel.errorField = '';
  if (!record.id) {
    const project = document.getElementById('edit-project')?.value ?? panel.project;
    return write('/visualizer/api/create', { project: project || null, title, content }, {
      onDone: receipt => { say('Saved.'); if (state.panel === panel) { state.panel = null; openMemory(receipt.id); } }
    });
  }
  const expected = panel.theirs ? panel.theirs.revision : record.revision;
  return write('/visualizer/api/update', { id: record.id, expected_revision: expected, title, content, purpose: record.purpose }, {
    // If another memory was opened while this saved, leave that panel and its focus alone.
    onDone: () => { Object.assign(panel, { mode: 'view', draft: null, theirs: null }); say('Saved.'); if (state.panel !== panel) return; render(); document.getElementById('panel-title')?.focus?.(); loadMemory(panel); },
    onConflict: current => { if (current) Object.assign(panel, { mode: 'conflict', theirs: current }); }
  });
}
function keepTheirs(panel) { Object.assign(panel, { mode: 'view', draft: null, theirs: null }); loadMemory(panel); }
// Moves to a full place: project, device and platform, where empty means every one.
function move(record, to) {
  const scope = { project: to.project || null, device: to.device || null, platform: to.platform || null };
  return write('/visualizer/api/move', { id: record.id, expected_revision: record.revision, ...scope }, {
    onDone: receipt => { say(`Moved to ${whereLabel(scope)}.`); openMemory(receipt.id); },
    onConflict: () => { say(`Not moved. ${whereLabel(scope)} may already have it, or it changed. Showing the latest.`); if (state.panel) { state.panel.mode = 'view'; loadMemory(state.panel); } }
  });
}

function offerUndo(record, revision, verb) {
  state.undo = { record, revision, verb };
  armUndo();
}
// Undo stays for ten seconds, counted only while it has neither keyboard
// focus nor the pointer, so nobody loses it while reaching for it.
function armUndo() {
  clearTimeout(timers.undo);
  // While an Undo request is in flight the bar never expires.
  if (state.undoBusy) return;
  const pending = state.undo;
  if (pending) timers.undo = setTimeout(() => { if (state.undo === pending) { state.undo = null; render(); } }, 10000);
}
function holdUndo() { clearTimeout(timers.undo); timers.undo = null; }
// True while the Undo bar has keyboard focus or the pointer.
function undoHeld() { const bar = root.querySelector?.('.undo'); return Boolean(bar && (bar.contains?.(document.activeElement) || bar.matches?.(':hover'))); }
async function undo() {
  const pending = state.undo;
  if (!pending) return;
  if (saving) { say('Another change is still saving. Try Undo again in a moment.'); return; }
  clearTimeout(timers.undo);
  state.undoBusy = true;
  let done;
  try {
    done = await write('/visualizer/api/archive', { id: pending.record.id, expected_revision: pending.revision, archived: false }, {
      onDone: () => { say(`Restored “${pending.record.title}”.`); if (state.undo === pending) state.undo = null; }
    });
  } finally { state.undoBusy = false; }
  // On failure the Undo stays for another ten seconds so it can be retried.
  if (!done && state.undo === pending && !undoHeld()) armUndo();
}

async function decidePairing(request, decision) {
  try {
    const { status } = await api('/visualizer/api/pairings', { request_id: request.request_id, code: request.code, decision });
    say(status >= 300 ? "Couldn't change this request. Try again." : decision === 'approve' ? `${request.device} connected.` : `${request.device} denied.`);
  } catch (error) { if (!error.session) say("Couldn't change this request. Try again."); }
  refreshDevices();
}
async function renameDevice(device, label, panel) {
  if (saving) return;
  saving = true; render();
  try {
    const { status, data } = await api('/visualizer/api/devices', { device: device.device, label: label.trim() }, { method: 'PATCH' });
    say(status >= 300 ? (status === 400 && data?.error ? `Not renamed. ${data.error.replace(/^invalid device name: /, '').replace(/^./, c => c.toUpperCase())}.` : "Couldn't rename it. Try again.") : label.trim() ? `Named “${label.trim()}”.` : `Showing ${device.device}.`);
    if (status < 300) {
      // Show the saved name at once, on every token of this device, so the form never offers the old one.
      for (const item of state.devices) if (item.device === device.device) item.label = label.trim() || null;
      // Clear only the draft that was saved, never one typed since or for another device.
      if (panel.nameDraft === label) panel.nameDraft = null;
    }
  } catch (error) { if (!error.session) say("Couldn't rename it. Try again."); }
  finally { saving = false; }
  refreshDevices();
  render();
}
async function disconnectDevice(device) {
  try {
    const { status } = await api('/visualizer/api/devices', { id: device.id }, { method: 'DELETE' });
    say(status >= 300 ? "Couldn't disconnect it. Try again." : `${device.label || device.device} disconnected.`);
    if (status < 300) state.panel = null;
  } catch (error) { if (!error.session) say("Couldn't disconnect it. Try again."); }
  refreshDevices();
  render();
}

async function openStartup(project = state.startupProject) {
  if (dirty() && !confirmLeave()) return;
  const panel = { type: 'startup', agent: state.startupAgent, project, data: null, error: '' };
  state.panel = panel; render();
  try {
    const { status, data } = await api('/visualizer/api/startup', { scope: panel.project ? { project: panel.project } : {}, budget: BUDGETS[panel.agent] });
    if (state.panel !== panel) return;
    if (status !== 200) throw new Error();
    panel.data = data;
  } catch (error) { if (!error.session && state.panel === panel) panel.error = "Couldn't load the preview."; }
  render();
}

async function exportAll() {
  try {
    const { status, data } = await api('/visualizer/api/export', { scope: { all_projects: true }, all: true, include_archived: state.exportArchived }, { timeout: 30000 });
    if (status !== 200 || !Array.isArray(data?.records) || data.error) throw new Error();
    const url = URL.createObjectURL(new Blob([JSON.stringify(data, null, 2)], { type: 'application/json' }));
    h('a', { href: url, download: 'grasshopper-memories.json' }).click();
    setTimeout(() => URL.revokeObjectURL(url), 1000);
    state.exportStatus = 'Downloaded.';
  } catch (error) { if (!error.session) state.exportStatus = "Couldn't export. Try again."; }
  render();
}

async function copyText(text, button) {
  try { await navigator.clipboard.writeText(text); button.textContent = 'Copied'; say('Copied.'); } catch { button.textContent = 'Select and copy'; say("Couldn't copy. Select the text and copy it."); }
  setTimeout(() => { button.textContent = 'Copy'; }, 1500);
}

// ---------- views ----------

function currentPlace() {
  const route = state.route;
  return route.page === 'project' ? projectName(route.project) : { all: 'All projects', new: 'New', agents: 'Agents', archived: 'Archived', settings: 'Settings', search: 'Search' }[route.page] || 'Grasshopper';
}

function navButton(label, route, { current = false, count = '', extra = '' } = {}) {
  return h('a', { href: routeLink(route), class: `nav ${extra}`.trim(), 'data-key': `nav:${routeHash(route)}`, 'aria-current': current ? 'page' : null, onclick: inPlace(() => go(route)) },
    h('span', { text: label }), count ? h('span', { class: 'count', text: count }) : null);
}

function sidebar() {
  const route = state.route;
  const projects = sortedProjects();
  // The five most recent, plus the open project; the rest by name, to scan.
  const open = route.page === 'project' ? route.project : '';
  const top = projects.filter((project, i) => i < 5 || project === open);
  // The open project stays even after its last memory leaves the index.
  if (open && !top.includes(open)) top.push(open);
  const rest = byName(projects.filter(project => !top.includes(project)));
  const shown = state.moreProjects ? [...top, ...rest] : top;
  const review = state.index?.review || 0;
  const devices = state.devices.filter(device => !device.revoked_at);
  const waiting = state.pairings.filter(request => request.status === 'pending').length;
  const active = devices.filter(device => recentlySeen(device)).length;
  return h('nav', { class: 'side', 'aria-label': 'Memory view' },
    h('p', { class: 'brand', text: 'GRASSHOPPER' }),
    review ? h('div', { class: 'nav-group' }, navButton('New', { page: 'new' }, { current: route.page === 'new', count: String(review), extra: 'new' })) : null,
    h('div', { class: 'nav-group' },
      navButton('All projects', { page: 'all' }, { current: route.page === 'all', extra: 'all' }),
      shown.map(project => navButton(projectName(project), { page: 'project', project }, { current: route.page === 'project' && route.project === project })),
      rest.length ? h('button', { type: 'button', class: 'nav', 'data-key': 'nav:more', onclick: () => { state.moreProjects = !state.moreProjects; render(); } }, h('span', { text: state.moreProjects ? 'Fewer' : `${rest.length} more` })) : null),
    h('div', { class: 'nav-group side-foot' },
      h('a', { href: routeLink({ page: 'agents' }), class: 'nav', 'data-key': 'nav:#page=agents', 'aria-current': route.page === 'agents' ? 'page' : null, onclick: inPlace(() => go({ page: 'agents' })) },
        h('span', {}, devices.length ? h('span', { class: 'dots', 'aria-hidden': 'true' }, devices.slice(0, 6).map(device => h('i', { class: recentlySeen(device) ? 'dot on' : 'dot' }))) : null, 'Agents',
          devices.length ? h('span', { class: 'visually-hidden', text: `, ${active} of ${devices.length} active` }) : null),
        waiting ? h('span', { class: 'count warn', text: `${waiting} waiting` }) : null),
      navButton('Archived', { page: 'archived' }, { current: route.page === 'archived' }),
      navButton('Settings', { page: 'settings' }, { current: route.page === 'settings' })));
}

function row(record, { meta } = {}) {
  const right = [record.purpose === 'handoff' && 'handoff', meta ?? [by(record), relDate(record.updated_at), onlyLabel(record.scope)].filter(Boolean).join(' · ')].filter(Boolean).join(' · ');
  return h('a', { href: routeLink({ ...state.route, memory: record.id }), class: 'row', 'data-key': `open:${record.id}`, 'aria-current': state.panel?.id === record.id ? 'true' : null,
    title: fullDate(record.updated_at), onclick: inPlace(() => openMemory(record.id)) },
  h('span', { class: 't', text: record.title || `Memory #${record.id}` }), h('span', { class: 'm', text: right }));
}

function section(label, children, cls = '') {
  return h('section', { class: 'sec' }, h('h2', { class: `sec-label ${cls}`.trim(), text: label }), h('div', { class: 'list' }, children));
}

function suggestionRow(record) {
  // Compare only the opened suggestion in panelBody, not every pair on redraw.
  const meta = [by(record), relDate(record.updated_at)].filter(Boolean).join(' · ');
  return h('div', { class: 'row-inline' }, row(record, { meta }),
    h('span', { class: 'actions tight' },
      h('button', { type: 'button', class: 'quiet', 'aria-disabled': saving, 'data-key': `keep:${record.id}`, 'aria-label': `Keep ${record.title}`, onclick: () => keep(record) }, 'Keep'),
      h('button', { type: 'button', class: 'quiet muted', 'aria-disabled': saving, 'data-key': `discard:${record.id}`, 'aria-label': `Discard ${record.title}`, onclick: () => discard(record) }, 'Discard')));
}

// Each group's label still names where its memories apply.
function grouped(records, place) {
  if (records.length <= 8) return null;
  return recencyGroups(records).map(group => {
    const limit = group.label === 'This week' ? Infinity : 3;
    const visible = state.expanded.has(group.label) ? group.records : group.records.slice(0, limit);
    const rest = group.records.length - visible.length;
    return section(`${place} · ${group.label}`, [visible.map(record => row(record)),
      rest > 0 ? h('button', { type: 'button', class: 'quiet more', onclick: () => { state.expanded.add(group.label); render(); } }, `Show ${rest} more`) : null]);
  });
}

function head(title, sub, { search = true } = {}) {
  return h('div', { class: 'head' },
    h('div', {}, h('h1', { id: 'page-title', text: title }), sub ? h('p', { class: 'meta sub' }, sub) : null),
    search ? h('form', { class: 'search', role: 'search', onsubmit: event => {
      event.preventDefault();
      const query = event.target.querySelector('input').value.trim();
      if (query) go({ page: 'search', query, project: state.route.page === 'project' ? state.route.project : '' });
    } }, h('input', { type: 'search', 'aria-label': 'Search memories', placeholder: 'Search…', value: state.searchDraft ?? (state.route.page === 'search' ? state.route.query : ''),
      oninput: event => { state.searchDraft = event.target.value; } })) : null);
}

function banner() {
  if (!state.offline) return null;
  return h('div', { class: 'banner', role: 'status' },
    h('span', { text: `Can't reach your server. Showing what loaded at ${state.offline.toLocaleTimeString(undefined, { hour: 'numeric', minute: '2-digit' })}.` }),
    h('button', { type: 'button', class: 'quiet', onclick: () => { state.offline = null; restartPolling(); } }, 'Retry'));
}

function loadingOr(content) {
  if (!state.page) return [banner(), h('p', { class: 'meta loading', text: 'Loading…' })];
  return content();
}

function firstRun() {
  return h('div', { class: 'done' }, h('h1', { id: 'page-title', text: 'Nothing saved yet' }),
    h('p', { class: 'reading muted', text: 'Connect an agent, then tell it something worth remembering.' }),
    h('div', { class: 'actions' }, h('button', { type: 'button', class: 'btn', onclick: () => go({ page: 'agents' }) }, 'Connect an agent'),
      h('button', { type: 'button', class: 'quiet', onclick: () => startCreate('') }, 'Add memory')));
}

function projectPage() {
  const project = state.route.project;
  return loadingOr(() => {
    const records = state.page.records;
    const fresh = records.filter(record => !record.confirmed && record.purpose !== 'handoff');
    const kept = records.filter(record => record.confirmed && record.purpose !== 'handoff');
    const own = kept.filter(record => record.scope?.project === project);
    const global = kept.filter(record => !record.scope?.project);
    const handoff = records.filter(record => record.purpose === 'handoff' && record.scope?.project === project).sort((a, b) => String(b.updated_at).localeCompare(String(a.updated_at)))[0];
    // Agents load this project's and every project's kept memories, so count and show both.
    const applying = own.length + global.length;
    return [head(projectName(project), [`${applying} ${applying === 1 ? 'memory applies' : 'memories apply'} here`, durable(project) ? h('button', { type: 'button', class: 'quiet', onclick: () => startCreate(project) }, 'Add memory') : null, h('button', { type: 'button', class: 'quiet', onclick: () => openStartup(project) }, 'Preview startup')]), banner(),
      handoff ? section(shortDate(handoff.updated_at), row(handoff, { meta: by(handoff) })) : null,
      fresh.length ? section('New', fresh.map(record => suggestionRow(record)), 'warn') : null,
      // An empty project section only matters when nothing else applies here.
      own.length || !global.length ? grouped(own, projectName(project)) || section(projectName(project), own.length ? own.map(record => row(record)) : h('p', { class: 'meta empty', text: 'Nothing kept for this project yet.' })) : null,
      global.length ? section('All projects', global.length <= 8 || !own.length ? global.map(record => row(record)) : state.showGlobal
        ? [global.map(record => row(record)), h('button', { type: 'button', class: 'quiet more muted', onclick: () => { state.showGlobal = false; render(); } }, 'Hide')]
        : h('button', { type: 'button', class: 'quiet more', onclick: () => { state.showGlobal = true; render(); } }, `Show ${global.length} that also apply here`)) : null];
  });
}

function allPage() {
  return loadingOr(() => {
    const records = state.page.records.filter(record => !record.scope?.project);
    if (state.index && !state.index.total && !records.length) return firstRun();
    const fresh = records.filter(record => !record.confirmed && record.purpose !== 'handoff');
    const kept = records.filter(record => record.confirmed && record.purpose !== 'handoff');
    const handoff = records.filter(record => record.purpose === 'handoff').sort((a, b) => String(b.updated_at).localeCompare(String(a.updated_at)))[0];
    return [head('All projects', ['Memories that apply to every project', h('button', { type: 'button', class: 'quiet', onclick: () => startCreate('') }, 'Add memory')]), banner(),
      handoff ? section(shortDate(handoff.updated_at), row(handoff, { meta: by(handoff) })) : null,
      fresh.length ? section('New', fresh.map(record => suggestionRow(record)), 'warn') : null,
      grouped(kept, 'All projects') || section('All projects', kept.length ? kept.map(record => row(record)) : h('p', { class: 'meta empty', text: 'Nothing applies to every project yet.' }))];
  });
}

async function keepAll() {
  for (const record of [...(state.page?.records || [])]) if (!(await keep(record))) break;
}
function newPage() {
  return loadingOr(() => {
    const records = state.page.records;
    if (!records.length) return h('div', { class: 'done' }, h('h1', { id: 'page-title', text: "You're caught up" }),
      h('p', { class: 'reading muted', text: 'New things your agents notice show up here.' }));
    return [h('div', { class: 'head' },
      h('div', {}, h('h1', { id: 'page-title', text: 'New' }), h('p', { class: 'meta sub', text: `${records.length} waiting · agents load them at startup only after you keep them` })),
      h('button', { type: 'button', class: 'quiet', 'aria-disabled': saving, onclick: keepAll }, 'Keep all')), banner(),
    // By name, not recency: keeping one must not move the rest.
    places(records, [null]).map(project => {
      const items = records.filter(record => (record.scope?.project || null) === project);
      return items.length ? section(projectName(project), items.map(record => suggestionRow(record))) : null;
    })];
  });
}

function searchPage() {
  const records = state.page?.records || [];
  const order = places(records, [state.route.project || null, null, ...sortedProjects()]);
  const body = !state.page ? h('p', { class: 'meta loading', text: 'Searching…' })
    : !records.length ? h('p', { class: 'meta empty', text: `Nothing matches “${state.route.query}”. Try fewer words.` })
    : order.map(project => { const items = records.filter(record => (record.scope?.project || null) === project); return items.length ? section(projectName(project), items.map(record => row(record))) : null; });
  const omitted = state.page?.omitted_records || [];
  const titles = state.page?.omitted_titles || {};
  const extra = omitted.length ? section('More', [h('p', { class: 'meta', text: `${omitted.length} more ${omitted.length === 1 ? 'match' : 'matches'} did not fit in one response.` }),
    omitted.map(reference => h('a', { href: routeLink({ ...state.route, memory: reference.id }), class: 'row', 'data-key': `open:${reference.id}`, onclick: inPlace(() => openMemory(reference.id)) },
      h('span', { class: 't', text: titles[reference.id] || `Memory #${reference.id}` }), h('span', { class: 'm', text: projectName(reference.scope?.project) })))]) : null;
  const found = records.length + omitted.length;
  return [head('Search', state.page ? `${found} found${state.page.semantic_ready === false ? ' · exact words only' : ''}` : ''), banner(), body, extra];
}

function archivedPage() {
  return loadingOr(() => {
    const all = state.page.records;
    if (!all.length) return [head('Archived', '', { search: false }), h('p', { class: 'meta empty', text: 'Nothing archived.' })];
    const label = record => {
      const source = record.provenance?.source || '';
      return `${source.startsWith('Replaced by handoff') ? 'replaced' : source.startsWith('Moved to memory') ? 'moved' : 'archived'} ${relDate(record.updated_at)}`;
    };
    // Replaced handoffs are history, not things the owner archived; fold them all behind one line.
    const replaced = all.filter(record => (record.provenance?.source || '').startsWith('Replaced by handoff'));
    const showReplaced = state.expanded.has('replaced');
    const records = showReplaced ? all : all.filter(record => !replaced.includes(record));
    return [head('Archived', "Agents don't load these at startup.", { search: false }), banner(),
      places(records, [null, ...sortedProjects()]).map(project => {
        const items = records.filter(record => (record.scope?.project || null) === project);
        return items.length ? section(projectName(project), items.map(record => h('div', { class: 'row-inline' }, row(record, { meta: label(record) }),
          h('button', { type: 'button', class: 'quiet', 'aria-disabled': saving, 'data-key': `restore:${record.id}`, 'aria-label': `Restore ${record.title}`, onclick: () => restore(record) }, 'Restore')))) : null;
      }),
      replaced.length && !showReplaced ? section('Earlier handoffs', h('button', { type: 'button', class: 'quiet more', 'data-key': 'show-replaced', onclick: () => { state.expanded.add('replaced'); render(); } }, `Show ${replaced.length} replaced by newer ones`)) : null];
  });
}

function agentsPage() {
  const waiting = state.pairings.filter(request => request.status === 'pending');
  const connected = state.devices.filter(device => !device.revoked_at);
  const linked = state.route.connect;
  return [head('Agents', '', { search: false }),
    waiting.length ? section('Waiting', waiting.map(request => h('div', { class: `pairing${request.request_id === linked ? ' focused' : ''}`, tabindex: request.request_id === linked ? '-1' : null, id: request.request_id === linked ? 'focused-request' : null },
      h('div', { class: 'row-inline' }, h('span', { class: 'reading', text: `${request.device} wants to connect` }), h('code', { class: 'code', text: request.code })),
      h('p', { class: 'label muted', text: `Approve only if the agent shows the same code. ${request.expires_in >= 60 ? `Expires in ${Math.ceil(request.expires_in / 60)} min.` : `Expires in ${request.expires_in} s.`}` }),
      h('div', { class: 'actions' },
        h('button', { type: 'button', class: 'btn', 'data-key': `approve:${request.request_id}`, 'aria-label': `Approve ${request.device}`, onclick: () => decidePairing(request, 'approve') }, 'Approve'),
        h('button', { type: 'button', class: 'quiet muted', 'data-key': `deny:${request.request_id}`, 'aria-label': `Deny ${request.device}`, onclick: () => decidePairing(request, 'deny') }, 'Deny')))), 'warn') : null,
    linked && state.devicesLoaded && !waiting.some(request => request.request_id === linked)
      ? h('p', { class: 'label muted', text: 'That connection request has expired or was already handled. Ask the agent to connect again.' }) : null,
    section('Connected', connected.length ? connected.map(device => h('button', { type: 'button', class: 'row', 'data-key': `device:${device.id}`, 'aria-current': state.panel?.type === 'device' && state.panel.id === device.id ? 'true' : null, onclick: () => { state.panel = { type: 'device', id: device.id, confirm: false }; render(); } },
      h('span', { class: 't' }, h('i', { class: recentlySeen(device) ? 'dot on' : 'dot', 'aria-hidden': 'true' }), device.label || device.device),
      h('span', { class: 'm', text: ago(device.last_seen_at) }))) : h('p', { class: 'meta empty', text: 'No agents connected yet.' })),
    section('Add one', h('div', { class: 'stack' },
      h('p', { class: 'label', text: 'In a new agent session, say:' }),
      h('div', { class: 'strip' }, h('code', { text: 'Connect Grasshopper' }), h('button', { type: 'button', class: 'btn', 'aria-label': 'Copy Connect Grasshopper', onclick: event => copyText('Connect Grasshopper', event.currentTarget) }, 'Copy')),
      h('a', { class: 'label', href: 'https://usegrasshopper.com/setup/', target: '_blank', rel: 'noopener' }, 'Setup guide')))];
}

function settingsPage() {
  const server = state.index?.server;
  const theme = window.grasshopperTheme?.saved?.() || 'system';
  return [head('Settings', '', { search: false }),
    section('Startup', h('div', { class: 'stack' },
      h('p', { class: 'label', text: 'What an agent gets when a session starts.' }),
      h('div', { class: 'two' },
        h('label', { class: 'field' }, 'Agent', h('select', { onchange: event => { state.startupAgent = event.target.value; } }, Object.keys(BUDGETS).map(name => h('option', { value: name, selected: state.startupAgent === name ? true : null }, name)))),
        h('label', { class: 'field' }, 'Project', h('select', { onchange: event => { state.startupProject = event.target.value; } },
          h('option', { value: '' }, 'No project'), sortedProjects().map(project => h('option', { value: project, selected: state.startupProject === project ? true : null }, projectName(project)))))),
      h('div', {}, h('button', { type: 'button', class: 'btn', onclick: () => openStartup() }, 'Preview')))),
    section('Export', h('div', { class: 'stack' },
      h('p', { class: 'label', text: 'Every memory as a JSON file.' }),
      h('label', { class: 'check' }, h('input', { type: 'checkbox', checked: state.exportArchived, onchange: event => { state.exportArchived = event.target.checked; } }), 'Include archived'),
      h('div', { class: 'actions' }, h('button', { type: 'button', class: 'btn', onclick: exportAll }, 'Download'), state.exportStatus ? h('span', { class: 'meta', text: state.exportStatus }) : null))),
    section('Appearance', h('div', { class: 'seg', role: 'group', 'aria-label': 'Appearance' },
      ['system', 'light', 'dark'].map(choice => h('button', { type: 'button', 'aria-pressed': String(theme === choice), onclick: () => { window.grasshopperTheme?.set(choice); render(); } }, choice[0].toUpperCase() + choice.slice(1))))),
    section('Sign-in', h('div', { class: 'stack' },
      h('p', { class: 'label', text: 'This browser stays signed in for 7 days.' }),
      h('div', { class: 'actions' },
        h('button', { type: 'button', class: 'quiet', onclick: () => signOut(false) }, 'Sign out'),
        h('button', { type: 'button', class: 'quiet danger', onclick: () => signOut(true) }, 'Sign out everywhere')))),
    server ? section('Server', h('p', { class: 'val' }, `Grasshopper ${server.version}`,
      h('span', { class: 'meta', text: `${server.memories} ${server.memories === 1 ? 'memory' : 'memories'} · ${server.archived} archived · ${server.model ? 'meaning search on' : 'exact-word search only'}` }))) : null];
}

function invalidProps(panel, id) {
  const invalid = Boolean(panel.error) && panel.errorField === id;
  return { 'aria-invalid': invalid ? 'true' : 'false', 'aria-describedby': invalid ? 'edit-error' : null };
}

function editForm(panel) {
  const record = panel.record;
  const draft = panel.draft || { title: record.title, content: record.content };
  const out = record.id ? [] : [h('h2', { class: 'panel-title', text: 'Add memory' })];
  if (panel.mode === 'conflict') out.push(h('div', { class: 'conflict', role: 'alert' },
    h('p', { class: 'label warn', text: `${by(panel.theirs) === 'you' ? 'Someone' : by(panel.theirs)} changed this while you were editing. Nothing was overwritten.` }),
    h('div', {}, h('p', { class: 'who', text: `${by(panel.theirs)} · ${relDate(panel.theirs.updated_at)}` }), h('p', { class: 'reading', text: panel.theirs.content })),
    h('div', {}, h('p', { class: 'who', text: 'Yours' }), h('p', { class: 'reading', text: draft.content })),
    h('div', { class: 'actions' },
      h('button', { type: 'button', class: 'btn', 'aria-disabled': saving, onclick: () => saveEdit(panel) }, 'Keep yours'),
      h('button', { type: 'button', class: 'quiet', onclick: () => keepTheirs(panel) }, 'Keep theirs'))));
  const cancel = () => { if (!dirty() || confirmLeave()) { if (record.id) Object.assign(panel, { mode: 'view', error: '', errorField: '', draft: null }); else state.panel = null; render(); } };
  out.push(h('form', { class: 'edit', hidden: panel.mode === 'conflict', onsubmit: event => { event.preventDefault(); saveEdit(panel); },
    // Cmd/Ctrl+Enter saves and Escape cancels, as in most editors.
    onkeydown: event => {
      if (event.key === 'Enter' && (event.metaKey || event.ctrlKey)) { event.preventDefault(); saveEdit(panel); }
      else if (event.key === 'Escape') { event.preventDefault(); cancel(); }
    } },
    h('label', { class: 'field' }, 'Title', h('input', { id: 'edit-title', type: 'text', value: draft.title, 'data-panel': panel.key, ...invalidProps(panel, 'edit-title') })),
    h('label', { class: 'field' }, 'Text', h('textarea', { id: 'edit-text', rows: '6', value: draft.content, 'data-panel': panel.key, ...invalidProps(panel, 'edit-text') })),
    record.id ? null : h('label', { class: 'field' }, 'Applies to', placeSelect('edit-project', panel.project, event => { panel.project = event.target.value; })),
    panel.error ? h('p', { class: 'error', id: 'edit-error', role: 'alert', text: panel.error }) : null,
    h('div', { class: 'actions' },
      h('button', { type: 'submit', class: 'btn', 'data-key': 'save', 'aria-disabled': saving }, saving ? 'Saving…' : 'Save'),
      h('button', { type: 'button', class: 'quiet muted', onclick: cancel }, 'Cancel')),
    record.id ? h('p', { class: 'meta', text: 'Your edit becomes the current version. The old one stays in History.' }) : null));
  return out;
}

// A new memory is an empty one in edit mode, so drafts, redraws and leaving work as for an edit.
function startCreate(project, draft = null) {
  if (dirty() && !confirmLeave()) return;
  if (state.route.memory) { state.route.memory = null; setURL(true); }
  state.panel = { type: 'memory', id: null, key: newRequestID(), record: { title: '', content: '', scope: {} }, mode: 'edit', project: durable(project) ? project : '', error: '', draft };
  render();
  document.getElementById('edit-title')?.focus?.();
}
function startEdit(panel) { panel.mode = 'edit'; render(); document.getElementById('edit-text')?.focus?.(); }

function memoryPanel(panel) {
  const record = panel.record;
  if (!record) return h('p', { class: panel.error ? 'label' : 'meta loading', text: panel.error || 'Loading…' });
  if (panel.mode === 'edit' || panel.mode === 'conflict') return editForm(panel);
  if (panel.mode === 'move') {
    const now = { project: record.scope?.project || '', device: record.scope?.device || '', platform: record.scope?.platform || '' };
    const back = () => { Object.assign(panel, { mode: 'view', error: '', moveTo: undefined }); render(); };
    // Choices live on the panel, so a redraw while saving or a retry keeps them.
    const chosen = (key, id) => panel.moveTo && key in panel.moveTo ? panel.moveTo[key] : document.getElementById(id)?.value ?? now[key];
    const pick = key => event => { panel.moveTo = { ...panel.moveTo, [key]: event.target.value }; };
    const devices = [...new Set(state.devices.filter(device => !device.revoked_at).map(device => device.device))];
    if (now.device && !devices.includes(now.device)) devices.push(now.device);
    return [h('h2', { class: 'panel-title', text: record.title }),
      h('form', { class: 'edit', onsubmit: event => {
        event.preventDefault();
        const to = { project: chosen('project', 'move-to'), device: chosen('device', 'move-device'), platform: chosen('platform', 'move-platform') };
        if (to.project === now.project && to.device === now.device && to.platform === now.platform) { panel.error = 'Pick another place.'; render(); return; }
        move(record, to);
      }, onkeydown: event => { if (event.key === 'Escape') { event.preventDefault(); back(); } } },
      h('label', { class: 'field' }, 'Project', placeSelect('move-to', panel.moveTo?.project ?? now.project, pick('project'))),
      h('div', { class: 'two' },
        h('label', { class: 'field' }, 'Device', optionSelect('move-device', [['', 'Every device'], ...devices.map(device => [device, deviceName(device)])], panel.moveTo?.device ?? now.device, pick('device'))),
        h('label', { class: 'field' }, 'OS', optionSelect('move-platform', [['', 'Every OS'], ...Object.entries(PLATFORM)], panel.moveTo?.platform ?? now.platform, pick('platform')))),
      panel.error ? h('p', { class: 'error', role: 'alert', text: panel.error }) : null,
      h('div', { class: 'actions' },
        h('button', { type: 'submit', class: 'btn', 'aria-disabled': saving }, 'Move'),
        h('button', { type: 'button', class: 'quiet muted', onclick: back }, 'Cancel')))];
  }
  const isNew = !record.confirmed && record.purpose !== 'handoff' && !record.archived;
  const similar = isNew ? similarTo(record, [...(state.index?.records || []), ...(state.page?.records || [])]) : null;
  // Moving changes only the project; a device or platform restriction stays.
  const actions = record.archived
    ? [h('button', { type: 'button', class: 'btn', 'aria-disabled': saving, onclick: () => restore(record) }, 'Restore')]
    : isNew
      ? [h('button', { type: 'button', class: 'btn', 'aria-disabled': saving, onclick: () => keep(record) }, 'Keep'),
        h('button', { type: 'button', class: 'quiet', onclick: () => startEdit(panel) }, 'Edit'),
        h('button', { type: 'button', class: 'quiet muted', 'aria-disabled': saving, onclick: () => discard(record) }, 'Discard')]
      : [h('button', { type: 'button', class: 'quiet', onclick: () => startEdit(panel) }, 'Edit'),
        h('button', { type: 'button', class: 'quiet', onclick: () => { panel.mode = 'move'; render(); } }, 'Move to…'),
        h('button', { type: 'button', class: 'quiet danger', 'aria-disabled': saving, onclick: () => archive(record) }, 'Archive')];
  const source = record.provenance || {};
  return [h('h2', { class: 'panel-title', id: 'panel-title', tabindex: '-1', text: record.title || `Memory #${record.id}` }),
    h('p', { class: 'reading full', text: record.content }),
    h('div', { class: 'actions' }, actions),
    similar ? section('Similar', [row(similar), h('button', { type: 'button', class: 'quiet more', 'aria-disabled': saving, onclick: () => keepAndReplace(record, similar) }, 'Keep and replace it')], 'warn') : null,
    section('Applies to', h('p', { class: 'val' }, whereLabel(record.scope), record.scope?.project ? h('span', { class: 'meta', text: record.scope.project }) : null)),
    // "Loads" means it can load; Preview startup shows the size limit's cuts.
    section('Startup', h('p', { class: 'val', text: record.archived ? 'Not loaded' : isNew ? 'Not until you keep it' : 'Loads' })),
    // The owner's own saves carry fixed internal provenance, so only the date shows.
    section(isNew ? 'Noticed by' : 'Saved by', by(record) === 'you' ? h('p', { class: 'val' }, 'You', h('span', { class: 'meta', text: fullDate(record.updated_at) }))
      : h('p', { class: 'val' }, by(record),
        h('span', { class: 'meta', text: [fullDate(record.updated_at), source.device && deviceName(source.device)].filter(Boolean).join(' · ') }),
        source.source ? h('span', { class: 'meta', text: `“${source.source}”` }) : null)),
    record.purpose === 'handoff' && !record.archived ? section('Kept until', h('p', { class: 'val' }, 'The next handoff', h('span', { class: 'meta', text: 'Then it moves to Archived.' }))) : null,
    // Shown only when an earlier version differs; a keep or archive repeats the text.
    // Earlier versions only, full width: the current text is already at the top.
    panel.history?.length || (panel.history && panel.olderFrom >= 1) ? h('section', { class: 'sec stacked' }, h('h2', { class: 'sec-label', text: 'History' }),
      h('div', { class: 'list' }, panel.history.map(older => versionBlock(record, older)),
        panel.olderFrom >= 1 ? h('button', { type: 'button', class: 'quiet more', onclick: () => loadHistory(panel, true) }, 'Show older versions') : null)) : null];
}

function devicePanel(panel) {
  const device = state.devices.find(item => item.id === panel.id);
  if (!device) return h('p', { class: 'label', text: 'This agent is no longer connected.' });
  const name = device.label || device.device;
  return [h('h2', { class: 'panel-title', text: name }),
    section('Name', h('form', { class: 'edit', onsubmit: event => { event.preventDefault(); renameDevice(device, event.target.querySelector('input').value, panel); } },
      h('input', { id: 'device-name', type: 'text', 'aria-label': 'Name', value: panel.nameDraft ?? device.label ?? '', placeholder: device.device, oninput: event => { panel.nameDraft = event.target.value; } }),
      h('p', { class: 'meta', text: `ID: ${device.device} · never changes. Leave the name empty to show the ID.` }),
      h('div', { class: 'actions' }, h('button', { type: 'submit', class: 'btn', 'aria-disabled': saving }, 'Save name')))),
    section('Status', h('p', { class: 'val' }, h('i', { class: recentlySeen(device) ? 'dot on' : 'dot', 'aria-hidden': 'true' }), recentlySeen(device) ? 'Active' : 'Idle',
      h('span', { class: 'meta', text: device.last_seen_at ? `Last seen ${ago(device.last_seen_at)}` : 'Not seen since 2.9.0' }))),
    section('Connected', h('p', { class: 'val', text: fullDate(device.created_at) })),
    // Focus goes to the question, never the destructive button, so a held or
    // repeated Enter on the opener cannot disconnect.
    panel.confirm
      ? h('div', { class: 'confirm', role: 'group', 'aria-labelledby': 'disconnect-question' },
        h('p', { class: 'label', id: 'disconnect-question', tabindex: '-1', text: `Disconnect ${name}? Its agents stop reading and saving right away.` }),
        h('div', { class: 'actions' },
          h('button', { type: 'button', class: 'btn danger', 'data-key': `disconnect-confirm:${device.id}`, onclick: () => disconnectDevice(device) }, 'Disconnect'),
          h('button', { type: 'button', class: 'quiet muted', 'data-key': `disconnect-cancel:${device.id}`, onclick: () => { panel.confirm = false; render(); focusByKey(`disconnect-open:${device.id}`); } }, 'Cancel')))
      : h('div', { class: 'actions' }, h('button', { type: 'button', class: 'quiet danger', 'data-key': `disconnect-open:${device.id}`, onclick: () => { panel.confirm = true; render(); document.getElementById('disconnect-question')?.focus?.(); } }, 'Disconnect'))];
}

function startupPanel(panel) {
  const data = panel.data;
  const reasons = { unconfirmed: 'not kept yet', older_handoff: 'older handoff', over_budget: `over ${panel.agent}'s size limit` };
  return [h('h2', { class: 'panel-title', text: `${panel.agent} in ${panel.project ? projectName(panel.project) : 'no project'}` }),
    panel.error ? h('p', { class: 'label', text: panel.error }) : !data ? h('p', { class: 'meta loading', text: 'Loading…' }) : [
      section('Gets', (data.records || []).length ? data.records.map(record => h('div', { class: 'row static' }, h('span', { class: 't', text: record.title }), h('span', { class: 'm', text: record.purpose === 'handoff' ? 'last session' : whereLabel(record.scope) }))) : h('p', { class: 'meta', text: 'Nothing would load.' })),
      (data.not_loaded || []).length ? section('Left out', [data.not_loaded.map(item => h('div', { class: 'row static' }, h('span', { class: 't muted', text: item.title || `Memory #${item.id}` }), h('span', { class: 'm', text: reasons[item.reason] || item.reason }))),
        data.not_loaded_total > data.not_loaded.length ? h('p', { class: 'meta', text: `And ${data.not_loaded_total - data.not_loaded.length} more.` }) : null]) : null]];
}

function panelBody(panel) {
  const back = { memory: currentPlace(), device: 'Agents', startup: currentPlace() }[panel.type];
  return [h('div', { class: 'panel-top' },
    h('button', { type: 'button', class: 'quiet back', 'data-key': 'back', onclick: closePanel }, h('span', { 'aria-hidden': 'true', text: '←' }), ` ${back}`),
    h('button', { type: 'button', class: 'icon-btn close-x', 'aria-label': 'Close', onclick: closePanel }, '×')),
  panel.type === 'memory' ? memoryPanel(panel) : panel.type === 'device' ? devicePanel(panel) : startupPanel(panel)];
}
function panelView() {
  const panel = state.panel;
  if (!panel) return null;
  return h('aside', { class: 'panel', 'aria-label': { memory: 'Memory', device: 'Agent', startup: 'Startup preview' }[panel.type] }, panelBody(panel));
}
// Below 1200px the opened item replaces the list, so it becomes the page's
// main region (and the skip link's target) instead of a hidden list.
const narrowLayout = window.matchMedia?.('(max-width: 1199px)');
narrowLayout?.addEventListener?.('change', () => { if (state.active) render(); });

function signInView() {
  return h('main', { class: 'signin', id: 'main', tabindex: '-1' },
    h('form', { class: 'signin-box', autocomplete: 'off', onsubmit: event => { event.preventDefault(); if (state.signingIn) return; const input = event.target.querySelector('#token'); const token = input.value; input.value = ''; signIn(token); } },
      h('p', { class: 'brand', text: 'GRASSHOPPER' }),
      h('h1', { text: 'Sign in' }),
      state.route.connect ? h('p', { class: 'label', text: 'An agent is asking to connect. Sign in to check its code.' }) : null,
      state.signedOutNote ? h('p', { class: 'label muted', role: 'status', text: state.signedOutNote }) : null,
      h('label', { class: 'field' }, 'Access token', h('input', { id: 'token', type: 'password', autocomplete: 'off', spellcheck: 'false', 'aria-invalid': state.signInError ? 'true' : 'false', 'aria-describedby': state.signInError ? 'token-error' : null })),
      state.signInError ? h('p', { class: 'error', id: 'token-error', role: 'alert', text: state.signInError }) : null,
      h('div', { class: 'actions' }, h('button', { type: 'submit', class: 'btn', 'data-key': 'sign-in', 'aria-disabled': state.signingIn }, state.signingIn ? 'Signing in…' : 'Sign in')),
      h('details', {}, h('summary', {}, "Where's my token?"),
        h('dl', {},
          h('dt', { text: 'macOS' }), h('dd', { text: '~/Library/Application Support/Grasshopper/access-token' }),
          h('dt', { text: 'Windows' }), h('dd', { text: '%APPDATA%\\Grasshopper\\access-token' }),
          h('dt', { text: 'Linux' }), h('dd', { text: '~/.config/Grasshopper/access-token' })),
        h('p', { class: 'meta', text: 'Use the owner token, not a device token. Never paste it into an agent chat.' }))));
}

function mainView() {
  switch (state.route.page) {
    case 'all': return allPage();
    case 'new': return newPage();
    case 'search': return searchPage();
    case 'archived': return archivedPage();
    case 'agents': return agentsPage();
    case 'settings': return settingsPage();
    default:
      if (state.route.project) return projectPage();
      return state.index && !state.index.projects.length && !state.index.total ? firstRun() : h('p', { class: 'meta loading', text: 'Loading…' });
  }
}

// Controls are matched across redraws by data-key first, so a control whose
// label changes (Save to Saving…) or two that share a label (Disconnect and
// its confirmation) never trade focus.
function keyOf(node) { return node.getAttribute?.('data-key') || node.getAttribute?.('aria-label') || node.id || node.textContent; }
function focusByKey(key) {
  const candidates = [...(root.querySelectorAll?.('button, input, textarea, select, a, summary, [tabindex]') || [])];
  // An exact data-key wins; after that a native control wins over a focusable
  // region (such as main, labelled with a memory title) that reads the same.
  const native = node => /^(BUTTON|INPUT|TEXTAREA|SELECT|A|SUMMARY)$/.test(node.tagName);
  const match = candidates.find(node => node.getAttribute?.('data-key') === key)
    || candidates.find(node => native(node) && keyOf(node) === key)
    || candidates.find(node => keyOf(node) === key);
  match?.focus?.();
  // A match inside hidden content cannot take focus; report that so the caller falls back.
  return Boolean(match) && document.activeElement === match;
}
// A redraw must not strand keyboard focus on a removed control.
function restoreFocus(key) { if (!focusByKey(key)) document.getElementById('main')?.focus?.(); }

let lastPlace = '';
function render() {
  if (state.checking) return;
  // Keep typing in the edit form across redraws, but only into the panel that owns the form.
  if (state.panel?.mode === 'edit' && document.getElementById('edit-text')?.getAttribute?.('data-panel') === state.panel.key) {
    state.panel.draft = { title: document.getElementById('edit-title').value, content: document.getElementById('edit-text').value };
  }
  const focused = document.activeElement;
  const focusKey = focused && focused !== document.body ? keyOf(focused) : '';
  document.title = state.active ? `${state.panel?.record?.title || currentPlace()} · Grasshopper` : 'Grasshopper';
  if (!state.active) { root.replaceChildren(signInView()); if (focusKey) restoreFocus(focusKey); return; }
  // Scrolling areas keep their position across a redraw; the list starts at the
  // top when the page changes, but not when a memory opens or closes beside it.
  const place = routeHash({ ...state.route, memory: null });
  const areas = ['.side', 'aside.panel', ...(place === lastPlace ? ['main'] : [])];
  lastPlace = place;
  const scrolled = areas.map(selector => [selector, root.querySelector?.(selector)?.scrollTop || 0]);
  const narrow = Boolean(narrowLayout?.matches) && Boolean(state.panel);
  const panel = narrow ? null : panelView();
  const review = state.index?.review || 0;
  root.replaceChildren(...[h('div', { class: `app${panel ? ' has-panel' : ''}${state.menu ? ' menu-open' : ''}` },
    h('header', { class: 'topbar' }, h('span', { class: 'brand', text: 'GRASSHOPPER' }),
      h('button', { type: 'button', class: 'menu-btn', 'data-key': 'menu', 'aria-expanded': String(state.menu), onclick: () => { state.menu = !state.menu; render(); } },
        `${currentPlace()} `, h('span', { 'aria-hidden': 'true', text: state.menu ? '▴' : '▾' }), review ? h('span', { class: 'warn', text: ` · ${review} new` }) : null)),
    sidebar(),
    narrow
      ? h('main', { id: 'main', tabindex: '-1', class: 'panel panel-main', 'aria-label': state.panel.record?.title || 'Memory' }, panelBody(state.panel))
      : h('main', { id: 'main', tabindex: '-1', class: state.offline ? 'offline' : null }, h('div', { class: 'page' }, mainView())),
    panel),
  state.undo ? h('div', { class: 'undo', role: 'status',
    onfocusin: holdUndo, onmouseenter: holdUndo,
    onfocusout: event => { if (!event.currentTarget.matches?.(':hover')) armUndo(); },
    onmouseleave: event => { if (!event.currentTarget.contains?.(document.activeElement)) armUndo(); } },
  h('span', { text: `${state.undo.verb} “${state.undo.record.title}”` }), h('button', { type: 'button', class: 'quiet', 'data-key': 'undo', onclick: undo }, 'Undo')) : null].filter(Boolean));
  for (const [selector, top] of scrolled) { const node = root.querySelector?.(selector); if (node && top) node.scrollTop = top; }
  if (focusKey) restoreFocus(focusKey);
  // An approval link moves focus to its request once, not on every redraw.
  if (state.route.connect && !state.approvalFocused) {
    const request = document.getElementById('focused-request');
    if (request) { request.focus?.(); state.approvalFocused = true; }
  }
}

checkSession();

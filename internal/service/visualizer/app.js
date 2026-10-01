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
  undo: null, startupAgent: 'Codex', startupProject: '', exportArchived: false, exportStatus: ''
};
const timers = { page: null, index: null, devices: null, undo: null };
const inFlight = { page: null, index: null, devices: null };
const writes = new Map();
let saving = false;

// ---------- small helpers ----------

function h(tag, props, ...children) {
  const node = document.createElement(tag);
  for (const [key, value] of Object.entries(props || {})) {
    if (value === undefined || value === null || value === false) continue;
    if (key === 'class') node.className = value;
    else if (key === 'text') node.textContent = value;
    else if (key.startsWith('on')) node.addEventListener(key.slice(2), value);
    else if (key === 'value' || key === 'checked' || key === 'disabled' || key === 'hidden') node[key] = value;
    else node.setAttribute(key, value === true ? '' : String(value));
  }
  for (const child of children.flat(Infinity)) if (child !== null && child !== undefined && child !== false) node.append(child);
  return node;
}

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

function whereLabel(scope = {}) {
  const extra = [scope.device && `${scope.device} only`, scope.platform && `${PLATFORM[scope.platform] || scope.platform} only`].filter(Boolean);
  return [projectName(scope.project), ...extra].join(' · ');
}

function by(record) { const harness = record?.provenance?.harness; return HARNESS[harness] || harness || 'unknown'; }

function day(value) { const date = new Date(value || NaN); return Number.isNaN(date.getTime()) ? null : date; }
function daysAgo(value, now = new Date()) {
  const date = day(value);
  if (!date) return null;
  const local = d => Date.UTC(d.getFullYear(), d.getMonth(), d.getDate());
  return Math.round((local(now) - local(date)) / 86400000);
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
  Object.assign(state, { active: false, generation: state.generation + 1, index: null, page: null, panel: null, devices: [], pairings: [], undo: null, signedOutNote: state.keptDraft ? 'Your session ended. Sign in again; your unsaved text is kept.' : 'Your session ended. Sign in again.' });
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
    if (changed && !editing() && !selecting()) render();
  } catch (error) {
    if (error.session || generation !== state.generation || inFlight.page !== controller) return;
    state.loading = false;
    state.offline = state.offline || new Date();
    render();
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

function sortedProjects() {
  const recency = state.index?.recency || new Map();
  return [...(state.index?.projects || [])].sort((a, b) => String(recency.get(b) || '').localeCompare(String(recency.get(a) || '')) || a.localeCompare(b));
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
    if ((page || wasOffline) && !editing() && !selecting()) render();
  } catch (error) {
    if (!error.session && generation === state.generation && inFlight.index === controller) { state.offline = state.offline || new Date(); render(); }
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
      if (changed && !editing()) render();
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
async function restoreDraft(kept) {
  await openMemory(kept.id, false);
  if (state.panel?.id === kept.id && state.panel.record) {
    Object.assign(state.panel, { mode: 'edit', draft: { title: kept.title, content: kept.content } });
    render();
  }
}

async function signOut(everywhere) {
  if (dirty() && !confirmLeave()) return;
  try {
    const result = everywhere ? await api('/visualizer/api/session/revoke-all', {}) : await api('/visualizer/api/session', undefined, { method: 'DELETE' });
    if (result.status >= 300) throw new Error();
    stopPolling();
    Object.assign(state, { active: false, generation: state.generation + 1, index: null, page: null, panel: null, undo: null, signedOutNote: everywhere ? 'Signed out on every browser.' : 'Signed out.' });
    writes.clear();
  } catch (error) { if (!error.session) say("Couldn't sign out. Try again."); }
  render();
}

// ---------- memory panel ----------

async function openMemory(id, push = true) {
  if (dirty() && !confirmLeave()) return;
  const panel = { type: 'memory', id, record: null, history: null, mode: 'view', error: '', draft: null, theirs: null };
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
  if (!editing()) render();
}

function closePanel() {
  if (dirty() && !confirmLeave()) return;
  state.panel = null;
  if (state.route.memory) { state.route.memory = null; setURL(true); }
  render();
}

function editing() { return state.panel?.mode === 'edit'; }
// The text being edited, if it differs from the saved memory.
function draftOf(panel) {
  if (!panel?.record || (panel.mode !== 'edit' && panel.mode !== 'conflict')) return null;
  const title = document.getElementById('edit-title')?.value ?? panel.draft?.title ?? panel.record.title;
  const content = document.getElementById('edit-text')?.value ?? panel.draft?.content ?? panel.record.content;
  return title !== panel.record.title || content !== panel.record.content ? { id: panel.record.id, title, content } : null;
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
  panel.draft = { title, content };
  if (!content.trim()) { panel.error = 'Write some text before saving.'; render(); return false; }
  const bytes = text => new TextEncoder().encode(text).length;
  if (bytes(title) > 512 || bytes(content) > 32768) { panel.error = 'Keep the title under 512 bytes and the text under 32,768.'; render(); return false; }
  panel.error = '';
  const expected = panel.theirs ? panel.theirs.revision : record.revision;
  return write('/visualizer/api/update', { id: record.id, expected_revision: expected, title, content, purpose: record.purpose }, {
    onDone: () => { Object.assign(panel, { mode: 'view', draft: null, theirs: null }); say('Saved.'); loadMemory(panel); },
    onConflict: current => { if (current) Object.assign(panel, { mode: 'conflict', theirs: current }); }
  });
}
function keepTheirs(panel) { Object.assign(panel, { mode: 'view', draft: null, theirs: null }); loadMemory(panel); }
function move(record, project) {
  return write('/visualizer/api/move', { id: record.id, expected_revision: record.revision, project: project || null }, {
    onDone: receipt => { say(`Moved to ${projectName(project)}.`); openMemory(receipt.id); },
    onConflict: () => { say(`Not moved. ${projectName(project)} may already have it, or it changed. Showing the latest.`); if (state.panel) { state.panel.mode = 'view'; loadMemory(state.panel); } }
  });
}

function offerUndo(record, revision, verb) {
  clearTimeout(timers.undo);
  state.undo = { record, revision, verb };
  timers.undo = setTimeout(() => { state.undo = null; render(); }, 10000);
}
async function undo() {
  const pending = state.undo;
  if (!pending) return;
  if (saving) { say('Another change is still saving. Try Undo again in a moment.'); return; }
  clearTimeout(timers.undo);
  const done = await write('/visualizer/api/archive', { id: pending.record.id, expected_revision: pending.revision, archived: false }, {
    onDone: () => { say(`Restored “${pending.record.title}”.`); if (state.undo === pending) state.undo = null; }
  });
  // On failure the Undo stays for another ten seconds so it can be retried.
  if (!done && state.undo === pending) timers.undo = setTimeout(() => { if (state.undo === pending) { state.undo = null; render(); } }, 10000);
}

async function decidePairing(request, decision) {
  try {
    const { status } = await api('/visualizer/api/pairings', { request_id: request.request_id, code: request.code, decision });
    say(status >= 300 ? "Couldn't change this request. Try again." : decision === 'approve' ? `${request.device} connected.` : `${request.device} denied.`);
  } catch (error) { if (!error.session) say("Couldn't change this request. Try again."); }
  refreshDevices();
}
async function disconnectDevice(device) {
  try {
    const { status } = await api('/visualizer/api/devices', { id: device.id }, { method: 'DELETE' });
    say(status >= 300 ? "Couldn't disconnect it. Try again." : `${device.device} disconnected.`);
    if (status < 300) state.panel = null;
  } catch (error) { if (!error.session) say("Couldn't disconnect it. Try again."); }
  refreshDevices();
  render();
}

async function openStartup() {
  const panel = { type: 'startup', agent: state.startupAgent, project: state.startupProject, data: null, error: '' };
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
  try { await navigator.clipboard.writeText(text); button.textContent = 'Copied'; } catch { button.textContent = 'Select and copy'; }
  setTimeout(() => { button.textContent = 'Copy'; }, 1500);
}

// ---------- views ----------

function currentPlace() {
  const route = state.route;
  return route.page === 'project' ? projectName(route.project) : { all: 'All projects', new: 'New', agents: 'Agents', archived: 'Archived', settings: 'Settings', search: 'Search' }[route.page] || 'Grasshopper';
}

function navButton(label, route, { current = false, count = '', extra = '' } = {}) {
  return h('button', { type: 'button', class: `nav ${extra}`.trim(), 'aria-current': current ? 'page' : null, onclick: () => go(route) },
    h('span', { text: label }), count ? h('span', { class: 'count', text: count }) : null);
}

function sidebar() {
  const route = state.route;
  const projects = sortedProjects();
  const shown = state.moreProjects ? projects : projects.slice(0, 5);
  const review = state.index?.review || 0;
  const devices = state.devices.filter(device => !device.revoked_at);
  const waiting = state.pairings.filter(request => request.status === 'pending').length;
  return h('nav', { class: 'side', 'aria-label': 'Memory view' },
    h('p', { class: 'brand', text: 'GRASSHOPPER' }),
    review ? h('div', { class: 'nav-group' }, navButton('New', { page: 'new' }, { current: route.page === 'new', count: String(review), extra: 'new' })) : null,
    h('div', { class: 'nav-group' },
      navButton('All projects', { page: 'all' }, { current: route.page === 'all' }),
      shown.map(project => navButton(projectName(project), { page: 'project', project }, { current: route.page === 'project' && route.project === project })),
      projects.length > 5 ? h('button', { type: 'button', class: 'nav', onclick: () => { state.moreProjects = !state.moreProjects; render(); } }, h('span', { text: state.moreProjects ? 'Fewer' : `${projects.length - 5} more` })) : null),
    h('div', { class: 'nav-group side-foot' },
      h('button', { type: 'button', class: 'nav', 'aria-current': route.page === 'agents' ? 'page' : null, onclick: () => go({ page: 'agents' }) },
        h('span', {}, h('span', { class: 'dots', 'aria-hidden': 'true' }, devices.slice(0, 6).map(device => h('i', { class: recentlySeen(device) ? 'dot on' : 'dot' }))), 'Agents'),
        waiting ? h('span', { class: 'count warn', text: `${waiting} waiting` }) : null),
      navButton('Archived', { page: 'archived' }, { current: route.page === 'archived' }),
      navButton('Settings', { page: 'settings' }, { current: route.page === 'settings' })));
}

function row(record, { meta } = {}) {
  const only = [record.scope?.device && `${record.scope.device} only`, record.scope?.platform && `${PLATFORM[record.scope.platform] || record.scope.platform} only`].filter(Boolean).join(' · ');
  const right = meta ?? [by(record), relDate(record.updated_at), only].filter(Boolean).join(' · ');
  return h('button', { type: 'button', class: `row${record.purpose === 'handoff' ? ' handoff' : ''}`, 'data-key': `open:${record.id}`, 'aria-current': state.panel?.id === record.id ? 'true' : null,
    title: fullDate(record.updated_at), onclick: () => openMemory(record.id) },
  h('span', { class: 't', text: record.title || `Memory #${record.id}` }), h('span', { class: 'm', text: right }));
}

function section(label, children, cls = '') {
  return h('section', { class: 'sec' }, h('h2', { class: `sec-label ${cls}`.trim(), text: label }), h('div', { class: 'list' }, children));
}

function suggestionRow(record, kept) {
  const similar = similarTo(record, kept);
  const meta = [by(record), relDate(record.updated_at), similar ? 'similar' : ''].filter(Boolean).join(' · ');
  return h('div', { class: 'row-inline' }, row(record, { meta }),
    h('span', { class: 'actions tight' },
      h('button', { type: 'button', class: 'quiet', disabled: saving, 'data-key': `keep:${record.id}`, 'aria-label': `Keep ${record.title}`, onclick: () => keep(record) }, 'Keep'),
      h('button', { type: 'button', class: 'quiet muted', disabled: saving, 'data-key': `discard:${record.id}`, 'aria-label': `Discard ${record.title}`, onclick: () => discard(record) }, 'Discard')));
}

function grouped(records) {
  if (records.length <= 8) return null;
  return recencyGroups(records).map(group => {
    const limit = group.label === 'This week' ? Infinity : 3;
    const visible = state.expanded.has(group.label) ? group.records : group.records.slice(0, limit);
    const rest = group.records.length - visible.length;
    return section(group.label, [visible.map(record => row(record)),
      rest > 0 ? h('button', { type: 'button', class: 'quiet more', onclick: () => { state.expanded.add(group.label); render(); } }, `Show ${rest} more`) : null]);
  });
}

function head(title, sub, { search = true } = {}) {
  return h('div', { class: 'head' },
    h('div', {}, h('h1', { id: 'page-title', text: title }), sub ? h('p', { class: 'meta sub', text: sub }) : null),
    search ? h('form', { class: 'search', role: 'search', onsubmit: event => {
      event.preventDefault();
      const query = event.target.querySelector('input').value.trim();
      if (query) go({ page: 'search', query, project: state.route.page === 'project' ? state.route.project : '' });
    } }, h('input', { type: 'search', 'aria-label': 'Search memories', placeholder: 'Search', value: state.searchDraft ?? (state.route.page === 'search' ? state.route.query : ''),
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
    h('div', { class: 'actions' }, h('button', { type: 'button', class: 'btn', onclick: () => go({ page: 'agents' }) }, 'Connect an agent')));
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
    return [head(projectName(project), `${own.length} ${own.length === 1 ? 'memory' : 'memories'}`), banner(),
      fresh.length ? section('New', fresh.map(record => suggestionRow(record, kept)), 'warn') : null,
      grouped(own) || section(projectName(project), own.length ? own.map(record => row(record)) : h('p', { class: 'meta empty', text: 'Nothing kept for this project yet.' })),
      global.length ? section('All projects', state.showGlobal
        ? [global.map(record => row(record)), h('button', { type: 'button', class: 'quiet more muted', onclick: () => { state.showGlobal = false; render(); } }, 'Hide')]
        : h('button', { type: 'button', class: 'quiet more', onclick: () => { state.showGlobal = true; render(); } }, `Show ${global.length} that also apply here`)) : null,
      handoff ? section(shortDate(handoff.updated_at), row(handoff, { meta: by(handoff) })) : null];
  });
}

function allPage() {
  return loadingOr(() => {
    const records = state.page.records.filter(record => !record.scope?.project);
    if (state.index && !state.index.total && !records.length) return firstRun();
    const fresh = records.filter(record => !record.confirmed && record.purpose !== 'handoff');
    const kept = records.filter(record => record.confirmed && record.purpose !== 'handoff');
    const handoff = records.filter(record => record.purpose === 'handoff').sort((a, b) => String(b.updated_at).localeCompare(String(a.updated_at)))[0];
    return [head('All projects', 'Memories that apply to every project'), banner(),
      fresh.length ? section('New', fresh.map(record => suggestionRow(record, kept)), 'warn') : null,
      grouped(kept) || section('All projects', kept.length ? kept.map(record => row(record)) : h('p', { class: 'meta empty', text: 'Nothing applies to every project yet.' })),
      handoff ? section(shortDate(handoff.updated_at), row(handoff, { meta: by(handoff) })) : null];
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
    const kept = state.index?.records || [];
    return [h('div', { class: 'head' },
      h('div', {}, h('h1', { id: 'page-title', text: 'New' }), h('p', { class: 'meta sub', text: `${records.length} waiting · agents load them at startup only after you keep them` })),
      h('button', { type: 'button', class: 'quiet', disabled: saving, onclick: keepAll }, 'Keep all')), banner(),
    [null, ...sortedProjects()].map(project => {
      const items = records.filter(record => (record.scope?.project || null) === project);
      return items.length ? section(projectName(project), items.map(record => suggestionRow(record, kept))) : null;
    })];
  });
}

function searchPage() {
  const records = state.page?.records || [];
  const order = [state.route.project || null, null, ...sortedProjects()].filter((value, index, all) => all.indexOf(value) === index);
  const body = !state.page ? h('p', { class: 'meta loading', text: 'Searching…' })
    : !records.length ? h('p', { class: 'meta empty', text: `Nothing matches “${state.route.query}”. Try fewer words.` })
    : order.map(project => { const items = records.filter(record => (record.scope?.project || null) === project); return items.length ? section(projectName(project), items.map(record => row(record))) : null; });
  const omitted = state.page?.omitted_records || [];
  const titles = state.page?.omitted_titles || {};
  const extra = omitted.length ? section('More', [h('p', { class: 'meta', text: `${omitted.length} more ${omitted.length === 1 ? 'match' : 'matches'} did not fit in one response.` }),
    omitted.map(reference => h('button', { type: 'button', class: 'row', 'data-key': `open:${reference.id}`, onclick: () => openMemory(reference.id) },
      h('span', { class: 't', text: titles[reference.id] || `Memory #${reference.id}` }), h('span', { class: 'm', text: projectName(reference.scope?.project) })))]) : null;
  const found = records.length + omitted.length;
  return [head('Search', state.page ? `${found} found${state.page.semantic_ready === false ? ' · exact words only' : ''}` : ''), banner(), body, extra];
}

function archivedPage() {
  return loadingOr(() => {
    const records = state.page.records;
    if (!records.length) return [head('Archived', '', { search: false }), h('p', { class: 'meta empty', text: 'Nothing archived.' })];
    const label = record => {
      const source = record.provenance?.source || '';
      return `${source.startsWith('Replaced by handoff') ? 'replaced' : source.startsWith('Moved to memory') ? 'moved' : 'archived'} ${relDate(record.updated_at)}`;
    };
    return [head('Archived', 'Agents never see these.', { search: false }), banner(),
      [null, ...sortedProjects()].map(project => {
        const items = records.filter(record => (record.scope?.project || null) === project);
        return items.length ? section(projectName(project), items.map(record => h('div', { class: 'row-inline' }, row(record, { meta: label(record) }),
          h('button', { type: 'button', class: 'quiet', disabled: saving, 'data-key': `restore:${record.id}`, 'aria-label': `Restore ${record.title}`, onclick: () => restore(record) }, 'Restore')))) : null;
      })];
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
      h('span', { class: 't' }, h('i', { class: recentlySeen(device) ? 'dot on' : 'dot', 'aria-hidden': 'true' }), device.device),
      h('span', { class: 'm', text: ago(device.last_seen_at) }))) : h('p', { class: 'meta empty', text: 'No agents connected yet.' })),
    section('Add one', h('div', { class: 'stack' },
      h('p', { class: 'label', text: 'In a new agent session, say:' }),
      h('div', { class: 'strip' }, h('code', { text: 'Connect Grasshopper' }), h('button', { type: 'button', class: 'btn', onclick: event => copyText('Connect Grasshopper', event.currentTarget) }, 'Copy')),
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
      h('div', {}, h('button', { type: 'button', class: 'btn', onclick: openStartup }, 'Preview')))),
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

function editForm(panel) {
  const record = panel.record;
  const draft = panel.draft || { title: record.title, content: record.content };
  const out = [];
  if (panel.mode === 'conflict') out.push(h('div', { class: 'conflict', role: 'alert' },
    h('p', { class: 'label warn', text: `${by(panel.theirs) === 'you' ? 'Someone' : by(panel.theirs)} changed this while you were editing. Nothing was overwritten.` }),
    h('div', {}, h('p', { class: 'who', text: `${by(panel.theirs)} · ${relDate(panel.theirs.updated_at)}` }), h('p', { class: 'reading', text: panel.theirs.content })),
    h('div', {}, h('p', { class: 'who', text: 'Yours' }), h('p', { class: 'reading', text: draft.content })),
    h('div', { class: 'actions' },
      h('button', { type: 'button', class: 'btn', disabled: saving, onclick: () => saveEdit(panel) }, 'Keep yours'),
      h('button', { type: 'button', class: 'quiet', onclick: () => keepTheirs(panel) }, 'Keep theirs'))));
  out.push(h('form', { class: 'edit', hidden: panel.mode === 'conflict', onsubmit: event => { event.preventDefault(); saveEdit(panel); } },
    h('label', { class: 'field' }, 'Title', h('input', { id: 'edit-title', type: 'text', value: draft.title })),
    h('label', { class: 'field' }, 'Text', h('textarea', { id: 'edit-text', rows: '6', value: draft.content })),
    panel.error ? h('p', { class: 'error', role: 'alert', text: panel.error }) : null,
    h('div', { class: 'actions' },
      h('button', { type: 'submit', class: 'btn', disabled: saving }, saving ? 'Saving…' : 'Save'),
      h('button', { type: 'button', class: 'quiet muted', onclick: () => { if (!dirty() || confirmLeave()) { Object.assign(panel, { mode: 'view', error: '', draft: null }); render(); } } }, 'Cancel')),
    h('p', { class: 'meta', text: 'Your edit becomes the current version. The old one stays in History.' })));
  return out;
}

function startEdit(panel) { panel.mode = 'edit'; render(); document.getElementById('edit-text')?.focus?.(); }

function memoryPanel(panel) {
  const record = panel.record;
  if (!record) return h('p', { class: panel.error ? 'label' : 'meta loading', text: panel.error || 'Loading…' });
  if (panel.mode === 'edit' || panel.mode === 'conflict') return editForm(panel);
  if (panel.mode === 'move') {
    return [h('h2', { class: 'panel-title', text: record.title }), h('p', { class: 'label', text: 'Move to' }),
      h('div', { class: 'picker' }, (record.scope?.project ? [null] : sortedProjects().filter(project => /^id:.+|^git:[^/]+\/.+/.test(project))).map(project =>
        h('button', { type: 'button', class: 'pick', disabled: saving, onclick: () => move(record, project) }, projectName(project)))),
      h('button', { type: 'button', class: 'quiet muted', onclick: () => { panel.mode = 'view'; render(); } }, 'Cancel')];
  }
  const isNew = !record.confirmed && record.purpose !== 'handoff' && !record.archived;
  const similar = isNew ? similarTo(record, [...(state.index?.records || []), ...(state.page?.records || [])]) : null;
  const movable = record.purpose !== 'handoff' && !record.scope?.device && !record.scope?.platform;
  const actions = record.archived
    ? [h('button', { type: 'button', class: 'btn', disabled: saving, onclick: () => restore(record) }, 'Restore')]
    : isNew
      ? [h('button', { type: 'button', class: 'btn', disabled: saving, onclick: () => keep(record) }, 'Keep'),
        h('button', { type: 'button', class: 'quiet', onclick: () => startEdit(panel) }, 'Edit'),
        h('button', { type: 'button', class: 'quiet muted', disabled: saving, onclick: () => discard(record) }, 'Discard')]
      : [h('button', { type: 'button', class: 'quiet', onclick: () => startEdit(panel) }, 'Edit'),
        movable ? h('button', { type: 'button', class: 'quiet', onclick: () => { panel.mode = 'move'; render(); } }, 'Move to…') : null,
        h('button', { type: 'button', class: 'quiet danger', disabled: saving, onclick: () => archive(record) }, 'Archive')];
  const source = record.provenance || {};
  return [h('h2', { class: 'panel-title', id: 'panel-title', tabindex: '-1', text: record.title || `Memory #${record.id}` }),
    h('p', { class: 'reading full', text: record.content }),
    h('div', { class: 'actions' }, actions),
    similar ? section('Similar', [row(similar), h('button', { type: 'button', class: 'quiet', disabled: saving, onclick: () => keepAndReplace(record, similar) }, 'Keep and replace it')], 'warn') : null,
    section('Applies to', h('p', { class: 'val' }, whereLabel(record.scope), h('span', { class: 'meta', text: record.scope?.project || 'Every project' }))),
    section(isNew ? 'Noticed by' : 'Saved by', h('p', { class: 'val' }, by(record) === 'you' ? 'You' : by(record),
      h('span', { class: 'meta', text: [fullDate(record.updated_at), source.device].filter(Boolean).join(' · ') }),
      source.source ? h('span', { class: 'meta', text: `“${source.source}”` }) : null)),
    record.purpose === 'handoff' && !record.archived ? section('Kept until', h('p', { class: 'val' }, 'The next handoff', h('span', { class: 'meta', text: 'Then it moves to Archived.' }))) : null,
    record.revision > 1 ? section('History', panel.history === null ? h('p', { class: 'meta', text: 'Loading…' }) : [
      h('div', { class: 'row static' }, h('span', { class: 't', text: record.title }), h('span', { class: 'm', text: `${by(record)} · ${relDate(record.updated_at)} · current` })),
      panel.history.map(older => h('div', { class: 'row-inline' },
        h('div', { class: 'row static' }, h('span', { class: 't muted', text: older.title || older.content.slice(0, 80) }), h('span', { class: 'm', text: `${by(older)} · ${relDate(older.updated_at)}` })),
        record.archived ? null : h('button', { type: 'button', class: 'quiet', disabled: saving, 'data-key': `restore-version:${older.revision}`, 'aria-label': `Restore version from ${relDate(older.updated_at)}`, onclick: () => restoreRevision(record, older) }, 'Restore'))),
      panel.olderFrom >= 1 ? h('button', { type: 'button', class: 'quiet more', onclick: () => loadHistory(panel, true) }, 'Show older versions') : null]) : null];
}

function devicePanel(panel) {
  const device = state.devices.find(item => item.id === panel.id);
  if (!device) return h('p', { class: 'label', text: 'This agent is no longer connected.' });
  return [h('h2', { class: 'panel-title', text: device.device }),
    section('Status', h('p', { class: 'val' }, h('i', { class: recentlySeen(device) ? 'dot on' : 'dot', 'aria-hidden': 'true' }), recentlySeen(device) ? 'Active' : 'Idle',
      h('span', { class: 'meta', text: device.last_seen_at ? `Last seen ${ago(device.last_seen_at)}` : 'Not seen since 2.9.0' }))),
    section('Connected', h('p', { class: 'val', text: fullDate(device.created_at) })),
    panel.confirm
      ? h('div', { class: 'confirm' }, h('p', { class: 'label', text: `Disconnect ${device.device}? Its agents stop reading and saving right away.` }),
        h('div', { class: 'actions' },
          h('button', { type: 'button', class: 'btn danger', onclick: () => disconnectDevice(device) }, 'Disconnect'),
          h('button', { type: 'button', class: 'quiet muted', onclick: () => { panel.confirm = false; render(); } }, 'Cancel')))
      : h('div', { class: 'actions' }, h('button', { type: 'button', class: 'quiet danger', onclick: () => { panel.confirm = true; render(); } }, 'Disconnect'))];
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
  const back = { memory: currentPlace(), device: 'Agents', startup: 'Settings' }[panel.type];
  return [h('div', { class: 'panel-top' },
    h('button', { type: 'button', class: 'quiet back', onclick: closePanel }, `← ${back}`),
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
    h('form', { class: 'signin-box', autocomplete: 'off', onsubmit: event => { event.preventDefault(); const input = event.target.querySelector('#token'); const token = input.value; input.value = ''; signIn(token); } },
      h('p', { class: 'brand', text: 'GRASSHOPPER' }),
      h('h1', { text: 'Sign in' }),
      state.route.connect ? h('p', { class: 'label', text: 'An agent is asking to connect. Sign in to check its code.' }) : null,
      state.signedOutNote ? h('p', { class: 'label muted', role: 'status', text: state.signedOutNote }) : null,
      h('label', { class: 'field' }, 'Access token', h('input', { id: 'token', type: 'password', autocomplete: 'off', spellcheck: 'false', 'aria-invalid': state.signInError ? 'true' : 'false', 'aria-describedby': state.signInError ? 'token-error' : null })),
      state.signInError ? h('p', { class: 'error', id: 'token-error', role: 'alert', text: state.signInError }) : null,
      h('div', { class: 'actions' }, h('button', { type: 'submit', class: 'btn', disabled: state.signingIn }, state.signingIn ? 'Signing in…' : 'Sign in')),
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

function render() {
  if (state.checking) return;
  // Keep typing in the edit form across redraws.
  if (state.panel?.mode === 'edit' && document.getElementById('edit-text')) {
    state.panel.draft = { title: document.getElementById('edit-title').value, content: document.getElementById('edit-text').value };
  }
  const focused = document.activeElement;
  const keyOf = node => node.getAttribute?.('data-key') || node.getAttribute?.('aria-label') || node.id || node.textContent;
  const focusKey = focused && focused !== document.body ? keyOf(focused) : '';
  document.title = state.active ? `${state.panel?.record?.title || currentPlace()} · Grasshopper` : 'Grasshopper';
  if (!state.active) { root.replaceChildren(signInView()); return; }
  // Scrolling areas keep their position across a redraw.
  const scrolled = ['side', 'panel'].map(name => [name, root.querySelector?.(`.${name}`)?.scrollTop || 0]);
  const narrow = Boolean(narrowLayout?.matches) && Boolean(state.panel);
  const panel = narrow ? null : panelView();
  const review = state.index?.review || 0;
  root.replaceChildren(...[h('div', { class: `app${panel ? ' has-panel' : ''}${state.menu ? ' menu-open' : ''}` },
    h('header', { class: 'topbar' }, h('span', { class: 'brand', text: 'GRASSHOPPER' }),
      h('button', { type: 'button', class: 'menu-btn', 'aria-expanded': String(state.menu), onclick: () => { state.menu = !state.menu; render(); } },
        `${currentPlace()} ${state.menu ? '▴' : '▾'}`, review ? h('span', { class: 'warn', text: ` · ${review} new` }) : null)),
    sidebar(),
    narrow
      ? h('main', { id: 'main', tabindex: '-1', class: 'panel panel-main', 'aria-label': state.panel.record?.title || 'Memory' }, panelBody(state.panel))
      : h('main', { id: 'main', tabindex: '-1', class: state.offline ? 'offline' : null }, h('div', { class: 'page' }, mainView())),
    panel),
  state.undo ? h('div', { class: 'undo', role: 'status' }, h('span', { text: `${state.undo.verb} “${state.undo.record.title}”` }), h('button', { type: 'button', class: 'quiet', onclick: undo }, 'Undo')) : null].filter(Boolean));
  for (const [name, top] of scrolled) { const node = root.querySelector?.(`.${name}`); if (node && top) node.scrollTop = top; }
  // A redraw must not strand keyboard focus on a removed control.
  if (focusKey) {
    const match = [...(root.querySelectorAll?.('button, input, textarea, select, a') || [])].find(node => keyOf(node) === focusKey);
    (match || document.getElementById('main'))?.focus?.();
  }
  // An approval link moves focus to its request once, not on every redraw.
  if (state.route.connect && !state.approvalFocused) {
    const request = document.getElementById('focused-request');
    if (request) { request.focus?.(); state.approvalFocused = true; }
  }
}

checkSession();

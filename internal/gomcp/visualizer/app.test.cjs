const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const test = require('node:test');
const vm = require('node:vm');

function view(hash = '', connected = true, sessionStatus = 200) {
  const elements = new Map();
  const element = () => ({
    value: '', textContent: '', open: false, children: [], listeners: {},
    addEventListener(name, action) { this.listeners[name] = action; },
    append(...items) { this.children.push(...items); },
    replaceChildren(...items) { this.children = items; },
    querySelector() { return null; },
    contains() { return false; },
    showModal() { this.open = true; }, close() { this.open = false; this.listeners.close?.(); },
    before(node) { node.placedBefore = this; },
    focus() { this.focused = true; }, scrollIntoView() {}, getAttribute(name) { return this.attributes?.[name]; }, setAttribute(name, value) { this.attributes = { ...this.attributes, [name]: value }; }, classList: { add() {}, toggle() {} }
  });
  const get = id => {
    if (!elements.has(id)) elements.set(id, element());
    return elements.get(id);
  };
  const requests = [];
  const windowListeners = {};
  const documentListeners = {};
  const timers = new Map();
  const timerDelays = new Map();
  let timerID = 0;
  const sandbox = {
    TextEncoder, URLSearchParams,
    history: { pushState(_state, _title, hash) { sandbox.location.hash = hash.startsWith('#') ? hash : ''; }, replaceState(_state, _title, hash) { sandbox.location.hash = hash.startsWith('#') ? hash : ''; } },
    document: { hidden: false, addEventListener(name, action) { documentListeners[name] = action; }, getElementById: get, querySelector: get, createElement: element, createDocumentFragment: element },
    window: { addEventListener(name, action) { windowListeners[name] = action; }, confirm: () => true, getSelection: () => null },
    location: { origin: 'http://127.0.0.1', hash },
    AbortController,
    setTimeout(fn, delay) { timers.set(++timerID, fn); timerDelays.set(timerID, delay); return timerID; },
    clearTimeout(id) { timers.delete(id); timerDelays.delete(id); },
    fetch(url, options) {
      if (url.endsWith('/session')) return Promise.resolve({ ok: sessionStatus < 400, status: sessionStatus, json: async () => ({ connected: false }) });
      return new Promise((resolve, reject) => requests.push({ url, options, reject, reply(data, status = 200) {
        resolve({ ok: status < 400, status, text: async () => typeof data === 'string' ? data : JSON.stringify(data), json: async () => data });
      } }));
    }
  };
  sandbox.window.history = sandbox.history;
  vm.createContext(sandbox);
  vm.runInContext(fs.readFileSync(path.join(__dirname, 'app.js'), 'utf8'), sandbox);
  const ui = vm.runInContext('({ refreshDevices, stop, setConnected, revokeDevice, refresh, draw, openMemory, loadRecord, closeMemory, restartMemoryView, chooseView, showViewTabs, updateViewCounts })', sandbox);
  if (connected) {
    ui.setConnected(true);
    get('device-panel').open = true;
  }
  return { ui, get, requests, timers, timerDelays, document: sandbox.document, location: sandbox.location, windowListeners, documentListeners };
}

function replyPair(v, start, devices = []) {
  v.requests[start].reply([]);
  v.requests[start + 1].reply(devices);
}

function deviceLabels(v) {
  return v.get('connected-devices').children.map(row => row.children[0].children[0].textContent);
}

test('newer device refresh wins and owns the only polling timer', async () => {
  const v = view();
  const older = v.ui.refreshDevices();
  const newer = v.ui.refreshDevices();
  replyPair(v, 2);
  await newer;
  replyPair(v, 0, [{ id: 1, device: 'revoked-device' }]);
  await older;
  assert.deepEqual(deviceLabels(v), ['No other devices connected.']);
  assert.equal(v.requests[0].options.signal.aborted, true);
  assert.equal(v.timers.size, 1);
});

function memory(id, revision = 1, content = 'Synthetic saved decision.') {
  return { id, revision, content, title: 'Synthetic decision', scope: {}, purpose: 'decision', confirmed: true, updated_at: '2026-09-29T10:00:00Z', provenance: { source: 'synthetic test', harness: 'test', device: 'test-device' } };
}

test('search submits the selected scope and runs once until explicit refresh', async () => {
  const v = view();
  v.get('project').value = 'id:test';
  v.get('search-query').value = '  orchard decision  ';
  v.get('search-form').listeners.submit({ preventDefault() {} });
  const request = v.requests[0];
  assert.equal(request.url, '/visualizer/api/search');
  assert.equal([...v.timerDelays.values()].includes(20000), true);
  assert.deepEqual(JSON.parse(request.options.body), { scope: { project: 'id:test' }, query: 'orchard decision' });
  request.reply({ records: [memory(1)], omitted: 0, devices: [], projects: ['id:test'], semantic_ready: false });
  await new Promise(setImmediate);
  assert.equal(v.get('status').textContent, 'Wording matches only · meaning search unavailable');
  assert.equal(v.get('clear-search').hidden, false);
  assert.equal(v.timers.size, 0);
});

test('changed scope rejects an old search response and clears visible content', async () => {
  const v = view();
  const older = v.ui.refresh();
  v.get('project').value = 'id:next';
  v.ui.restartMemoryView();
  v.requests[1].reply({ records: [memory(2)], omitted: 0, devices: [], projects: ['id:next'] });
  await new Promise(setImmediate);
  const current = v.get('records').children[0];
  v.requests[0].reply({ records: [memory(1)], omitted: 0 });
  await older;
  assert.equal(v.get('records').children[0], current);
  assert.equal(v.requests[0].options.signal.aborted, true);
  assert.equal(v.timers.size, 1);
});

test('full memory and earlier revision preserve complete text and source', async () => {
  const v = view();
  v.ui.openMemory(memory(2));
  const full = '<script>never execute this</script> ' + 'full text '.repeat(1000);
  v.requests[0].reply(memory(2, 2, full));
  await new Promise(setImmediate);
  assert.equal(v.get('memory-detail-content').textContent, full);
  assert.equal(v.get('memory-detail-meta').children.some(item => item.textContent === 'Source: synthetic test'), true);
  assert.equal(v.get('previous-revision').disabled, false);
  v.get('previous-revision').listeners.click();
  assert.equal(JSON.parse(v.requests[1].options.body).revision, 1);
  v.requests[1].reply(memory(2, 1, 'Earlier content.'));
  await new Promise(setImmediate);
  assert.equal(v.get('memory-detail-content').textContent, 'Earlier content.');
  assert.match(v.get('memory-revision').textContent, /Revision 1 of 2/);
  assert.equal(v.get('previous-revision').disabled, true);
  assert.equal(v.get('next-revision').disabled, false);
});

test('closing and opening another memory ignores both a late response and an old close event', async () => {
  const v = view();
  v.ui.openMemory(memory(1));
  v.ui.closeMemory();
  v.ui.openMemory(memory(2));
  v.get('memory-dialog').listeners.close();
  v.requests[1].reply(memory(2, 3, 'New selected content.'));
  await new Promise(setImmediate);
  v.requests[0].reply(memory(1, 1, 'Old selected content.'));
  await new Promise(setImmediate);
  assert.equal(v.get('memory-detail-content').textContent, 'New selected content.');
  assert.equal(v.requests[0].options.signal.aborted, true);
});

test('expired record request clears private content and disconnects', async () => {
  const v = view();
  v.ui.openMemory(memory(1));
  v.requests[0].reply(memory(1));
  await new Promise(setImmediate);
  const pending = v.ui.loadRecord();
  v.requests[1].reply({}, 401);
  await pending;
  assert.equal(v.get('memory-detail-content').textContent, '');
  assert.equal(v.get('memory-dialog').open, false);
  assert.equal(v.get('search-form').hidden, true);
  assert.equal(v.get('status').textContent, 'Session expired');
});

test('omitted records remain directly openable without replacing focused controls on unchanged polls', async () => {
  const v = view();
  const page = { records: [], omitted: 1, omitted_records: [{ id: 32, revision: 1, scope: {} }] };
  v.ui.draw(page);
  const links = v.get('omissions').children[1];
  v.ui.draw(page);
  assert.equal(v.get('omissions').children[1], links);
  links.children[0].listeners.click();
  assert.equal(JSON.parse(v.requests[0].options.body).id, 32);
  v.requests[0].reply(memory(32, 1, 'Large memory '.repeat(2000)));
  await new Promise(setImmediate);
  assert.equal(v.get('memory-detail-content').textContent.length, 26000);
});

test('closing and reopening the panel rejects responses from its previous lifetime', async () => {
  const v = view();
  const older = v.ui.refreshDevices();
  v.get('device-panel').open = false;
  v.get('device-panel').listeners.toggle();
  assert.equal(v.requests[0].options.signal.aborted, true);
  v.get('device-panel').open = true;
  const newer = v.ui.refreshDevices();
  replyPair(v, 2, [{ id: 2, device: 'current-device' }]);
  await newer;
  replyPair(v, 0, [{ id: 1, device: 'old-device' }]);
  await older;
  assert.deepEqual(deviceLabels(v), ['current-device']);
  assert.equal(v.timers.size, 1);
});

test('disconnect cancels device requests and leaves no polling timer', async () => {
  const v = view();
  const pending = v.ui.refreshDevices();
  v.ui.stop(true);
  replyPair(v, 0, [{ id: 1, device: 'old-device' }]);
  await pending;
  assert.deepEqual(deviceLabels(v), []);
  assert.equal(v.requests[0].options.signal.aborted, true);
  assert.equal(v.timers.size, 0);
});

test('a failed request cannot let its sibling repaint and polling can recover', async () => {
  const v = view();
  const failed = v.ui.refreshDevices();
  v.requests[0].reject(new Error('offline'));
  await failed;
  assert.equal(v.requests[1].options.signal.aborted, true);
  // The controlled fetch ignores abort so even a late sibling completion is
  // exercised. A real fetch would reject as soon as its signal is aborted.
  v.requests[1].reply([{ id: 1, device: 'old-device' }]);
  await new Promise(setImmediate);
  assert.deepEqual(deviceLabels(v), []);
  const retry = v.ui.refreshDevices();
  replyPair(v, 2, [{ id: 2, device: 'recovered-device' }]);
  await retry;
  assert.deepEqual(deviceLabels(v), ['recovered-device']);
  assert.equal(v.timers.size, 1);
});

test('the default view asks for every project and other choices narrow it', () => {
  const v = view();
  v.ui.refresh();
  assert.deepEqual(JSON.parse(v.requests[0].options.body), { all_projects: true });
  assert.equal(v.get('scope-summary').textContent, 'All projects · All devices · All platforms');
  v.get('project').value = '\u0000global';
  v.ui.restartMemoryView();
  assert.deepEqual(JSON.parse(v.requests[1].options.body), {});
  assert.equal(v.get('scope-summary').textContent, 'Global memories only · All devices · All platforms');
  v.get('project').value = 'git:github.com/example/orchard';
  v.ui.restartMemoryView();
  assert.deepEqual(JSON.parse(v.requests[2].options.body), { project: 'git:github.com/example/orchard' });
});

test('an empty store guides first use, and an empty filter says how to widen it', () => {
  const v = view();
  v.ui.draw({ records: [], omitted: 0 });
  const empty = v.get('records').children[0].children[0];
  assert.match(empty.children[0].textContent, /Nothing is saved yet/);
  assert.equal(empty.children[1].children.length, 4);
  v.get('project').value = 'id:quiet';
  v.ui.restartMemoryView();
  v.ui.draw({ records: [], omitted: 0 });
  assert.match(v.get('records').children[0].children[0].children[0].textContent, /Choose All projects/);
});

test('cards name their exact scope and say what an agent will load', () => {
  const v = view();
  const scoped = { ...memory(3), confirmed: false, scope: { project: 'git:github.com/example/orchard', device: 'mac-mini', platform: 'macos' } };
  const handoff = { ...memory(4), confirmed: false, purpose: 'handoff' };
  v.ui.draw({ records: [scoped, handoff], omitted: 0 });
  const [first, second] = v.get('records').children[0].children;
  assert.equal(first.children[0].children[1].textContent, 'Git project github.com/example/orchard · device mac-mini · macOS');
  assert.equal(first.children[0].children[2].textContent, '#3');
  assert.match(first.children[2].textContent, /^Unconfirmed · not loaded at agent startup/);
  assert.match(second.children[2].textContent, /^Unconfirmed handoff · loads at startup/);
  assert.equal(first.children[4].attributes['aria-label'], 'Open memory: Synthetic decision');
});

test('omitted records are named and a poll redraw keeps keyboard focus', () => {
  const v = view();
  v.ui.draw({ records: [memory(5)], omitted: 1, omitted_ids: [9], omitted_records: [{ id: 9, revision: 1, scope: {} }], omitted_titles: { 9: 'Large lesson' } });
  assert.equal(v.get('omissions').children[1].children[0].textContent, 'Open “Large lesson” (#9)');
  v.document.activeElement = v.get('records').children[0].children[0].children[4];
  const before = v.document.activeElement;
  v.ui.draw({ records: [memory(5, 2)], omitted: 0 });
  const after = v.get('records').children[0].children[0].children[4];
  assert.notEqual(after, before);
  assert.equal(after.focused, true);
});

test('a truncated list preview ends with an ellipsis and short text does not', () => {
  const v = view();
  v.ui.draw({ records: [{ ...memory(6), content: 'x'.repeat(240), content_truncated: true }, memory(7)], omitted: 0 });
  const [long, short] = v.get('records').children[0].children;
  assert.equal(long.children[3].textContent, 'x'.repeat(240) + '…');
  assert.equal(short.children[3].textContent, 'Synthetic saved decision.');
});

test('a changed review poll retains the focused inline action and removal finds a surviving memory', async () => {
  const v = view();
  v.ui.chooseView('review');
  v.requests.at(-1).reply({ records: [memory(1), memory(2)] });
  await tick();
  const first = v.get('records').children[0].children[0];
  v.document.activeElement = first.children.at(-1).children[0];
  v.ui.draw({ records: [memory(1, 2), memory(2)] });
  const replacement = v.get('records').children[0].children[0].children.at(-1).children[0];
  assert.equal(replacement.focused, true);
  v.document.activeElement = replacement;
  v.ui.draw({ records: [memory(2)] });
  assert.equal(v.get('records').children[0].children[0].children[4].focused, true);
});

test('search explanation consumes space only when there is a query', async () => {
  const v = view();
  const refresh = v.ui.refresh();
  v.requests.at(-1).reply({ records: [memory(1)] });
  await refresh;
  assert.equal(v.get('search-hint').hidden, true);
  v.get('search-query').value = 'decision';
  v.get('search-form').listeners.submit({ preventDefault() {} });
  v.requests.at(-1).reply({ records: [memory(1)], semantic_ready: false });
  await tick();
  assert.equal(v.get('search-hint').hidden, false);
});

const tick = () => new Promise(setImmediate);

async function openLatest(v, record) {
  v.ui.openMemory(record);
  v.requests.at(-1).reply(record);
  await tick();
}

const withoutRequestID = ({ request_id, ...rest }) => rest;
const submitEdit = v => v.get('edit-form').listeners.submit({ preventDefault() {} });

test('latest actions and earlier revision restore follow the memory state', async () => {
  const v = view();
  await openLatest(v, { ...memory(1, 2), confirmed: false });
  assert.equal(v.get('memory-actions').hidden, false);
  assert.equal(v.get('confirm-memory').hidden, false);
  assert.equal(v.get('archive-memory').hidden, false);
  assert.equal(v.get('restore-memory').hidden, true);
  v.get('previous-revision').listeners.click();
  v.requests.at(-1).reply(memory(1, 1));
  await tick();
  assert.equal(v.get('memory-actions').hidden, false);
  assert.equal(v.get('edit-memory').hidden, true);
  assert.equal(v.get('confirm-memory').hidden, true);
  assert.equal(v.get('archive-memory').hidden, true);
  assert.equal(v.get('restore-revision').hidden, false);
  v.get('latest-revision').listeners.click();
  v.requests.at(-1).reply({ ...memory(1, 2), confirmed: true });
  await tick();
  assert.equal(v.get('confirm-memory').hidden, true);
  await openLatest(v, { ...memory(2, 3), archived: true });
  assert.equal(v.get('restore-memory').hidden, false);
  assert.equal(v.get('edit-memory').hidden, true);
  assert.equal(v.get('archive-memory').hidden, true);
});

test('saving an edit sends the expected revision, confirms, and a retry cannot save twice', async () => {
  const v = view();
  await openLatest(v, { ...memory(7, 2), confirmed: false, purpose: 'observation' });
  v.get('edit-memory').listeners.click();
  assert.equal(v.get('edit-form').hidden, false);
  assert.equal(v.get('edit-content').value, 'Synthetic saved decision.');
  v.get('edit-content').value = 'Corrected decision.';
  v.get('edit-purpose').value = 'decision';
  submitEdit(v);
  const first = v.requests.at(-1);
  assert.equal(first.url, '/visualizer/api/update');
  const body = JSON.parse(first.options.body);
  assert.deepEqual(withoutRequestID(body), { id: 7, expected_revision: 2, title: 'Synthetic decision', content: 'Corrected decision.', purpose: 'decision' });
  assert.ok(body.request_id.length > 8);
  first.reply(null, 503);
  await tick();
  assert.match(v.get('memory-detail-status').textContent, /Your text is still here/);
  assert.equal(v.get('edit-form').hidden, false);
  submitEdit(v);
  const retry = v.requests.at(-1);
  assert.equal(JSON.parse(retry.options.body).request_id, body.request_id);
  retry.reply({ id: 7, revision: 3, deduplicated: false });
  await tick();
  v.requests.at(-1).reply({ ...memory(7, 3, 'Corrected decision.'), confirmed: true });
  await tick();
  assert.equal(v.get('edit-form').hidden, true);
  assert.equal(v.get('memory-detail-content').textContent, 'Corrected decision.');
  assert.match(v.get('memory-detail-status').textContent, /Saved as revision 3 and confirmed/);
  assert.equal(v.requests.some(request => request.url === '/visualizer/api/context'), true);
});

test('a different edit after a failed save gets its own request ID', async () => {
  const v = view();
  await openLatest(v, memory(8, 1));
  v.get('edit-memory').listeners.click();
  v.get('edit-content').value = 'First draft.';
  submitEdit(v);
  const first = JSON.parse(v.requests.at(-1).options.body).request_id;
  v.requests.at(-1).reply(null, 503);
  await tick();
  v.get('edit-content').value = 'Second draft.';
  submitEdit(v);
  assert.notEqual(JSON.parse(v.requests.at(-1).options.body).request_id, first);
});

test('a conflicting save keeps the draft, shows the newer text, and saves against the new revision', async () => {
  const v = view();
  await openLatest(v, memory(9, 1));
  v.get('edit-memory').listeners.click();
  v.get('edit-content').value = 'My draft.';
  submitEdit(v);
  v.requests.at(-1).reply({ error: 'revision_conflict', current: { ...memory(9, 2, 'Agent text.'), title: 'Synthetic decision' } }, 409);
  await tick();
  assert.equal(v.get('conflict').hidden, false);
  assert.equal(v.get('conflict-text').textContent, 'Agent text.');
  assert.match(v.get('conflict-note').textContent, /revision 2/);
  assert.equal(v.get('edit-content').value, 'My draft.');
  submitEdit(v);
  assert.equal(JSON.parse(v.requests.at(-1).options.body).expected_revision, 2);
});

test('confirm as is resends the existing text, and archive and restore change visibility', async () => {
  const v = view();
  await openLatest(v, { ...memory(10, 1), confirmed: false });
  v.get('confirm-memory').listeners.click();
  let request = v.requests.at(-1);
  assert.deepEqual(withoutRequestID(JSON.parse(request.options.body)), { id: 10, expected_revision: 1, title: 'Synthetic decision', content: 'Synthetic saved decision.', purpose: 'decision' });
  request.reply({ id: 10, revision: 2, deduplicated: false });
  await tick();
  v.requests.at(-1).reply({ ...memory(10, 2), confirmed: true });
  await tick();
  assert.match(v.get('memory-detail-status').textContent, /Confirmed as revision 2/);
  v.get('archive-memory').listeners.click();
  request = v.requests.at(-1);
  assert.equal(request.url, '/visualizer/api/archive');
  assert.deepEqual(withoutRequestID(JSON.parse(request.options.body)), { id: 10, expected_revision: 2, archived: true });
  request.reply({ id: 10, revision: 3, deduplicated: false });
  await tick();
  assert.equal(v.get('memory-dialog').open, false);
  assert.match(v.get('status').textContent, /Archived “Synthetic decision”/);
});

test('an expired session during a save clears private content', async () => {
  const v = view();
  await openLatest(v, memory(11, 1));
  v.get('edit-memory').listeners.click();
  v.get('edit-content').value = 'Draft.';
  submitEdit(v);
  v.requests.at(-1).reply(null, 401);
  await tick();
  assert.equal(v.get('status').textContent, 'Session expired');
  assert.equal(v.get('memory-dialog').open, false);
});

test('choosing a list view asks the server for it and hides search', async () => {
  const v = view();
  v.ui.chooseView('review');
  assert.deepEqual(JSON.parse(v.requests.at(-1).options.body), { all_projects: true, view: 'review' });
  assert.equal(v.get('search-form').hidden, true);
  v.requests.at(-1).reply({ records: [], omitted: 0, devices: [], projects: [], review_count: 0, archived_count: 2 });
  await tick();
  assert.match(v.get('records').children[0].children[0].children[0].textContent, /Nothing needs review/);
  v.ui.chooseView('');
  assert.equal(v.get('search-form').hidden, false);
  assert.deepEqual(JSON.parse(v.requests.at(-1).options.body), { all_projects: true });
});

const request64 = 'a'.repeat(64);

test('a pending request shows a prominent code, time left, and a primary Approve', async () => {
  const v = view();
  const refreshed = v.ui.refreshDevices();
  v.requests[0].reply([{ request_id: request64, code: 'c0de42', device: 'work-hp', status: 'pending', expires_in: 272 }]);
  v.requests[1].reply([]);
  await refreshed;
  const row = v.get('pending-devices').children[0];
  const [approve, deny] = row.children[1].children;
  assert.equal(row.children[0].children[0].textContent, 'work-hp wants to connect');
  const detail = row.children[0].children[1];
  assert.equal(detail.children[0], 'Code ');
  assert.equal(detail.children[1].textContent, 'c0de42');
  assert.equal(detail.children[2].textContent, ' · Expires in 5 min');
  assert.equal(approve.className, '');
  assert.equal(deny.className, 'quiet');
  approve.listeners.click();
  assert.deepEqual(JSON.parse(v.requests[2].options.body), { request_id: request64, code: 'c0de42', decision: 'approve' });
});

test('a poll updates the countdown in place and keeps unchanged rows for keyboard users', async () => {
  const v = view();
  const first = v.ui.refreshDevices();
  v.requests[0].reply([{ request_id: request64, code: 'c0de42', device: 'work-hp', status: 'pending', expires_in: 100 }]);
  v.requests[1].reply([{ id: 1, device: 'mac-mini', created_at: '2026-09-29T10:00:00Z' }]);
  await first;
  const pendingRow = v.get('pending-devices').children[0];
  const connectedRow = v.get('connected-devices').children[0];
  const second = v.ui.refreshDevices();
  v.requests[2].reply([{ request_id: request64, code: 'c0de42', device: 'work-hp', status: 'pending', expires_in: 40 }]);
  v.requests[3].reply([{ id: 1, device: 'mac-mini', created_at: '2026-09-29T10:00:00Z' }]);
  await second;
  assert.equal(v.get('pending-devices').children[0], pendingRow);
  assert.equal(v.get('connected-devices').children[0], connectedRow);
  assert.equal(pendingRow.children[0].children[1].children[2].textContent, ' · Expires in 40 s');
  assert.match(connectedRow.children[0].children[1].children[0], /Connected/);
});

test('disconnecting asks in the page, survives a poll, and can be cancelled', async () => {
  const v = view();
  let asked = false;
  const first = v.ui.refreshDevices();
  v.requests[0].reply([]);
  v.requests[1].reply([{ id: 4, device: 'old-laptop', created_at: '2026-09-01T10:00:00Z' }]);
  await first;
  v.get('connected-devices').children[0].children[1].children[0].listeners.click();
  const ask = v.requests.length;
  v.requests[ask - 2].reply([]);
  v.requests[ask - 1].reply([{ id: 4, device: 'old-laptop', created_at: '2026-09-01T10:00:00Z' }]);
  await new Promise(setImmediate);
  let row = v.get('connected-devices').children[0];
  assert.equal(row.children[0].children[0].textContent, 'Disconnect old-laptop?');
  assert.deepEqual(row.children[1].children.map(button => button.textContent), ['Disconnect now', 'Keep']);
  assert.equal(asked, false);
  const next = v.ui.refreshDevices();
  v.requests.at(-2).reply([]);
  v.requests.at(-1).reply([{ id: 4, device: 'old-laptop', created_at: '2026-09-01T10:00:00Z' }]);
  await next;
  assert.equal(v.get('connected-devices').children[0], row);
  row.children[1].children[0].listeners.click();
  const removal = v.requests.at(-1);
  assert.equal(removal.options.method, 'DELETE');
  assert.deepEqual(JSON.parse(removal.options.body), { id: 4 });
});

test('opening an approval link while signed out offers sign-in and returns to the request', async () => {
  const v = view('#connect=' + request64, false);
  await tick();
  assert.notEqual(v.get('token-field').hidden, true);
  assert.notEqual(v.get('connect').hidden, true);
  assert.equal(v.get('approval-notice').hidden, false);
  assert.match(v.get('approval-notice').textContent, /Sign in as the owner/);
  v.get('token').value = 'synthetic-owner-token-0123456789-abcdef';
  v.get('connection-form').listeners.submit({ preventDefault() {} });
  await tick();
  await tick();
  assert.equal(v.get('approval-notice').hidden, true);
  assert.equal(v.get('device-panel').open, true);
});

test('the server line reports version, search model and counts', () => {
  const v = view();
  v.ui.stop(true);
  v.ui.setConnected(true);
  v.ui.refresh();
  v.requests.at(-1).reply({ records: [], omitted: 0, devices: [], projects: [], server: { version: '2.7.0', model: 'granite-test', memories: 24, archived: 3 } });
  return tick().then(() => {
    assert.equal(v.get('server-status').textContent, 'Server 2.7.0 · meaning search granite-test · 24 memories, 3 archived');
    v.ui.refresh();
    v.requests.at(-1).reply({ records: [], omitted: 0, devices: [], projects: [], server: { version: '2.7.0', model: '', memories: 1, archived: 0 } });
    return tick();
  }).then(() => {
    assert.equal(v.get('server-status').textContent, 'Server 2.7.0 · wording search only · 1 memory, 0 archived');
    assert.equal(v.get('server-status').hidden, false);
  });
});

test('the startup preview asks for one exact scope and explains what is left out', async () => {
  const v = view();
  v.get('project').value = 'git:github.com/example/orchard';
  v.get('startup-agent').value = '3000';
  v.ui.chooseView('startup');
  const request = v.requests.at(-1);
  assert.equal(request.url, '/visualizer/api/startup');
  assert.deepEqual(JSON.parse(request.options.body), { scope: { project: 'git:github.com/example/orchard' }, budget: 3000 });
  assert.equal(v.get('startup-controls').hidden, false);
  assert.equal(v.get('search-form').hidden, true);
  request.reply({ budget: 3000, records: [memory(1)], not_loaded: [
    { id: 2, revision: 1, scope: {}, title: 'Maybe weekly', purpose: 'observation', reason: 'unconfirmed' },
    { id: 3, revision: 1, scope: { project: 'git:github.com/example/orchard' }, title: 'Old handoff', purpose: 'handoff', reason: 'older_handoff' },
    { id: 4, revision: 1, scope: {}, title: 'Heavy lesson', purpose: 'lesson', reason: 'over_budget' }
  ] });
  await tick();
  assert.equal(v.get('summary').textContent, '1 memory loads at startup within 3,000 bytes · 3 not loaded');
  const rows = v.get('not-loaded').children[1].children;
  assert.equal(v.get('not-loaded').hidden, false);
  assert.equal(rows[0].children[0].children[0].textContent, 'Maybe weekly (#2)');
  assert.match(rows[0].children[0].children[1].textContent, /^Not confirmed\. Confirm it to load at startup\./);
  assert.match(rows[1].children[0].children[1].textContent, /^An older handoff\. Only the latest handoff loads\./);
  assert.match(rows[2].children[0].children[1].textContent, /^Over the startup size budget\./);
  v.ui.chooseView('');
  assert.equal(v.get('not-loaded').hidden, true);
  assert.equal(v.get('startup-controls').hidden, true);
});

test('the startup preview never widens an unchosen project to every project', () => {
  const v = view();
  v.ui.chooseView('startup');
  assert.deepEqual(JSON.parse(v.requests.at(-1).options.body), { scope: {}, budget: 12000 });
  assert.match(v.get('scope-summary').textContent, /^No project chosen · global memories only/);
});

test('scope labels keep Git projects and project IDs apart', () => {
  const v = view();
  v.ui.draw({ records: [
    { ...memory(20), scope: { project: 'git:github.com/example/orchard' } },
    { ...memory(21), scope: { project: 'id:github.com/example/orchard' } }
  ], omitted: 0 });
  const [git, id] = v.get('records').children[0].children;
  assert.equal(git.children[0].children[1].textContent, 'Git project github.com/example/orchard');
  assert.equal(id.children[0].children[1].textContent, 'Project ID github.com/example/orchard');
});

test('an omission list capped at 100 says how many memories it does not name', () => {
  const v = view();
  const named = Array.from({ length: 100 }, (_, index) => ({ id: index + 1, revision: 1, scope: {} }));
  v.ui.draw({ records: [], omitted: 130, omitted_ids: named.map(item => item.id), omitted_records: named });
  const text = v.get('omissions').children[0].textContent;
  assert.match(text, /130 memories are outside/);
  assert.match(text, /first 100 are named below.*the other 30/);
});

test('closing the dialog mid-save keeps the write lock until that request ends', async () => {
  const v = view();
  await openLatest(v, memory(30, 1));
  v.get('edit-memory').listeners.click();
  v.get('edit-content').value = 'First memory edit.';
  submitEdit(v);
  const first = v.requests.at(-1);
  v.ui.closeMemory(true);
  await openLatest(v, memory(31, 1));
  assert.equal(v.get('edit-memory').disabled, true);
  v.get('edit-memory').listeners.click();
  v.get('edit-content').value = 'Second memory edit.';
  const before = v.requests.length;
  submitEdit(v);
  assert.equal(v.requests.length, before);
  assert.match(v.get('memory-detail-status').textContent, /still finishing/);
  first.reply({ id: 30, revision: 2, deduplicated: false });
  await tick();
  assert.equal(v.get('edit-memory').disabled, false);
  assert.equal(v.requests.some(request => request.url === '/visualizer/api/context'), true);
  submitEdit(v);
  assert.equal(JSON.parse(v.requests.at(-1).options.body).id, 31);
});

test('a conflict on confirm reloads the latest memory instead of an edit form', async () => {
  const v = view();
  await openLatest(v, { ...memory(32, 1), confirmed: false });
  v.get('confirm-memory').listeners.click();
  v.requests.at(-1).reply({ error: 'revision_conflict', current: { ...memory(32, 2, 'Agent text.'), confirmed: false } }, 409);
  await tick();
  assert.equal(v.get('conflict').hidden, true);
  assert.equal(v.get('edit-form').hidden, true);
  const reload = v.requests.at(-1);
  assert.equal(reload.url, '/visualizer/api/record');
  reload.reply({ ...memory(32, 2, 'Agent text.'), confirmed: false });
  await tick();
  assert.equal(v.get('memory-detail-content').textContent, 'Agent text.');
  assert.match(v.get('memory-detail-status').textContent, /changed while you were looking/);
  assert.equal(v.get('memory-actions').hidden, false);
});

test('cancelling after a conflict shows the newer text, not the stale page', async () => {
  const v = view();
  await openLatest(v, memory(33, 1, 'Original.'));
  v.get('edit-memory').listeners.click();
  v.get('edit-content').value = 'Draft.';
  submitEdit(v);
  v.requests.at(-1).reply({ error: 'revision_conflict', current: memory(33, 2, 'Agent text.') }, 409);
  await tick();
  v.get('cancel-edit').listeners.click();
  v.get('discard-draft').listeners.click();
  const reload = v.requests.at(-1);
  assert.equal(reload.url, '/visualizer/api/record');
  reload.reply(memory(33, 2, 'Agent text.'));
  await tick();
  assert.equal(v.get('memory-detail-content').textContent, 'Agent text.');
  assert.equal(v.get('edit-form').hidden, true);
});

test('view tabs show zero counts but no number before a count is known', () => {
  const v = view();
  const tabs = ['', 'review', 'archived'].map(name => ({ dataset: { view: name }, textContent: '', setAttribute() {}, addEventListener() {} }));
  v.get('view-tabs').children = tabs;
  v.ui.showViewTabs();
  assert.deepEqual(tabs.map(tab => tab.textContent), ['Saved', 'Needs review', 'Archived']);
  v.ui.updateViewCounts({ review_count: 0, archived_count: 3 });
  assert.deepEqual(tabs.map(tab => tab.textContent), ['Saved', 'Needs review (0)', 'Archived (3)']);
});

test('the device panel returns to its place when the approval fragment is cleared', () => {
  const v = view('#connect=' + request64);
  assert.equal(v.get('device-panel').placedBefore, v.get('memory-section'));
  v.location.hash = '';
  v.windowListeners.hashchange();
  assert.equal(v.get('device-panel').placedBefore, v.get('server-status'));
});


test('failed live polls recover automatically with one backed-off polling loop', async () => {
  const v = view();
  await tick();
  const first = v.ui.refresh();
  v.requests.at(-1).reply({ records: [memory(1)], omitted: 0 });
  await first;
  for (const delay of [3000, 10000, 30000]) {
    const failed = v.ui.refresh();
    v.requests.at(-1).reply(null, 503);
    await failed;
    assert.equal(v.get('status').textContent, 'Service unavailable');
    assert.deepEqual([...v.timerDelays.values()], [delay]);
  }
  const callback = [...v.timers.values()][0];
  const recovered = callback();
  v.requests.at(-1).reply({ records: [memory(1)], omitted: 0 });
  await recovered;
  assert.equal(v.get('status').textContent, 'Live');
  assert.deepEqual([...v.timerDelays.values()], [3000]);
});

test('hidden pages pause memory and device polling and refresh once on return', async () => {
  const v = view();
  await tick();
  const first = v.ui.refresh();
  v.requests.at(-1).reply({ records: [], omitted: 0 });
  await first;
  v.document.hidden = true;
  v.documentListeners.visibilitychange();
  assert.equal(v.timers.size, 0);
  const count = v.requests.length;
  await v.ui.refresh();
  await v.ui.refreshDevices();
  assert.equal(v.requests.length, count);
  v.document.hidden = false;
  v.documentListeners.visibilitychange();
  assert.equal(v.requests.length, count + 3);
  v.documentListeners.visibilitychange();
  assert.equal(v.requests.length, count + 3);
});

test('edit limits count UTF-8 bytes and prevent oversized saves', async () => {
  const v = view();
  await openLatest(v, memory(41));
  v.get('edit-memory').listeners.click();
  v.get('edit-content').value = 'é'.repeat(16385);
  v.get('edit-content').listeners.input();
  assert.equal(v.get('edit-content-count').textContent, '32,770 / 32,768 bytes');
  assert.equal(v.get('save-edit').disabled, true);
  const count = v.requests.length;
  await submitEdit(v);
  assert.equal(v.requests.length, count);
  v.get('edit-content').value = 'Valid text.';
  v.get('edit-title').value = 'é'.repeat(257);
  v.get('edit-title').listeners.input();
  assert.equal(v.get('edit-title-count').textContent, '514 / 512 bytes');
  assert.equal(v.get('save-edit').disabled, true);
  v.get('edit-title').value = 'é'.repeat(256);
  v.get('edit-title').listeners.input();
  assert.equal(v.get('save-edit').disabled, false);
});

for (const [reason, text] of [
  ['content must be 1-32768 bytes', /Text must contain 1 to 32,768 bytes/],
  ['metadata too large', /Title or tags are too long/],
  ['invalid purpose', /Choose a valid memory kind/]
]) test(`save explains ${reason}`, async () => {
  const v = view();
  await openLatest(v, memory(42));
  v.get('edit-memory').listeners.click();
  v.get('edit-content').value = 'Draft.';
  const saved = submitEdit(v);
  v.requests.at(-1).reply(reason + '\n', 400);
  await saved;
  assert.match(v.get('memory-detail-status').textContent, text);
});

test('search highlights literal text safely and suggests all projects only when filtered', async () => {
  const v = view();
  v.get('search-query').value = 'orchard <script>';
  v.get('search-form').listeners.submit({ preventDefault() {} });
  v.requests.at(-1).reply({ records: [memory(44, 1, 'Orchard <script> trees.')], omitted: 0, semantic_ready: false });
  await tick();
  const content = v.get('records').children[0].children[0].children[3];
  assert.deepEqual(content.children.filter(part => typeof part !== 'string').map(part => part.textContent), ['Orchard', '<script>']);
  v.ui.draw({ records: [], omitted: 0 });
  let text = v.get('records').children[0].children[0].children[0].textContent;
  assert.doesNotMatch(text, /search all projects/);
  v.get('project').value = 'id:filtered';
  v.ui.restartMemoryView();
  v.requests.at(-1).reply({ records: [], omitted: 0 });
  await tick();
  text = v.get('records').children[0].children[0].children[0].textContent;
  assert.match(text, /search all projects/);
});

test('signed-in connection section is hidden and dialog heading receives focus', async () => {
  const v = view();
  assert.equal(v.get('connection-section').hidden, true);
  await openLatest(v, memory(45));
  assert.equal(v.get('memory-detail-title').focused, true);
  assert.doesNotMatch(v.get('memory-detail-meta').children[2].textContent, /:\d{2}:\d{2}/);
  v.ui.stop();
  assert.equal(v.get('connection-section').hidden, false);
});

test('long lists show eight memories first and reveal more without a fetch', () => {
  const v = view();
  const page = { records: Array.from({ length: 32 }, (_, i) => memory(i + 1)) };
  v.ui.draw(page);
  assert.equal(v.get('records').children[0].children.length, 8);
  assert.equal(v.get('show-more').hidden, false);
  v.get('show-more').listeners.click();
  assert.equal(v.get('records').children[0].children.length, 16);
  assert.equal(v.requests.length, 0);
  v.ui.draw(page);
  assert.equal(v.get('records').children[0].children.length, 16);
});

test('masthead devices link opens the connection panel', () => {
  const v = view();
  v.get('device-panel').open = false;
  v.get('devices-link').listeners.click({ preventDefault() {} });
  assert.equal(v.get('device-panel').open, true);
});

test('wrong token reports at the field and returns focus without a contradictory summary', async () => {
  const v = view('', false, 401);
  await tick();
  v.get('token').value = 'synthetic-wrong-token';
  await v.get('connection-form').listeners.submit({ preventDefault() {} });
  assert.match(v.get('token-error').textContent, /owner access token/);
  assert.equal(v.get('token-error').hidden, false);
  assert.equal(v.get('token').focused, true);
  assert.equal(v.get('token').attributes['aria-invalid'], 'true');
  assert.equal(v.get('summary').textContent, 'Sign-in failed. Check the token field above.');
});


function inlineActions(v, record) {
  v.ui.chooseView('review');
  v.requests.at(-1).reply({ records: [record] });
  return tick().then(() => v.get('records').children[0].children[0].children.at(-1));
}

test('inline confirm reads full content, uses expected revision, and replays uncertain writes', async () => {
  const v = view();
  const reference = { ...memory(21, 4, 'Short preview…'), confirmed: false, content_truncated: true };
  const actions = await inlineActions(v, reference);
  actions.children[0].listeners.click();
  const read = v.requests.at(-1);
  assert.equal(read.url, '/visualizer/api/record');
  const full = memory(21, 4, 'Full synthetic memory. '.repeat(800));
  full.confirmed = false;
  read.reply(full);
  await tick();
  const write = v.requests.at(-1);
  const body = JSON.parse(write.options.body);
  assert.equal(write.url, '/visualizer/api/update');
  assert.equal(body.content, full.content);
  assert.equal(body.expected_revision, 4);
  write.reject(new TypeError('synthetic network failure'));
  await tick();
  actions.children[0].listeners.click();
  const retry = v.requests.at(-1);
  assert.equal(retry.url, '/visualizer/api/update');
  assert.equal(retry.options.body, write.options.body);
  retry.reply({ revision: 5 });
  await tick();
  assert.equal(v.requests.at(-1).url, '/visualizer/api/context');
});

test('inline review refuses a concurrently changed memory and refreshes the list', async () => {
  const v = view();
  const actions = await inlineActions(v, { ...memory(22), confirmed: false });
  actions.children[0].listeners.click();
  v.requests.at(-1).reply(memory(22, 2, 'Concurrent correction.'));
  await tick();
  assert.equal(v.requests.some(request => request.url.endsWith('/update')), false);
  assert.match(v.get('status').textContent, /Review the latest text/);
  assert.equal(v.requests.at(-1).url, '/visualizer/api/context');
});

test('inline archive posts the existing revision and conflict never overwrites', async () => {
  const v = view();
  const actions = await inlineActions(v, { ...memory(23), confirmed: false });
  actions.children[1].listeners.click();
  v.requests.at(-1).reply({ ...memory(23), confirmed: false });
  await tick();
  const write = v.requests.at(-1);
  assert.equal(write.url, '/visualizer/api/archive');
  assert.deepEqual(withoutRequestID(JSON.parse(write.options.body)), { id: 23, expected_revision: 1, archived: true });
  write.reply({ current: memory(23, 2) }, 409);
  await tick();
  assert.match(v.get('status').textContent, /Review the latest text/);
  assert.equal(v.requests.at(-1).url, '/visualizer/api/context');
});


test('a poll redraw during inline review enables the current row after failure', async () => {
  const v = view();
  const record = { ...memory(24), confirmed: false };
  const actions = await inlineActions(v, record);
  actions.children[0].listeners.click();
  v.requests.at(-1).reply(record);
  await tick();
  const write = v.requests.at(-1);
  v.ui.draw({ records: [{ ...record, title: 'Changed list title' }] });
  const current = v.get('records').children[0].children[0].children.at(-1);
  assert.notEqual(current, actions);
  assert.equal(current.children[0].disabled, true);
  write.reject(new TypeError('synthetic connection failure'));
  await tick();
  assert.equal(current.children[0].disabled, false);
  assert.equal(current.children[1].disabled, false);
});

test('an inline read from a former session cannot write into a new sign-in', async () => {
  const v = view();
  const record = { ...memory(25), confirmed: false };
  const actions = await inlineActions(v, record);
  actions.children[0].listeners.click();
  const read = v.requests.at(-1);
  v.ui.stop(true);
  v.ui.setConnected(true);
  const count = v.requests.length;
  read.reply(record);
  await tick();
  assert.equal(v.requests.length, count);
  assert.equal(v.requests.some(request => request.url.endsWith('/update')), false);
});


for (const action of ['Close', 'Escape', 'backdrop', 'Cancel', 'another memory', 'tab', 'filter']) test(`dirty edits survive ${action} until Discard`, async () => {
  const v = view();
  await openLatest(v, memory(51));
  v.get('edit-memory').listeners.click();
  v.get('edit-content').value = 'Unsaved correction.';
  const leave = () => {
    if (action === 'Close') v.get('close-memory').listeners.click();
    if (action === 'Escape') v.get('memory-dialog').listeners.cancel({ preventDefault() {} });
    if (action === 'backdrop') v.get('memory-dialog').listeners.click({ target: v.get('memory-dialog'), clientX: -1, clientY: -1 });
    if (action === 'Cancel') v.get('cancel-edit').listeners.click();
    if (action === 'another memory') v.ui.openMemory(memory(52));
    if (action === 'tab') v.ui.chooseView('review');
    if (action === 'filter') { v.get('project').value = 'id:next'; v.get('project').listeners.change(); }
  };
  leave();
  assert.equal(v.get('discard-changes').hidden, false);
  assert.equal(v.get('memory-dialog').open, true);
  assert.equal(v.get('edit-content').value, 'Unsaved correction.');
  v.get('keep-editing').listeners.click();
  assert.equal(v.get('discard-changes').hidden, true);
  assert.equal(v.get('edit-content').value, 'Unsaved correction.');
  leave();
  v.get('discard-draft').listeners.click();
  assert.equal(v.get('discard-changes').hidden, true);
  if (action === 'another memory') assert.equal(JSON.parse(v.requests.at(-1).options.body).id, 52);
  else if (action === 'tab') assert.equal(JSON.parse(v.requests.at(-1).options.body).view, 'review');
  else if (action === 'filter') assert.equal(JSON.parse(v.requests.at(-1).options.body).project, 'id:next');
  else if (action === 'Cancel') assert.equal(v.get('edit-form').hidden, true);
  else assert.equal(v.get('memory-dialog').open, false);
});

test('URL restores tab filters query and safe memory links before sign-in', async () => {
  const hash = '#view=review&project=id%3Aorchard&device=laptop&platform=windows&memory=56&token=never-keep';
  const v = view(hash, false);
  await tick();
  assert.equal(v.get('project').value, 'id:orchard');
  assert.equal(v.get('device').value, 'laptop');
  assert.equal(v.get('platform').value, 'windows');
  v.get('token').value = 'synthetic-owner-token-0123456789-abcdef';
  await v.get('connection-form').listeners.submit({ preventDefault() {} });
  assert.equal(v.requests.some(request => request.url === '/visualizer/api/record' && JSON.parse(request.options.body).id === 56), true);
  assert.equal(JSON.parse(v.requests.find(request => request.url === '/visualizer/api/context').options.body).view, 'review');
  assert.doesNotMatch(v.location.hash, /token|never-keep/);
  v.ui.closeMemory();
  v.location.hash = '#query=orchard&project=id%3Aorchard';
  v.windowListeners.hashchange();
  assert.equal(v.get('search-query').value, 'orchard');
  assert.equal(v.requests.at(-1).url, '/visualizer/api/search');
  v.location.hash = '#view=archived';
  v.windowListeners.hashchange();
  assert.equal(JSON.parse(v.requests.at(-1).options.body).view, 'archived');
});

test('restoring a revision posts its ID against the latest revision and retries safely', async () => {
  const v = view();
  await openLatest(v, memory(61, 2, 'New text.'));
  v.get('previous-revision').listeners.click();
  v.requests.at(-1).reply(memory(61, 1, 'Earlier text.'));
  await tick();
  assert.equal(v.get('restore-revision').hidden, false);
  v.get('restore-revision').listeners.click();
  const first = v.requests.at(-1);
  const body = JSON.parse(first.options.body);
  assert.deepEqual(withoutRequestID(body), { id: 61, expected_revision: 2, restore_revision: 1 });
  first.reply(null, 503);
  await tick();
  v.get('restore-revision').listeners.click();
  assert.deepEqual(JSON.parse(v.requests.at(-1).options.body), body);
  v.requests.at(-1).reply({ id: 61, revision: 3 });
  await tick();
  v.requests.at(-1).reply(memory(61, 3, 'Earlier text.'));
  await tick();
  assert.match(v.get('memory-detail-status').textContent, /Restored revision 1 as revision 3 and confirmed/);
});

test('reload keeps review scope and approval links can coexist with memory state', async () => {
  const v = view();
  v.get('project').value = 'id:orchard';
  v.get('device').value = 'laptop';
  v.get('platform').value = 'macos';
  v.ui.chooseView('review');
  const reloaded = view(v.location.hash);
  reloaded.ui.refresh();
  assert.deepEqual(JSON.parse(reloaded.requests.at(-1).options.body), { project: 'id:orchard', device: 'laptop', platform: 'macos', view: 'review' });
  const linked = view('#connect=' + request64 + '&view=review&memory=72');
  assert.equal(linked.get('device-panel').placedBefore, linked.get('memory-section'));
  linked.ui.openMemory(memory(72));
  assert.match(linked.location.hash, /connect=/);
  assert.match(linked.location.hash, /memory=72/);
});

test('hash navigation asks before discarding and Keep editing restores the old URL', async () => {
  const v = view();
  await openLatest(v, memory(73));
  const old = v.location.hash;
  v.get('edit-memory').listeners.click();
  v.get('edit-title').value = 'New title';
  v.location.hash = '#view=archived';
  v.windowListeners.hashchange();
  assert.equal(v.location.hash, old);
  assert.equal(v.get('discard-changes').hidden, false);
  v.get('keep-editing').listeners.click();
  assert.equal(v.get('edit-title').value, 'New title');
  v.location.hash = '#view=archived';
  v.windowListeners.hashchange();
  v.get('discard-draft').listeners.click();
  assert.match(v.location.hash, /view=archived/);
  assert.equal(JSON.parse(v.requests.at(-1).options.body).view, 'archived');
});

test('unchanged edit closes without a discard prompt', async () => {
  const v = view();
  await openLatest(v, memory(74));
  v.get('edit-memory').listeners.click();
  v.get('close-memory').listeners.click();
  assert.equal(v.get('discard-changes').hidden, true);
  assert.equal(v.get('memory-dialog').open, false);
});

test('explicit signout guards a draft and keeps the memory link for sign-in', async () => {
  const v = view();
  await openLatest(v, memory(75));
  v.get('edit-memory').listeners.click();
  v.get('edit-purpose').value = 'lesson';
  v.get('disconnect').listeners.click();
  assert.equal(v.get('discard-changes').hidden, false);
  v.get('discard-draft').listeners.click();
  await tick();
  assert.equal(v.get('memory-dialog').open, false);
  assert.match(v.location.hash, /memory=75/);
  v.get('token').value = 'synthetic-owner-token-0123456789-abcdef';
  await v.get('connection-form').listeners.submit({ preventDefault() {} });
  assert.equal(JSON.parse(v.requests.at(-1).options.body).id, 75);
});

test('restoring a stale revision shows the newer memory without overwriting it', async () => {
  const v = view();
  await openLatest(v, memory(76, 2));
  v.get('previous-revision').listeners.click();
  v.requests.at(-1).reply(memory(76, 1, 'Earlier.'));
  await tick();
  v.get('restore-revision').listeners.click();
  v.requests.at(-1).reply({ error: 'revision_conflict', current: memory(76, 3, 'Newest agent text.') }, 409);
  await tick();
  const latest = v.requests.at(-1);
  assert.equal(latest.url, '/visualizer/api/record');
  latest.reply(memory(76, 3, 'Newest agent text.'));
  await tick();
  assert.equal(v.get('memory-detail-content').textContent, 'Newest agent text.');
  assert.match(v.get('memory-detail-status').textContent, /changed while you were looking/);
  assert.equal(v.get('restore-revision').hidden, true);
});

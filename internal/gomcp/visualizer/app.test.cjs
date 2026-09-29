const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const test = require('node:test');
const vm = require('node:vm');

function view(hash = '', connected = true) {
  const elements = new Map();
  const element = () => ({
    value: '', textContent: '', open: false, children: [], listeners: {},
    addEventListener(name, action) { this.listeners[name] = action; },
    append(...items) { this.children.push(...items); },
    replaceChildren(...items) { this.children = items; },
    querySelector() { return null; },
    contains() { return false; },
    showModal() { this.open = true; }, close() { this.open = false; this.listeners.close?.(); },
    focus() { this.focused = true; }, scrollIntoView() {}, setAttribute(name, value) { this.attributes = { ...this.attributes, [name]: value }; }, classList: { add() {} }
  });
  const get = id => {
    if (!elements.has(id)) elements.set(id, element());
    return elements.get(id);
  };
  const requests = [];
  const timers = new Map();
  const timerDelays = new Map();
  let timerID = 0;
  const sandbox = {
    document: { getElementById: get, querySelector: get, createElement: element, createDocumentFragment: element },
    window: { addEventListener() {}, confirm: () => true, getSelection: () => null },
    location: { origin: 'http://127.0.0.1', hash },
    AbortController,
    setTimeout(fn, delay) { timers.set(++timerID, fn); timerDelays.set(timerID, delay); return timerID; },
    clearTimeout(id) { timers.delete(id); timerDelays.delete(id); },
    fetch(url, options) {
      if (url.endsWith('/session')) return Promise.resolve({ ok: true, json: async () => ({ connected: false }) });
      return new Promise((resolve, reject) => requests.push({ url, options, reject, reply(data, status = 200) {
        resolve({ ok: status < 400, status, json: async () => data });
      } }));
    }
  };
  vm.createContext(sandbox);
  vm.runInContext(fs.readFileSync(path.join(__dirname, 'app.js'), 'utf8'), sandbox);
  const ui = vm.runInContext('({ refreshDevices, stop, setConnected, revokeDevice, refresh, draw, openMemory, loadRecord, closeMemory, restartMemoryView, chooseView, showViewTabs, updateViewCounts })', sandbox);
  if (connected) {
    ui.setConnected(true);
    get('device-panel').open = true;
  }
  return { ui, get, requests, timers, timerDelays, document: sandbox.document };
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
  assert.equal(v.get('status').textContent, 'Connection expired');
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

const tick = () => new Promise(setImmediate);

async function openLatest(v, record) {
  v.ui.openMemory(record);
  v.requests.at(-1).reply(record);
  await tick();
}

const withoutRequestID = ({ request_id, ...rest }) => rest;
const submitEdit = v => v.get('edit-form').listeners.submit({ preventDefault() {} });

test('actions appear only on the latest revision and follow the memory state', async () => {
  const v = view();
  await openLatest(v, { ...memory(1, 2), confirmed: false });
  assert.equal(v.get('memory-actions').hidden, false);
  assert.equal(v.get('confirm-memory').hidden, false);
  assert.equal(v.get('archive-memory').hidden, false);
  assert.equal(v.get('restore-memory').hidden, true);
  v.get('previous-revision').listeners.click();
  v.requests.at(-1).reply(memory(1, 1));
  await tick();
  assert.equal(v.get('memory-actions').hidden, true);
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
  assert.equal(v.get('status').textContent, 'Connection expired');
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
    assert.equal(v.get('server-status').hidden, false);
  });
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
  v.ui.closeMemory();
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

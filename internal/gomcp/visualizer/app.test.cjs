const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const test = require('node:test');
const vm = require('node:vm');

function view() {
  const elements = new Map();
  const element = () => ({
    value: '', textContent: '', open: false, children: [], listeners: {},
    addEventListener(name, action) { this.listeners[name] = action; },
    append(...items) { this.children.push(...items); },
    replaceChildren(...items) { this.children = items; },
    querySelector() { return null; },
    contains() { return false; },
    showModal() { this.open = true; }, close() { this.open = false; this.listeners.close?.(); },
    focus() {}, scrollIntoView() {}, classList: { add() {} }
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
    location: { origin: 'http://127.0.0.1', hash: '' },
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
  const ui = vm.runInContext('({ refreshDevices, stop, setConnected, revokeDevice, refresh, draw, openMemory, loadRecord, closeMemory, restartMemoryView })', sandbox);
  ui.setConnected(true);
  get('device-panel').open = true;
  return { ui, get, requests, timers, timerDelays };
}

function replyPair(v, start, devices = []) {
  v.requests[start].reply([]);
  v.requests[start + 1].reply(devices);
}

function deviceLabels(v) {
  return v.get('connected-devices').children.map(row => row.children[0].textContent);
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
  assert.equal(v.get('status').textContent, 'Search by wording');
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

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
    focus() {}, scrollIntoView() {}, classList: { add() {} }
  });
  const get = id => {
    if (!elements.has(id)) elements.set(id, element());
    return elements.get(id);
  };
  const requests = [];
  const timers = new Map();
  let timerID = 0;
  const sandbox = {
    document: { getElementById: get, querySelector: get, createElement: element, createDocumentFragment: element },
    window: { addEventListener() {}, confirm: () => true, getSelection: () => null },
    location: { origin: 'http://127.0.0.1', hash: '' },
    AbortController,
    setTimeout(fn) { timers.set(++timerID, fn); return timerID; },
    clearTimeout(id) { timers.delete(id); },
    fetch(url, options) {
      if (url.endsWith('/session')) return Promise.resolve({ ok: true, json: async () => ({ connected: false }) });
      return new Promise((resolve, reject) => requests.push({ url, options, reject, reply(data, status = 200) {
        resolve({ ok: status < 400, status, json: async () => data });
      } }));
    }
  };
  vm.createContext(sandbox);
  vm.runInContext(fs.readFileSync(path.join(__dirname, 'app.js'), 'utf8'), sandbox);
  const ui = vm.runInContext('({ refreshDevices, stop, setConnected, revokeDevice })', sandbox);
  ui.setConnected(true);
  get('device-panel').open = true;
  return { ui, get, requests, timers };
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

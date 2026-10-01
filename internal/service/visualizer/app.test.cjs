// Memory view regressions. A small fake DOM and a scripted fetch drive the
// real app.js in a VM; no npm dependencies.
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const test = require('node:test');
const vm = require('node:vm');

const source = fs.readFileSync(path.join(__dirname, 'app.js'), 'utf8');
const TOKEN = 'synthetic-test-only-'.repeat(3);
const PROJECT = 'git:github.com/example/grasshopper';

// ---------- fake DOM ----------
class Node {
  constructor(tag) { Object.assign(this, { tagName: tag.toUpperCase(), children: [], attributes: {}, listeners: {}, className: '', value: '', checked: false, disabled: false, hidden: false, parent: null, _text: null }); }
  get id() { return this.attributes.id || ''; }
  get textContent() { return this._text ?? this.children.map(child => typeof child === 'string' ? child : child.textContent).join(''); }
  set textContent(value) { this._text = String(value); this.children = []; }
  append(...items) { for (const item of items) { if (typeof item !== 'string') item.parent = this; this.children.push(item); } this._text = null; }
  replaceChildren(...items) { this.children = []; this._text = null; this.append(...items); }
  setAttribute(name, value) { this.attributes[name] = String(value); }
  getAttribute(name) { return this.attributes[name] ?? null; }
  addEventListener(name, action) { (this.listeners[name] ||= []).push(action); }
  dispatch(name, extra = {}) { const event = { target: this, currentTarget: this, preventDefault() {}, ...extra }; for (const action of this.listeners[name] || []) action(event); return event; }
  click() {
    if (this.tagName === 'A') { this.ownerDocument.downloads.push(this.attributes.download); return; }
    if (this.disabled) return;
    this.dispatch('click');
    // As in a browser, a submit button submits its form.
    if (this.tagName === 'BUTTON' && this.attributes.type === 'submit') for (let node = this.parent; node; node = node.parent) if (node.tagName === 'FORM') { node.dispatch('submit'); break; }
  }
  focus() { this.ownerDocument.activeElement = this; }
  contains(node) { for (let current = node; current; current = current.parent) if (current === this) return true; return false; }
  *walk() { for (const child of this.children) if (typeof child !== 'string') { yield child; yield* child.walk(); } }
  matches(selector) {
    return selector.split(',').map(part => part.trim()).some(part => {
      const match = part.match(/^([a-z0-9]*)(#[\w-]+)?((?:\.[\w-]+)*)$/i);
      if (!match) return false;
      const [, tag, id, classes] = match;
      if (tag && this.tagName !== tag.toUpperCase()) return false;
      if (id && this.id !== id.slice(1)) return false;
      const own = this.className.split(/\s+/);
      return classes.split('.').filter(Boolean).every(name => own.includes(name));
    });
  }
  querySelectorAll(selector) { return [...this.walk()].filter(node => node.matches(selector)); }
  querySelector(selector) { return this.querySelectorAll(selector)[0] || null; }
}

function response(status, data, headers = {}) {
  return { status, ok: status < 400, headers: { get: name => headers[name] || null }, json: async () => data, text: async () => data === undefined ? '' : JSON.stringify(data) };
}
async function flush() { for (let i = 0; i < 12; i++) await new Promise(resolve => setImmediate(resolve)); }

function setup({ hash = '', session = false, hidden = false } = {}) {
  const requests = [];
  const timers = new Map();
  let timerID = 0;
  const documentListeners = {};
  const windowListeners = {};
  const document = {
    hidden, activeElement: null, title: '', downloads: [], body: null,
    createElement(tag) { const node = new Node(tag); node.ownerDocument = document; return node; },
    getElementById(id) { if (id === 'app') return app; if (id === 'announce') return announce; return [...app.walk()].find(node => node.id === id) || null; },
    addEventListener(name, action) { (documentListeners[name] ||= []).push(action); }
  };
  const app = document.createElement('div');
  const announce = document.createElement('p');
  document.body = document.createElement('body');
  const history = [];
  const location = { hash, pathname: '/visualizer/' };
  const sandbox = {
    document, location, URLSearchParams, TextEncoder, AbortController, console,
    Blob: class { constructor(parts) { this.parts = parts; } },
    URL: { createObjectURL: () => 'blob:test', revokeObjectURL() {} },
    window: {
      addEventListener(name, action) { (windowListeners[name] ||= []).push(action); },
      confirm: () => true, getSelection: () => null,
      history: { pushState(_s, _t, url) { history.push(url); location.hash = url.startsWith('#') ? url : ''; }, replaceState(_s, _t, url) { location.hash = url.startsWith('#') ? url : ''; } },
      grasshopperTheme: { saved: () => 'system', set() {} }
    },
    navigator: { clipboard: { writeText: async () => {} } },
    setTimeout(fn, delay) { timers.set(++timerID, { fn, delay }); return timerID; },
    clearTimeout(id) { timers.delete(id); },
    fetch(url, options = {}) {
      if (url === '/visualizer/api/session' && (options.method || 'GET') === 'GET') return Promise.resolve(response(200, { connected: session }));
      return new Promise((resolve, reject) => requests.push({
        url, method: options.method || 'GET', headers: options.headers || {}, body: options.body ? JSON.parse(options.body) : undefined,
        reply(status, data, headers = {}) { resolve(response(status, data, headers)); }, fail() { reject(new TypeError('network')); }
      }));
    }
  };
  sandbox.globalThis = sandbox;
  vm.createContext(sandbox);
  vm.runInContext(source, sandbox);
  return {
    app, announce, document, requests, timers, history, location, sandbox,
    find: text => [...app.walk()].find(node => node.tagName === 'BUTTON' && (node.textContent.trim() === text || node.getAttribute('aria-label') === text)),
    rows: text => app.querySelectorAll('button.row').find(node => node.textContent.includes(text)),
    text: () => app.textContent,
    pending: url => requests.filter(request => request.url === url && !request.done),
    async answer(url, status, data, headers) {
      await flush();
      const request = requests.find(item => item.url === url && !item.done);
      assert.ok(request, `expected a request to ${url}`);
      request.done = true;
      request.reply(status, data, headers);
      await flush();
      return request;
    },
    setHidden(value) { document.hidden = value; for (const action of documentListeners.visibilitychange || []) action(); },
    hashChange(value) { location.hash = value; for (const action of windowListeners.hashchange || []) action(); }
  };
}

const today = new Date().toISOString();
function memory(id, title, fields = {}) {
  return { id, revision: 1, title, content: `${title}.`, purpose: 'decision', confirmed: true, scope: { project: PROJECT }, provenance: { harness: 'codex-desktop', device: 'mac', source: 'Example.' }, updated_at: today, ...fields };
}
const projectRecords = [
  memory(1, 'Squash merge only', { provenance: { harness: 'claude', device: 'mac', source: 'Owner said so.' } }),
  memory(2, 'Use gh for PR status', { confirmed: false, purpose: 'observation' }),
  memory(3, 'Prefers deterministic setups', { scope: {}, purpose: 'preference' }),
  memory(4, 'Shipped 2.8.5. Next: redesign.', { purpose: 'handoff', confirmed: false })
];

// An active session on a project page, with its first loads answered.
async function signedIn() {
  const ui = setup({ session: true, hash: `#page=project&project=${encodeURIComponent(PROJECT)}` });
  await flush();
  await ui.answer('/visualizer/api/context', 200, { records: projectRecords, projects: [PROJECT], review_count: 1, server: { version: '2.9.0', memories: 4, archived: 0 }, total: 4 }, { ETag: '"index-1"' });
  await ui.answer('/visualizer/api/context', 200, { records: projectRecords, projects: [PROJECT], total: 4 }, { ETag: '"page-1"' });
  await ui.answer('/visualizer/api/pairings', 200, []);
  await ui.answer('/visualizer/api/devices', 200, [{ id: 7, device: 'mac', created_at: today, revoked_at: null, last_seen_at: today }]);
  return ui;
}
async function openSquash(ui) {
  ui.rows('Squash merge only').click();
  await ui.answer('/visualizer/api/record', 200, projectRecords[0]);
}

// ---------- tests ----------

test('memory text is only placed as text and credentials are never stored', () => {
  assert.doesNotMatch(source, /innerHTML|insertAdjacentHTML|outerHTML|localStorage/);
  assert.match(source, /textContent/);
});

test('signing in rejects an empty token, reports a wrong one, and sends the bearer once', async () => {
  const ui = setup();
  await flush();
  ui.app.querySelector('form.signin-box').dispatch('submit');
  assert.match(ui.text(), /Paste your access token/);
  ui.app.querySelector('#token').value = 'wrong-token-0123456789-0123456789';
  ui.app.querySelector('form.signin-box').dispatch('submit');
  await ui.answer('/visualizer/api/session', 401);
  assert.match(ui.text(), /That token was rejected/);
  assert.equal(ui.app.querySelector('#token').value, '', 'the field never keeps a token');
  ui.app.querySelector('#token').value = TOKEN;
  ui.app.querySelector('form.signin-box').dispatch('submit');
  const sent = await ui.answer('/visualizer/api/session', 200, {});
  assert.equal(sent.headers.Authorization, `Bearer ${TOKEN}`);
  assert.ok(ui.pending('/visualizer/api/context').length, 'signing in loads memories');
});

test('a project page shows suggestions, its own memories, everywhere memories and the latest handoff', async () => {
  const ui = await signedIn();
  const labels = ui.app.querySelectorAll('h2.sec-label').map(node => node.textContent);
  assert.deepEqual(labels, ['New', 'grasshopper', 'All projects', new Date().toLocaleDateString(undefined, { month: 'short', day: 'numeric' })]);
  assert.ok(ui.find('Keep Use gh for PR status'), 'suggestions have Keep');
  assert.ok(ui.find('Show 1 that also apply here'), 'everywhere memories are folded');
  assert.match(ui.rows('Squash merge only').textContent, /Claude Code · today/);
  assert.match(ui.text(), /New1/, 'the sidebar counts suggestions');
});

test('Keep confirms at the shown revision and a retry reuses its request ID', async () => {
  const ui = await signedIn();
  ui.find('Keep Use gh for PR status').click();
  await flush();
  const first = ui.requests.find(request => request.url === '/visualizer/api/update');
  assert.deepEqual({ ...first.body, request_id: undefined }, { id: 2, expected_revision: 1, action: 'confirm', request_id: undefined });
  first.done = true; first.fail();
  await flush();
  assert.match(ui.announce.textContent, /won't save twice/);
  ui.find('Keep Use gh for PR status').click();
  await flush();
  const retry = ui.requests.filter(request => request.url === '/visualizer/api/update')[1];
  assert.equal(retry.body.request_id, first.body.request_id);
  retry.done = true; retry.reply(200, { id: 2, revision: 2 });
  await flush();
  assert.match(ui.announce.textContent, /Kept “Use gh for PR status”/);
});

test('an edit that loses a race shows both versions and saves against the newer one', async () => {
  const ui = await signedIn();
  await openSquash(ui);
  ui.find('Edit').click();
  ui.document.getElementById('edit-text').value = 'Squash merge only. Always.';
  ui.find('Save').click();
  await flush();
  const save = ui.requests.find(request => request.url === '/visualizer/api/update');
  assert.equal(save.body.expected_revision, 1);
  const theirs = { ...projectRecords[0], revision: 2, content: 'Squash or rebase.', provenance: { harness: 'codex-desktop', device: 'mac', source: 'x' } };
  save.done = true; save.reply(409, { error: 'revision_conflict', current: theirs });
  await flush();
  assert.match(ui.text(), /Codex changed this while you were editing\. Nothing was overwritten\./);
  assert.match(ui.text(), /Squash or rebase\./);
  assert.match(ui.text(), /Squash merge only\. Always\./);
  ui.find('Keep yours').click();
  await flush();
  const again = ui.requests.filter(request => request.url === '/visualizer/api/update')[1];
  assert.equal(again.body.expected_revision, 2);
  assert.equal(again.body.content, 'Squash merge only. Always.');
});

test('archiving offers Undo, which restores at the archived revision', async () => {
  const ui = await signedIn();
  await openSquash(ui);
  ui.find('Archive').click();
  await ui.answer('/visualizer/api/archive', 200, { id: 1, revision: 2 });
  assert.match(ui.text(), /Archived “Squash merge only”Undo/);
  ui.find('Undo').click();
  await flush();
  const undo = ui.requests.filter(request => request.url === '/visualizer/api/archive')[1];
  assert.deepEqual({ ...undo.body, request_id: undefined }, { id: 1, expected_revision: 2, archived: false, request_id: undefined });
});

test('Move to All projects posts a null project', async () => {
  const ui = await signedIn();
  await openSquash(ui);
  ui.find('Move to…').click();
  ui.app.querySelectorAll('button.pick').find(node => node.textContent === 'All projects').click();
  await flush();
  const moved = ui.requests.find(request => request.url === '/visualizer/api/move');
  assert.deepEqual({ ...moved.body, request_id: undefined }, { id: 1, expected_revision: 1, project: null, request_id: undefined });
});

test('only a list already on screen is asked with its ETag, and 304 keeps it', async () => {
  const ui = await signedIn();
  for (const [id, timer] of [...ui.timers.entries()]) if (timer.delay === 5000) { ui.timers.delete(id); timer.fn(); }
  await flush();
  const poll = ui.pending('/visualizer/api/context').find(request => request.body?.project === PROJECT);
  assert.equal(poll.headers['If-None-Match'], '"page-1"');
  poll.done = true; poll.reply(304);
  await flush();
  assert.match(ui.text(), /Squash merge only/);
  // Coming back to a list must load it, not wait on a 304 for an empty screen.
  ui.hashChange('#page=archived');
  await flush();
  ui.hashChange(`#page=project&project=${encodeURIComponent(PROJECT)}`);
  await flush();
  const revisit = ui.pending('/visualizer/api/context').filter(request => request.body?.project === PROJECT).pop();
  assert.equal(revisit.headers['If-None-Match'], undefined);
});

test('an ended session clears memories and asks to sign in again', async () => {
  const ui = await signedIn();
  ui.rows('Squash merge only').click();
  await ui.answer('/visualizer/api/record', 401);
  assert.match(ui.text(), /Your session ended\. Sign in again\./);
  assert.doesNotMatch(ui.text(), /Squash merge only/);
});

test('a hidden page stops polling and one loop resumes when visible', async () => {
  const ui = await signedIn();
  ui.setHidden(true);
  assert.equal(ui.timers.size, 0, 'no polling while hidden');
  ui.setHidden(false);
  await flush();
  assert.equal(ui.pending('/visualizer/api/context').length, 2, 'one index and one page request');
});

test('an approval link opens Agents, focuses its request once, and approves with the code', async () => {
  const id = 'a'.repeat(64);
  const ui = setup({ session: true, hash: `#connect=${id}` });
  await flush();
  await ui.answer('/visualizer/api/context', 200, { records: projectRecords, projects: [PROJECT], review_count: 0, total: 4 });
  await ui.answer('/visualizer/api/pairings', 200, [{ request_id: id, code: 'C0DE42', device: 'linux-box', status: 'pending', expires_in: 240 }]);
  await ui.answer('/visualizer/api/devices', 200, []);
  assert.match(ui.text(), /linux-box wants to connectC0DE42/);
  assert.equal(ui.document.activeElement?.id, 'focused-request');
  ui.document.activeElement = null;
  ui.find('Approve').click();
  await flush();
  assert.notEqual(ui.document.activeElement?.id, 'focused-request', 'focus moves only once per link');
  const decision = ui.requests.find(request => request.url === '/visualizer/api/pairings' && request.method === 'POST');
  assert.deepEqual(decision.body, { request_id: id, code: 'C0DE42', decision: 'approve' });
});

test('agents show when each machine was last seen', async () => {
  const ui = await signedIn();
  ui.find('Agents').click();
  await flush();
  assert.match(ui.text(), /macactive now/);
});

test('helpers: recency groups, similar suggestions and project names', () => {
  const ui = setup();
  const now = new Date(2026, 8, 30, 12);
  const records = [
    memory(1, 'a', { updated_at: new Date(2026, 8, 29).toISOString() }),
    memory(2, 'b', { updated_at: new Date(2026, 8, 10).toISOString() }),
    memory(3, 'c', { updated_at: new Date(2026, 6, 1).toISOString() })
  ];
  const groups = vm.runInContext('recencyGroups', ui.sandbox)(records, now);
  assert.equal(JSON.stringify(groups.map(group => [group.label, group.records.map(record => record.id)])), JSON.stringify([['This week', [1]], [now.toLocaleDateString(undefined, { month: 'long' }), [2]], ['Earlier', [3]]]));
  const similar = vm.runInContext('similarTo', ui.sandbox);
  const kept = [memory(9, 'One short sentence of help text', { content: 'Help text in the product is one short sentence at most.' })];
  assert.equal(similar(memory(5, 'Prefers terse UI copy', { content: 'Keep interface copy short. One sentence of help text at most.', confirmed: false }), kept)?.id, 9);
  assert.equal(similar(memory(6, 'Metric units first', { content: 'Grams before cups.' }), kept), null);
  const name = vm.runInContext('projectName', ui.sandbox);
  assert.equal(name('git:github.com/MylesMCook/grasshopper'), 'grasshopper');
  assert.equal(name('id:recipes'), 'recipes');
  assert.equal(name(''), 'All projects');
});

test('persisted theme applies before body DOM exists and its script precedes styles', () => {
  for (const theme of ['light', 'dark']) {
    const document = { documentElement: { dataset: {} } };
    vm.runInNewContext(fs.readFileSync(path.join(__dirname, 'theme-init.js'), 'utf8'), { document, localStorage: { getItem: () => theme } });
    assert.equal(document.documentElement.dataset.theme, theme);
  }
  const pages = [path.join(__dirname, 'index.html'), ...['index.html', 'setup/index.html', 'view/index.html', '404.html'].map(file => path.join(__dirname, '../../../web/public', file))];
  for (const file of pages) { const html = fs.readFileSync(file, 'utf8'); assert.ok(html.indexOf('theme-init.js') < html.indexOf('rel="stylesheet"')); assert.ok(!/<script[^>]*theme-init.js[^>]*defer/.test(html)); }
});

test('an unchanged poll does not redraw, and an in-page anchor is not navigation', async () => {
  const ui = await signedIn();
  const drawn = ui.app.children[0];
  for (const [id, timer] of [...ui.timers.entries()]) if (timer.delay === 5000) { ui.timers.delete(id); timer.fn(); }
  await flush();
  const poll = ui.pending('/visualizer/api/context').find(request => request.body?.project === PROJECT);
  poll.done = true; poll.reply(304);
  await flush();
  assert.equal(ui.app.children[0], drawn, 'a 304 keeps the page as drawn');
  ui.hashChange('#main');
  await flush();
  assert.equal(ui.app.children[0], drawn, 'the skip link does not navigate');
  assert.match(ui.text(), /Squash merge only/);
});

test('a conflict on Keep refreshes the open memory, and an uncertain failure keeps the request ID', async () => {
  const ui = await signedIn();
  ui.rows('Use gh for PR status').click();
  await ui.answer('/visualizer/api/record', 200, projectRecords[1]);
  ui.find('Keep').click();
  await flush();
  const first = ui.requests.find(request => request.url === '/visualizer/api/update');
  first.done = true; first.reply(503, { error: 'unavailable' });
  await flush();
  ui.find('Keep').click();
  await flush();
  const retry = ui.requests.filter(request => request.url === '/visualizer/api/update')[1];
  assert.equal(retry.body.request_id, first.body.request_id, 'a 5xx retry reuses its ID');
  retry.done = true; retry.reply(409, { error: 'revision_conflict', current: { ...projectRecords[1], revision: 3, content: 'Use gh, newer.' } });
  await flush();
  assert.match(ui.text(), /Use gh, newer\./);
  ui.app.querySelector('aside.panel').querySelectorAll('button').find(node => node.textContent === 'Keep').click();
  await flush();
  assert.equal(ui.requests.filter(request => request.url === '/visualizer/api/update')[2].body.expected_revision, 3);
});

test('a project memory can only move to All projects', async () => {
  const ui = await signedIn();
  await openSquash(ui);
  ui.find('Move to…').click();
  assert.deepEqual(ui.app.querySelectorAll('button.pick').map(node => node.textContent), ['All projects']);
});

test('a rate-limited sign-in says to wait', async () => {
  const ui = setup();
  await flush();
  ui.app.querySelector('#token').value = TOKEN;
  ui.app.querySelector('form.signin-box').dispatch('submit');
  await ui.answer('/visualizer/api/session', 429);
  assert.match(ui.text(), /Too many attempts\. Wait five minutes/);
});

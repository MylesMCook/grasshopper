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
  // As in a browser, a disabled or hidden control cannot take focus.
  focus() { for (let node = this; node; node = node.parent) if (node.hidden) return; if (!this.disabled) this.ownerDocument.activeElement = this; }
  contains(node) { for (let current = node; current; current = current.parent) if (current === this) return true; return false; }
  *walk() { for (const child of this.children) if (typeof child !== 'string') { yield child; yield* child.walk(); } }
  matches(selector) {
    return selector.split(',').map(part => part.trim()).some(part => {
      const match = part.match(/^([a-z0-9]*)(#[\w-]+)?((?:\.[\w-]+)*)(?:\[([\w-]+)\])?$/i);
      if (!match) return false;
      const [, tag, id, classes, attribute] = match;
      if (attribute && !(attribute in this.attributes)) return false;
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

function setup({ hash = '', session = false, hidden = false, narrow = false } = {}) {
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
      grasshopperTheme: { saved: () => 'system', set() {} },
      // narrow: true lays out as a laptop, where the open memory replaces the list.
      matchMedia: () => ({ matches: narrow, addEventListener() {} })
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
async function signedIn(options = {}) {
  const ui = setup({ session: true, hash: `#page=project&project=${encodeURIComponent(PROJECT)}`, ...options });
  await flush();
  await ui.answer('/visualizer/api/context', 200, { records: projectRecords, projects: [PROJECT], review_count: 1, server: { version: '2.9.0', memories: 4, archived: 0 }, total: 4 }, { ETag: '"index-1"' });
  await ui.answer('/visualizer/api/context', 200, { records: projectRecords, projects: [PROJECT], total: 4 }, { ETag: '"page-1"' });
  await ui.answer('/visualizer/api/pairings', 200, []);
  await ui.answer('/visualizer/api/devices', 200, [{ id: 7, device: 'mac', created_at: today, revoked_at: null, last_seen_at: today }]);
  return ui;
}
function agentsLink(ui) { return ui.app.querySelectorAll('button.nav').find(node => node.getAttribute('data-key') === 'nav:#page=agents'); }
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
  await flush();
  ui.app.querySelector('#token').value = 'typed-while-signing-in';
  ui.app.querySelector('form.signin-box').dispatch('submit');
  assert.equal(ui.app.querySelector('#token').value, 'typed-while-signing-in', 'a second submit while signing in is ignored');
  assert.equal(ui.pending('/visualizer/api/session').length, 1);
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

test('the owner names a device; the name shows everywhere and the ID stays visible', async () => {
  const ui = await signedIn();
  ui.hashChange('#page=agents');
  await flush();
  ui.rows('mac').click();
  await flush();
  assert.match(ui.text(), /ID: mac/, 'the panel always shows the device ID');
  const field = ui.document.getElementById('device-name');
  field.value = 'Studio Mac';
  field.dispatch('input');
  // A routine refresh with a newer last-seen time must not wipe the typing.
  for (const [id, timer] of [...ui.timers.entries()]) if (timer.delay === 5000 || timer.delay === 30000) { ui.timers.delete(id); timer.fn(); }
  await flush();
  for (const request of ui.pending('/visualizer/api/pairings')) { request.done = true; request.reply(200, []); }
  for (const request of ui.pending('/visualizer/api/devices')) { request.done = true; request.reply(200, [{ id: 7, device: 'mac', created_at: today, revoked_at: null, last_seen_at: new Date(Date.now() + 60000).toISOString() }]); }
  await flush();
  assert.equal(ui.document.getElementById('device-name').value, 'Studio Mac', 'typing survives a refresh');
  ui.find('Save name').click();
  await flush();
  const patch = ui.requests.find(request => request.url === '/visualizer/api/devices' && request.method === 'PATCH');
  assert.deepEqual(patch.body, { device: 'mac', label: 'Studio Mac' });
  patch.done = true; patch.reply(204);
  await flush();
  for (const request of ui.pending('/visualizer/api/pairings')) { request.done = true; request.reply(200, []); }
  for (const request of ui.pending('/visualizer/api/devices')) { request.done = true; request.reply(200, [{ id: 7, device: 'mac', label: 'Studio Mac', created_at: today, revoked_at: null, last_seen_at: today }]); }
  await flush();
  assert.ok(ui.rows('Studio Mac'), 'the Agents list shows the name');
  assert.match(ui.app.querySelector('h2.panel-title').textContent, /Studio Mac/);
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
  agentsLink(ui).click();
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

test('opening the Disconnect confirmation never puts focus on the destructive button', async () => {
  const ui = await signedIn();
  agentsLink(ui).click();
  await flush();
  ui.rows('mac').click();
  const opener = ui.app.querySelector('aside.panel').querySelectorAll('button').find(node => node.textContent === 'Disconnect');
  opener.focus();
  opener.click();
  const confirm = ui.app.querySelectorAll('button.btn').find(node => node.textContent === 'Disconnect');
  assert.ok(confirm, 'the confirmation shows its Disconnect button');
  assert.notEqual(ui.document.activeElement, confirm, 'a held Enter must not disconnect');
  assert.equal(ui.document.activeElement?.id, 'disconnect-question', 'focus moves to the question');
  for (const [id, timer] of [...ui.timers.entries()]) if (timer.delay === 5000) { ui.timers.delete(id); timer.fn(); }
  await ui.answer('/visualizer/api/pairings', 200, []);
  await ui.answer('/visualizer/api/devices', 200, [{ id: 7, device: 'mac', created_at: today, revoked_at: null, last_seen_at: null }]);
  assert.equal(ui.document.activeElement?.id, 'disconnect-question', 'a poll redraw keeps focus on the question');
  ui.find('Cancel').click();
  assert.equal(ui.document.activeElement?.getAttribute('data-key'), 'disconnect-open:7', 'Cancel returns focus to the opener');
  assert.equal(ui.pending('/visualizer/api/devices').filter(request => request.method === 'DELETE').length, 0);
});

test('a focused Save keeps focus while saving and cannot be pressed twice', async () => {
  const ui = await signedIn();
  await openSquash(ui);
  ui.find('Edit').click();
  ui.document.getElementById('edit-text').value = 'Squash merge only. Always.';
  const save = ui.find('Save');
  save.focus();
  save.click();
  await flush();
  const focused = ui.document.activeElement;
  assert.equal(focused?.getAttribute('data-key'), 'save', 'focus stays on Save while it reads Saving…');
  assert.equal(focused.textContent, 'Saving…');
  assert.equal(focused.getAttribute('aria-disabled'), 'true');
  focused.click();
  await flush();
  assert.equal(ui.requests.filter(request => request.url === '/visualizer/api/update').length, 1, 'one save, not two');
  await ui.answer('/visualizer/api/update', 200, { id: 1, revision: 2 });
  assert.equal(ui.document.activeElement?.id, 'panel-title', 'a finished save leaves focus in the panel');
});

test('the Agents link says how many agents are active', async () => {
  const ui = await signedIn();
  const agents = ui.app.querySelectorAll('button.nav').find(node => node.textContent.startsWith('Agents'));
  assert.match(agents.textContent, /Agents, 1 of 1 active/);
  assert.equal(agents.getAttribute('data-key'), 'nav:#page=agents');
});

test('an edit error is tied to its field and takes focus', async () => {
  const ui = await signedIn();
  await openSquash(ui);
  ui.find('Edit').click();
  ui.document.getElementById('edit-text').value = '   ';
  ui.find('Save').click();
  const error = ui.document.getElementById('edit-error');
  assert.equal(error?.textContent, 'Write some text before saving.');
  const field = ui.document.getElementById('edit-text');
  assert.equal(field.getAttribute('aria-invalid'), 'true');
  assert.equal(field.getAttribute('aria-describedby'), 'edit-error');
  assert.equal(ui.document.getElementById('edit-title').getAttribute('aria-invalid'), 'false');
  assert.equal(ui.document.activeElement, field);
});

test('Undo waits while it has focus or the pointer, then times out again', async () => {
  const ui = await signedIn();
  await openSquash(ui);
  ui.find('Archive').click();
  await ui.answer('/visualizer/api/archive', 200, { id: 1, revision: 2 });
  const waiting = () => [...ui.timers.values()].filter(timer => timer.delay === 10000).length;
  assert.equal(waiting(), 1);
  const bar = ui.app.querySelector('div.undo');
  bar.dispatch('focusin');
  assert.equal(waiting(), 0, 'focus pauses the timer');
  bar.dispatch('focusout');
  assert.equal(waiting(), 1, 'leaving restarts it');
  bar.dispatch('mouseenter');
  assert.equal(waiting(), 0, 'hover pauses the timer');
  bar.dispatch('mouseleave');
  assert.equal(waiting(), 1);
});

test('decorative arrows are hidden from assistive technology', async () => {
  const ui = await signedIn();
  const menu = ui.app.querySelector('button.menu-btn');
  assert.ok(menu.querySelectorAll('span').some(node => node.getAttribute('aria-hidden') === 'true' && node.textContent === '▾'));
  await openSquash(ui);
  const back = ui.app.querySelector('button.back');
  assert.ok(back.querySelectorAll('span').some(node => node.getAttribute('aria-hidden') === 'true' && node.textContent === '←'));
  agentsLink(ui).click();
  await flush();
  assert.ok(ui.find('Copy Connect Grasshopper'), 'Copy names what it copies');
});

test('focus rings stay visible on filled controls and in forced colors', () => {
  const css = fs.readFileSync(path.join(__dirname, 'style.css'), 'utf8');
  assert.match(css, /\.undo :focus-visible\s*\{[^}]*outline-color:var\(--on-primary\)/);
  assert.match(css, /\.seg button\[aria-pressed="true"\]:focus-visible\s*\{[^}]*outline-color:var\(--on-primary\)/);
  const forced = css.slice(css.indexOf('@media (forced-colors:active)'));
  assert.ok(forced.length < css.length, 'a forced-colors block exists');
  for (const selector of ['.row[aria-current="true"]', '.seg button[aria-pressed="true"]', '.undo', '.confirm', '.conflict', '.strip', '.dot']) assert.ok(forced.includes(selector), selector);
});

test('a conflict after a focused Save moves focus somewhere visible', async () => {
  const ui = await signedIn();
  await openSquash(ui);
  ui.find('Edit').click();
  ui.document.getElementById('edit-text').value = 'Squash merge only. Always.';
  const save = ui.find('Save');
  save.focus();
  save.click();
  await flush();
  const theirs = { ...projectRecords[0], revision: 2, content: 'Squash or rebase.', provenance: { harness: 'codex-desktop', device: 'mac', source: 'x' } };
  await ui.answer('/visualizer/api/update', 409, { error: 'revision_conflict', current: theirs });
  const focused = ui.document.activeElement;
  assert.ok(focused && [...ui.app.walk()].includes(focused), 'focus is on a control in the current page, not a replaced one');
  for (let node = focused; node; node = node.parent) assert.equal(node.hidden, false, 'focus is not inside the hidden edit form');
});

test('saving text another memory already has keeps the edit and says why', async () => {
  const ui = await signedIn();
  await openSquash(ui);
  ui.find('Edit').click();
  ui.document.getElementById('edit-text').value = 'Use gh for PR status.';
  ui.find('Save').click();
  await flush();
  await ui.answer('/visualizer/api/update', 409, { error: 'duplicate_conflict', current: projectRecords[0] });
  assert.match(ui.announce.textContent, /Another memory already has this text/);
  assert.equal(ui.document.getElementById('edit-text').value, 'Use gh for PR status.', 'the draft is kept');
  assert.ok(!ui.app.querySelector('div.conflict'), 'no conflict view for a duplicate');
});

test('a failed Undo keeps waiting while it still has focus', async () => {
  const ui = await signedIn();
  await openSquash(ui);
  ui.find('Archive').click();
  await ui.answer('/visualizer/api/archive', 200, { id: 1, revision: 2 });
  const waiting = () => [...ui.timers.values()].filter(timer => timer.delay === 10000).length;
  const button = ui.find('Undo');
  button.focus();
  ui.app.querySelector('div.undo').dispatch('focusin');
  assert.equal(waiting(), 0);
  button.click();
  await flush();
  await ui.answer('/visualizer/api/archive', 500, { error: 'unavailable' });
  assert.ok(ui.app.querySelector('div.undo'), 'Undo is still offered');
  assert.equal(ui.document.activeElement?.getAttribute('data-key'), 'undo', 'focus is back on Undo');
  assert.equal(waiting(), 0, 'the timer stays paused while Undo has focus');
});

test('the pressed segment keeps a contrasting focus ring in forced colors', () => {
  const css = fs.readFileSync(path.join(__dirname, 'style.css'), 'utf8');
  const forced = css.slice(css.indexOf('@media (forced-colors:active)'));
  assert.match(forced, /\.seg button\[aria-pressed="true"\]:focus-visible\s*\{[^}]*outline-color:HighlightText/);
});

test('a save that finishes after opening another memory leaves focus where it is', async () => {
  const ui = await signedIn();
  await openSquash(ui);
  ui.find('Edit').click();
  ui.document.getElementById('edit-text').value = 'Squash merge only. Always.';
  ui.find('Save').click();
  await flush();
  ui.rows('Use gh for PR status').click();
  await ui.answer('/visualizer/api/record', 200, projectRecords[1]);
  const here = ui.find('Edit');
  here.focus();
  await ui.answer('/visualizer/api/update', 200, { id: 1, revision: 2 });
  assert.equal(ui.document.activeElement?.textContent, 'Edit', 'the finished save did not pull focus to a panel title');
  assert.match(ui.text(), /Use gh for PR status/);
});

test('leaving Undo while its request is pending keeps it available for a retry', async () => {
  const ui = await signedIn();
  await openSquash(ui);
  ui.find('Archive').click();
  await ui.answer('/visualizer/api/archive', 200, { id: 1, revision: 2 });
  const waiting = () => [...ui.timers.values()].filter(timer => timer.delay === 10000).length;
  const button = ui.find('Undo');
  button.focus();
  ui.app.querySelector('div.undo').dispatch('focusin');
  button.click();
  await flush();
  ui.document.activeElement = null;
  ui.app.querySelector('div.undo').dispatch('focusout');
  assert.equal(waiting(), 0, 'no expiry while the Undo request is pending');
  await ui.answer('/visualizer/api/archive', 500, { error: 'unavailable' });
  assert.ok(ui.app.querySelector('div.undo'), 'Undo is still offered after the failure');
  assert.equal(waiting(), 1, 'the timer restarts once the request fails and the bar is not held');
});

test('the selected row keeps a contrasting focus ring in forced colors', () => {
  const css = fs.readFileSync(path.join(__dirname, 'style.css'), 'utf8');
  const forced = css.slice(css.indexOf('@media (forced-colors:active)'));
  assert.match(forced, /\.row\[aria-current="true"\]:focus-visible\s*\{[^}]*outline-color:CanvasText/);
});

test('focus restore prefers an exact data-key, even when a title matches it', async () => {
  const ui = await signedIn({ narrow: true });
  ui.rows('Squash merge only').click();
  await ui.answer('/visualizer/api/record', 200, { ...projectRecords[0], title: 'save' });
  ui.find('Edit').click();
  ui.document.getElementById('edit-text').value = 'Squash merge only. Always.';
  const save = ui.find('Save');
  save.focus();
  save.click();
  await flush();
  assert.equal(ui.document.activeElement?.getAttribute('data-key'), 'save', 'focus stays on Save, not the panel named "save"');
});

test('focus restore prefers a native control over a region that shares its name', async () => {
  const ui = await signedIn({ narrow: true });
  ui.rows('Squash merge only').click();
  await ui.answer('/visualizer/api/record', 200, { ...projectRecords[0], title: 'Edit' });
  ui.app.querySelectorAll('button').find(node => node.textContent === 'Edit').focus();
  vm.runInContext('render()', ui.sandbox);
  const focused = ui.document.activeElement;
  assert.equal(focused?.tagName, 'BUTTON', 'focus stays on a button, not the region titled "Edit"');
  assert.equal(focused.textContent, 'Edit');
});

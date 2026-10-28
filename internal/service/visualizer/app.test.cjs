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
    if (this.tagName === 'A' && 'download' in this.attributes) { this.ownerDocument.downloads.push(this.attributes.download); return; }
    if (this.disabled) return;
    this.dispatch('click');
    // As in a browser, a submit button submits its form.
    if (this.tagName === 'BUTTON' && this.attributes.type === 'submit') for (let node = this.parent; node; node = node.parent) if (node.tagName === 'FORM') { node.dispatch('submit'); break; }
  }
  // As in a browser, a disabled or hidden control cannot take focus.
  focus() { for (let node = this; node; node = node.parent) if (node.hidden) return; if (!this.disabled) this.ownerDocument.activeElement = this; }
  blur() { if (this.ownerDocument.activeElement === this) this.ownerDocument.activeElement = null; }
  contains(node) { for (let current = node; current; current = current.parent) if (current === this) return true; return false; }
  closest(selector) { for (let node = this; node; node = node.parent) if (node.matches(selector)) return node; return null; }
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
    find: text => [...app.walk()].find(node => (node.tagName === 'BUTTON' || node.tagName === 'A') && (node.textContent.trim() === text || node.getAttribute('aria-label') === text)),
    rows: text => app.querySelectorAll('.row').find(node => node.textContent.includes(text)),
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
    keydown(key) { const event = { key, defaultPrevented: false, target: document.activeElement, preventDefault() { this.defaultPrevented = true; } }; for (const action of documentListeners.keydown || []) action(event); return event; },
    setHidden(value) { document.hidden = value; for (const action of documentListeners.visibilitychange || []) action(); },
    hashChange(value) { location.hash = value; for (const action of windowListeners.hashchange || []) action(); },
    beforeUnload() { const event = { defaultPrevented: false, preventDefault() { this.defaultPrevented = true; } }; for (const action of windowListeners.beforeunload || []) action(event); return event; }
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
function agentsLink(ui) { return ui.app.querySelectorAll('.nav').find(node => node.getAttribute('data-key') === 'nav:#page=agents'); }
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
  assert.deepEqual(labels, [new Date().toLocaleDateString(undefined, { month: 'short', day: 'numeric' }), 'New', 'grasshopper', 'All projects'], 'the latest handoff leads');
  assert.ok(ui.find('Keep Use gh for PR status'), 'suggestions have Keep');
  assert.ok(ui.rows('Prefers deterministic setups'), 'memories for every project show inline, because agents load them here');
  assert.match(ui.text(), /2 memories apply here/, 'the count is what agents start with, not only this project');
  assert.match(ui.rows('Squash merge only').textContent, /Claude Code · today/);
  assert.match(ui.text(), /New1/, 'the sidebar counts suggestions');
});

test('a project with nothing of its own lists what applies from every project, without an empty section', async () => {
  const records = Array.from({ length: 9 }, (_, i) => memory(i + 1, `Everywhere ${i + 1}`, { scope: {} }));
  const ui = setup({ session: true, hash: `#page=project&project=${encodeURIComponent(PROJECT)}` });
  await flush();
  await ui.answer('/visualizer/api/context', 200, { records, projects: [PROJECT], review_count: 0, server: { version: '2.9.10' }, total: 9 });
  await ui.answer('/visualizer/api/context', 200, { records, projects: [PROJECT], total: 9 });
  assert.deepEqual(ui.app.querySelectorAll('h2.sec-label').map(node => node.textContent), ['All projects']);
  assert.doesNotMatch(ui.text(), /Nothing kept for this project/);
  assert.ok(ui.rows('Everywhere 9'), 'not folded behind Show more when nothing else is listed');
});

test('a scope restricted to a device does not repeat its platform', async () => {
  const records = [memory(1, 'Device note', { scope: { device: 'mac', platform: 'macos' } })];
  const ui = setup({ session: true, hash: '#page=all' });
  await flush();
  await ui.answer('/visualizer/api/context', 200, { records, projects: [], review_count: 0, server: { version: '2.9.10' }, total: 1 });
  await ui.answer('/visualizer/api/context', 200, { records, projects: [], total: 1 });
  assert.match(ui.rows('Device note').textContent, /mac only/);
  assert.doesNotMatch(ui.rows('Device note').textContent, /macOS only/);
});

test('Preview startup opens what an agent gets for this project', async () => {
  const ui = await signedIn();
  ui.find('Preview startup').click();
  await flush();
  const request = ui.pending('/visualizer/api/startup')[0];
  assert.ok(request, 'the preview loads');
  assert.deepEqual(request.body.scope, { project: PROJECT });
  const back = ui.app.querySelectorAll('button').find(node => node.getAttribute('data-key') === 'back');
  assert.match(back.textContent, /grasshopper/, 'Back names the page it returns to');
});

test('Preview startup asks before discarding an unsaved edit', async () => {
  const ui = await signedIn();
  await openSquash(ui);
  ui.find('Edit').click();
  ui.document.getElementById('edit-text').value = 'Squash merge only. Always.';
  let asked = 0;
  ui.sandbox.window.confirm = () => { asked++; return false; };
  ui.find('Preview startup').click();
  await flush();
  assert.equal(asked, 1, 'the owner is asked first');
  assert.equal(ui.pending('/visualizer/api/startup').length, 0, 'declining keeps the edit');
  assert.equal(ui.document.getElementById('edit-text').value, 'Squash merge only. Always.');
});

test('agent names read as products, including desktop and CLI variants', async () => {
  const records = [memory(1, 'Desktop note', { provenance: { harness: 'claude-code-desktop', device: 'mac', source: 'x' } }),
    memory(2, 'Cursor note', { provenance: { harness: 'cursor-agent', device: 'mac', source: 'x' } })];
  const ui = setup({ session: true, hash: `#page=project&project=${encodeURIComponent(PROJECT)}` });
  await flush();
  await ui.answer('/visualizer/api/context', 200, { records, projects: [PROJECT], review_count: 0, server: { version: '2.9.7' }, total: 2 });
  await ui.answer('/visualizer/api/context', 200, { records, projects: [PROJECT], total: 2 });
  assert.match(ui.rows('Desktop note').textContent, /Claude Code · today/);
  assert.match(ui.rows('Cursor note').textContent, /Cursor · today/);
});

test('Archived folds replaced handoffs so owner-archived memories stay visible', async () => {
  const ui = await signedIn();
  ui.hashChange('#page=archived');
  await flush();
  const replaced = id => memory(id, `Old handoff ${id}`, { purpose: 'handoff', confirmed: false, archived: true, provenance: { harness: 'codex', device: 'mac', source: 'Replaced by handoff 99' } });
  const page = ui.pending('/visualizer/api/context').find(request => request.body?.view === 'archived');
  page.done = true; page.reply(200, { records: [memory(10, 'Archived by hand', { archived: true }), replaced(11), replaced(12)], projects: [PROJECT], total: 3 });
  await flush();
  assert.ok(ui.rows('Archived by hand'), 'an owner-archived memory is listed');
  assert.ok(!ui.rows('Old handoff 11'), 'replaced handoffs start folded');
  ui.find('Show 2 replaced by newer ones').click();
  assert.ok(ui.rows('Old handoff 11') && ui.rows('Old handoff 12'), 'they expand on request');
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
  // Before the device list refreshes, the field already holds the new name, so a second Save cannot undo it.
  assert.equal(ui.document.getElementById('device-name').value, 'Studio Mac');
  ui.find('Save name').click();
  await flush();
  const again = ui.requests.filter(request => request.method === 'PATCH').pop();
  assert.equal(again.body.label, 'Studio Mac');
  again.done = true; again.reply(204);
  await flush();
  for (const request of ui.pending('/visualizer/api/pairings')) { request.done = true; request.reply(200, []); }
  for (const request of ui.pending('/visualizer/api/devices')) { request.done = true; request.reply(200, [{ id: 7, device: 'mac', label: 'Studio Mac', created_at: today, revoked_at: null, last_seen_at: today }]); }
  await flush();
  assert.ok(ui.rows('Studio Mac'), 'the Agents list shows the name');
  assert.match(ui.app.querySelector('h2.panel-title').textContent, /Studio Mac/);
});

test('a rejected device name says why, and a finished rename never clears another device', async () => {
  const devices = [{ id: 7, device: 'mac', created_at: today, revoked_at: null, last_seen_at: today }, { id: 8, device: 'pc', created_at: today, revoked_at: null, last_seen_at: today }];
  const ui = await signedIn();
  for (const request of ui.pending('/visualizer/api/devices')) { request.done = true; request.reply(200, devices); }
  ui.hashChange('#page=agents');
  await flush();
  for (const request of ui.pending('/visualizer/api/pairings')) { request.done = true; request.reply(200, []); }
  for (const request of ui.pending('/visualizer/api/devices')) { request.done = true; request.reply(200, devices); }
  await flush();
  ui.rows('mac').click();
  await flush();
  const type = value => { const field = ui.document.getElementById('device-name'); field.value = value; field.dispatch('input'); };
  type('x'.repeat(10));
  ui.find('Save name').click();
  await flush();
  let patch = ui.requests.find(request => request.method === 'PATCH' && !request.done);
  // The server sends plain text, which api() reads as { error }.
  patch.done = true; patch.reply(400, { error: 'invalid device name: use up to 64 characters on one line' });
  await flush();
  assert.match(ui.announce.textContent, /up to 64 characters/, 'the reason is shown, not only Try again');
  type('Studio Mac');
  ui.find('Save name').click();
  await flush();
  patch = ui.requests.find(request => request.method === 'PATCH' && !request.done);
  ui.rows('pc').click();
  await flush();
  type('Gaming PC');
  patch.done = true; patch.reply(204);
  await flush();
  assert.equal(ui.document.getElementById('device-name').value, 'Gaming PC', "another device's typing survives");
});

test('a refresh waits while the device name field has focus, so the caret stays put', async () => {
  const ui = await signedIn();
  ui.hashChange('#page=agents');
  await flush();
  ui.rows('mac').click();
  await flush();
  const field = ui.document.getElementById('device-name');
  assert.equal(field.getAttribute('maxlength'), null, 'the server, which counts characters, sets the limit');
  field.focus();
  field.value = 'Studio';
  field.dispatch('input');
  for (const [id, timer] of [...ui.timers.entries()]) if (timer.delay === 5000 || timer.delay === 30000) { ui.timers.delete(id); timer.fn(); }
  await flush();
  for (const request of ui.pending('/visualizer/api/pairings')) { request.done = true; request.reply(200, []); }
  for (const request of ui.pending('/visualizer/api/devices')) { request.done = true; request.reply(200, [{ id: 7, device: 'mac', created_at: today, revoked_at: null, last_seen_at: new Date(Date.now() + 60000).toISOString() }]); }
  await flush();
  assert.equal(ui.document.getElementById('device-name'), field, 'the field was not replaced mid-edit');
  // A change held back while typing shows on the next poll after focus leaves, even if that poll is unchanged.
  const renamed = [{ id: 7, device: 'mac', label: 'Renamed elsewhere', created_at: today, revoked_at: null, last_seen_at: today }];
  const poll = async () => {
    for (const [id, timer] of [...ui.timers.entries()]) if (timer.delay === 5000 || timer.delay === 30000) { ui.timers.delete(id); timer.fn(); }
    await flush();
    for (const request of ui.pending('/visualizer/api/pairings')) { request.done = true; request.reply(200, []); }
    for (const request of ui.pending('/visualizer/api/devices')) { request.done = true; request.reply(200, renamed); }
    await flush();
  };
  await poll();
  assert.ok(!ui.rows('Renamed elsewhere'), 'held back while typing');
  field.blur();
  await poll();
  assert.ok(ui.rows('Renamed elsewhere'), 'shown once typing ends');
});

test('an index change held back while typing a device name shows after focus leaves', async () => {
  const ui = await signedIn();
  ui.hashChange('#page=agents');
  await flush();
  ui.rows('mac').click();
  await flush();
  const field = ui.document.getElementById('device-name');
  field.focus();
  const fire = async delay => { for (const [id, timer] of [...ui.timers.entries()]) if (timer.delay === delay) { ui.timers.delete(id); timer.fn(); } await flush(); };
  await fire(15000);
  for (const request of ui.pending('/visualizer/api/context')) { request.done = true; request.reply(200, { records: projectRecords, projects: [PROJECT], review_count: 3, server: { version: '2.9.0' }, total: 4 }); }
  await flush();
  assert.doesNotMatch(ui.text(), /New3/, 'held back while typing');
  field.blur();
  // The next refresh brings nothing new, yet draws what was held back.
  await fire(5000); await fire(30000);
  for (const request of ui.pending('/visualizer/api/pairings')) { request.done = true; request.reply(200, []); }
  for (const request of ui.pending('/visualizer/api/devices')) { request.done = true; request.reply(200, [{ id: 7, device: 'mac', created_at: today, revoked_at: null, last_seen_at: today }]); }
  await flush();
  assert.match(ui.text(), /New3/);
});

test('an opened memory lists where it applies, whether it loads at startup, and who saved it', async () => {
  const ui = await signedIn();
  const panel = () => ui.app.querySelector('aside');
  const facts = () => Object.fromEntries(panel().querySelectorAll('section').map(sec => [sec.querySelector('h2').textContent, sec.querySelector('p')?.textContent]));
  await openSquash(ui);
  assert.ok(!ui.app.querySelector('p.about'), 'no prose sentence');
  const labels = panel().querySelectorAll('h2.sec-label').map(node => node.textContent);
  assert.deepEqual(labels.slice(0, 3), ['Applies to', 'Startup', 'Saved by'], 'facts in a fixed order, with no kind');
  assert.equal(facts().Startup, 'Loads');
  ui.rows('Use gh for PR status').click();
  await ui.answer('/visualizer/api/record', 200, projectRecords[1]);
  assert.equal(facts().Startup, 'Not until you keep it');
  ui.rows('Squash merge only').click();
  await ui.answer('/visualizer/api/record', 200, { ...projectRecords[0], archived: true });
  assert.equal(facts().Startup, 'Not loaded');
  assert.equal(ui.rows('Squash merge only').textContent, 'Squash merge onlyClaude Code · today', 'rows show only source and date');
  assert.equal(ui.rows('Use gh for PR status').textContent, 'Use gh for PR statusCodex · today', 'suggestions too');
  assert.match(ui.rows('Shipped 2.8.5. Next: redesign.').textContent, /handoff · Codex/, 'handoffs still say what they are');
});

test("the owner's own saves show no internal provenance", async () => {
  const ui = await signedIn();
  ui.rows('Squash merge only').click();
  await ui.answer('/visualizer/api/record', 200, { ...projectRecords[0], provenance: { harness: 'memory-view', device: 'owner browser', source: 'Owner action in the memory view' } });
  const panel = ui.app.querySelector('aside').textContent;
  assert.match(panel, /Saved byYou/);
  assert.doesNotMatch(panel, /owner browser|Owner action/);
});

test('history shows the text each version would restore', async () => {
  const ui = await signedIn();
  ui.rows('Squash merge only').click();
  await ui.answer('/visualizer/api/record', 200, { ...projectRecords[0], revision: 2, content: 'Squash merge only. Always.' });
  await ui.answer('/visualizer/api/record', 200, { ...projectRecords[0], content: 'Rebase is fine too.' });
  const history = ui.app.querySelector('aside').querySelectorAll('section').find(sec => sec.querySelector('h2').textContent === 'History').textContent;
  assert.doesNotMatch(history, /Squash merge only\. Always\./, 'the current text is shown above, not repeated');
  assert.match(history, /Rebase is fine too\./, 'an older text that differs only in content');
  assert.ok(ui.find('Restore version from today'), 'each earlier version can be restored');
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
  ui.document.getElementById('move-to').value = '';
  ui.find('Move').click();
  await flush();
  const moved = ui.requests.find(request => request.url === '/visualizer/api/move');
  assert.deepEqual({ ...moved.body, request_id: undefined }, { id: 1, expected_revision: 1, project: null, device: null, platform: null, request_id: undefined });
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

test('large suggestion lists compare only the opened memory', async () => {
  for (const route of [`#page=project&project=${encodeURIComponent(PROJECT)}`, '#page=all', '#page=new']) {
    const ui = setup({ session: true, hash: route });
    const scope = route === '#page=all' ? {} : { project: PROJECT };
    let contentReads = 0;
    const records = Array.from({ length: 4000 }, (_, i) => {
      const record = memory(i + 1, `Synthetic memory ${i + 1}`, { scope, confirmed: i < 2000 });
      Object.defineProperty(record, 'content', { get() { contentReads++; return 'Short interface help text.'; } });
      return record;
    });
    await flush();
    await ui.answer('/visualizer/api/context', 200, { records, projects: [PROJECT], review_count: 2000, total: records.length });
    await ui.answer('/visualizer/api/context', 200, { records: route === '#page=new' ? records.slice(2000) : records, total: records.length });
    assert.ok(contentReads <= records.length * 4, `${route}: loading must read a linear number of bodies`);
    contentReads = 0;
    vm.runInContext('render()', ui.sandbox);
    assert.equal(contentReads, 0, `${route}: list redraws must not compare memory bodies`);
    assert.ok(ui.find('Keep Synthetic memory 2001'));
    assert.ok(ui.find('Discard Synthetic memory 2001'));
    ui.rows('Synthetic memory 2001').click();
    await ui.answer('/visualizer/api/record', 200, records[2000]);
    assert.ok(ui.find('Keep and replace it'), 'opening a suggestion still offers its similar kept memory');
    assert.ok(ui.app.querySelectorAll('h2.sec-label').some(node => node.textContent === 'Similar'));
    assert.ok(contentReads > 0 && contentReads < 20000, 'one open comparison reads a linear number of bodies');
  }
});

test('similarity keeps project scope, unique words, exclusions and first-on-tie behavior', () => {
  const similar = vm.runInContext('similarTo', setup().sandbox);
  const suggestion = memory(10, 'CAFÉ metric units', { content: 'Grams grams.', confirmed: false });
  const global = memory(1, 'café metric units', { scope: {} });
  const tied = memory(2, 'café metric units');
  const excluded = [suggestion, memory(3, 'café metric units grams', { confirmed: false }),
    memory(4, 'café metric units grams', { purpose: 'handoff' }),
    memory(5, 'café metric units grams', { scope: { project: 'id:elsewhere' } })];
  assert.equal(similar(suggestion, [...excluded, global, tied])?.id, global.id);
  assert.equal(similar({ ...suggestion, scope: {} }, [tied, global])?.id, global.id);
  assert.equal(similar(memory(11, 'grams grams grams', { content: '' }), [memory(12, 'grams grams grams', { content: '' })]), null);
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

test('Move to starts at where the memory applies now and saves nothing until that changes', async () => {
  const ui = await signedIn();
  await openSquash(ui);
  ui.find('Move to…').click();
  const where = ui.document.getElementById('move-to');
  assert.equal(where.querySelectorAll('option').find(node => node.getAttribute('selected') !== null)?.value, PROJECT);
  where.value = PROJECT;
  ui.find('Move').click();
  await flush();
  assert.equal(ui.pending('/visualizer/api/move').length, 0);
  assert.match(ui.text(), /Pick another place/);
  ui.app.querySelector('form.edit').dispatch('keydown', { key: 'Escape' });
  assert.ok(ui.find('Move to…'), 'Escape goes back to the memory');
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
  const agents = ui.app.querySelectorAll('.nav').find(node => node.textContent.startsWith('Agents'));
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

test('Add memory saves a kept memory for the project on screen, then opens it', async () => {
  const ui = await signedIn();
  ui.find('Add memory').click();
  assert.equal(ui.document.activeElement, ui.document.getElementById('edit-title'), 'focus starts at the title');
  assert.equal(ui.document.getElementById('edit-project').querySelectorAll('option').find(node => node.getAttribute('selected') !== null)?.value, PROJECT, 'it applies to the project on screen by default');
  ui.document.getElementById('edit-title').value = 'Short answers';
  ui.document.getElementById('edit-text').value = 'Prefer short answers.';
  ui.document.getElementById('edit-project').value = PROJECT;
  ui.find('Save').click();
  const create = await ui.answer('/visualizer/api/create', 200, { id: 9, revision: 1 });
  assert.deepEqual({ ...create.body, request_id: undefined }, { project: PROJECT, title: 'Short answers', content: 'Prefer short answers.', request_id: undefined });
  const opened = ui.pending('/visualizer/api/record')[0];
  assert.equal(opened?.body.id, 9, 'the new memory opens');
});

test('Add memory can apply to every project, and asks before dropping typed text', async () => {
  const ui = await signedIn();
  ui.find('Add memory').click();
  const where = ui.document.getElementById('edit-project');
  where.value = '';
  where.dispatch('change');
  ui.document.getElementById('edit-text').value = 'Prefer short answers.';
  let asked = 0;
  ui.sandbox.window.confirm = () => { asked++; return false; };
  ui.find('Cancel').click();
  assert.equal(asked, 1, 'typed text is not dropped silently');
  ui.find('Save').click();
  const create = await ui.answer('/visualizer/api/create', 200, { id: 9, revision: 1 });
  assert.equal(create.body.project, null);
});

test('Move to offers every other project as well as All projects', async () => {
  const OTHER = 'git:github.com/example/tempus';
  const ui = setup({ session: true, hash: `#page=project&project=${encodeURIComponent(PROJECT)}` });
  await flush();
  await ui.answer('/visualizer/api/context', 200, { records: projectRecords, projects: [PROJECT, OTHER, 'legacy-folder'], review_count: 1, server: { version: '2.9.10' }, total: 4 });
  await ui.answer('/visualizer/api/context', 200, { records: projectRecords, projects: [PROJECT, OTHER], total: 4 });
  await openSquash(ui);
  ui.find('Move to…').click();
  assert.deepEqual(ui.document.getElementById('move-to').querySelectorAll('option').map(node => node.textContent), ['All projects', 'grasshopper', 'tempus'], 'by name, never a folder name');
  ui.document.getElementById('move-to').value = OTHER;
  ui.find('Move').click();
  await flush();
  assert.equal(ui.requests.find(request => request.url === '/visualizer/api/move').body.project, OTHER);
});

test('in an edit, Cmd or Ctrl+Enter saves and Escape cancels', async () => {
  const ui = await signedIn();
  await openSquash(ui);
  ui.find('Edit').click();
  const form = ui.app.querySelector('form.edit');
  form.dispatch('keydown', { key: 'Escape' });
  assert.ok(ui.find('Edit') && !ui.app.querySelector('form.edit'), 'Escape leaves an unchanged edit');
  ui.find('Edit').click();
  ui.document.getElementById('edit-text').value = 'Squash merge only. Always.';
  ui.app.querySelector('form.edit').dispatch('keydown', { key: 'Enter', metaKey: true });
  await flush();
  assert.equal(ui.pending('/visualizer/api/update')[0]?.body.content, 'Squash merge only. Always.');
});

test('large lists keep saying where they apply', async () => {
  const records = Array.from({ length: 9 }, (_, i) => memory(i + 1, `Project rule ${i + 1}`));
  const ui = setup({ session: true, hash: `#page=project&project=${encodeURIComponent(PROJECT)}` });
  await flush();
  await ui.answer('/visualizer/api/context', 200, { records, projects: [PROJECT], review_count: 0, server: { version: '2.9.10' }, total: 9 });
  await ui.answer('/visualizer/api/context', 200, { records, projects: [PROJECT], total: 9 });
  assert.deepEqual(ui.app.querySelectorAll('h2.sec-label').map(node => node.textContent), ['grasshopper · This week']);
});

test('New lists projects by name, so deciding one never reshuffles the rest', async () => {
  const OTHER = 'git:github.com/example/apples';
  const older = new Date(Date.now() - 86400000).toISOString();
  const records = [memory(1, 'Grasshopper idea', { confirmed: false, purpose: 'observation' }), memory(2, 'Apples idea', { confirmed: false, purpose: 'observation', scope: { project: OTHER }, updated_at: older })];
  const ui = setup({ session: true, hash: '#page=new' });
  await flush();
  await ui.answer('/visualizer/api/context', 200, { records, projects: [PROJECT, OTHER], review_count: 2, server: { version: '2.9.10' }, total: 2 });
  await ui.answer('/visualizer/api/context', 200, { records, projects: [PROJECT, OTHER], total: 2 });
  assert.deepEqual(ui.app.querySelectorAll('h2.sec-label').map(node => node.textContent), ['apples', 'grasshopper']);
});

test('History appears only when an earlier version differs', async () => {
  const ui = await signedIn();
  ui.rows('Squash merge only').click();
  await ui.answer('/visualizer/api/record', 200, { ...projectRecords[0], revision: 2 });
  await ui.answer('/visualizer/api/record', 200, projectRecords[0]);
  assert.ok(!ui.app.querySelector('aside').querySelectorAll('h2').some(node => node.textContent === 'History'), 'a kept suggestion has nothing to restore');
});

test('memories and places are links, so they also open in a new tab', async () => {
  const ui = await signedIn();
  const link = ui.rows('Squash merge only');
  assert.equal(link.tagName, 'A');
  assert.equal(link.getAttribute('href'), `#${new URLSearchParams({ page: 'project', project: PROJECT, memory: '1' })}`);
  assert.equal(agentsLink(ui).tagName, 'A');
  assert.equal(agentsLink(ui).getAttribute('href'), '#page=agents');
  link.dispatch('click', { metaKey: true });
  link.dispatch('click', { button: 1 });
  assert.equal(ui.pending('/visualizer/api/record').length, 0, 'a modified or middle click is left to the browser');
  let prevented = false;
  link.dispatch('click', { button: 0, preventDefault() { prevented = true; } });
  assert.ok(prevented, 'a plain click opens in place');
  await ui.answer('/visualizer/api/record', 200, projectRecords[0]);
  assert.ok(ui.app.querySelector('aside.panel'));
});

test('the list keeps its scroll across redraws and starts at the top on another page', async () => {
  const ui = await signedIn();
  ui.app.querySelector('main').scrollTop = 300;
  await openSquash(ui);
  assert.equal(ui.app.querySelector('main').scrollTop, 300, 'opening a memory keeps the list where it was');
  agentsLink(ui).click();
  assert.equal(ui.app.querySelector('main').scrollTop || 0, 0);
});

test('Escape closes an open memory and focus returns to its row', async () => {
  const ui = await signedIn();
  await openSquash(ui);
  ui.keydown('Escape');
  assert.equal(ui.app.querySelector('aside'), null, 'the panel closed');
  assert.equal(ui.document.activeElement?.getAttribute('data-key'), 'open:1', 'focus is back on the row');
  await openSquash(ui);
  ui.find('Close').click();
  assert.equal(ui.document.activeElement?.getAttribute('data-key'), 'open:1', 'Close returns focus too');
  await openSquash(ui);
  ui.find('Edit').click();
  ui.keydown('Escape');
  assert.ok(ui.app.querySelector('aside'), 'an edit is left to its own Escape');
});

// Seven projects, newest first by their memory's date: p1 is the most recent.
function projectSet() {
  const projects = ['p1', 'p2', 'p3', 'p4', 'p5', 'zeta', 'alpha'].map(name => `git:github.com/example/${name}`);
  const records = projects.map((project, i) => memory(i + 1, `Rule ${i + 1}`, { scope: { project }, updated_at: new Date(Date.now() - i * 3600000).toISOString() }));
  return { projects, records };
}
const navNames = ui => ui.app.querySelector('nav.side').querySelectorAll('.nav').map(node => node.textContent.trim());

test('the sidebar shows the five most recent projects, then the rest by name', async () => {
  const { projects, records } = projectSet();
  const ui = setup({ session: true, hash: '#page=all' });
  await flush();
  await ui.answer('/visualizer/api/context', 200, { records, projects, review_count: 0, server: { version: '2.9.11' }, total: 7 });
  assert.deepEqual(navNames(ui).slice(0, 7), ['All projects', 'p1', 'p2', 'p3', 'p4', 'p5', '2 more']);
  ui.find('2 more').click();
  assert.deepEqual(navNames(ui).slice(6, 9), ['alpha', 'zeta', 'Fewer'], 'the rest are easy to scan');
});

test('the project on screen always shows in the sidebar', async () => {
  const { projects, records } = projectSet();
  const ui = setup({ session: true, hash: `#page=project&project=${encodeURIComponent(projects[6])}` });
  await flush();
  await ui.answer('/visualizer/api/context', 200, { records, projects, review_count: 0, server: { version: '2.9.11' }, total: 7 });
  assert.ok(navNames(ui).includes('alpha'), 'alpha is older than the top five but is open');
  assert.ok(navNames(ui).includes('1 more'));
});

test('the sidebar keeps its order while the owner works', async () => {
  const { projects, records } = projectSet();
  const ui = setup({ session: true, hash: '#page=all' });
  await flush();
  await ui.answer('/visualizer/api/context', 200, { records, projects, review_count: 0, server: { version: '2.9.11' }, total: 7 });
  // p5 changes most recently; the next index refresh must not move it to the top.
  const changed = records.map(record => record.id === 5 ? { ...record, updated_at: new Date(Date.now() + 60000).toISOString() } : record);
  for (const [id, timer] of [...ui.timers.entries()]) if (timer.delay === 15000) { ui.timers.delete(id); timer.fn(); }
  await flush();
  for (const request of ui.pending('/visualizer/api/context')) { request.done = true; request.reply(200, { records: changed, projects, review_count: 0, server: { version: '2.9.11' }, total: 7 }); }
  await flush();
  assert.deepEqual(navNames(ui).slice(1, 6), ['p1', 'p2', 'p3', 'p4', 'p5']);
});

test('Move to keeps the current project as an option even when it is not in the project list', async () => {
  const OLD = 'git:github.com/example/not-listed';
  const ui = await signedIn();
  ui.rows('Squash merge only').click();
  await ui.answer('/visualizer/api/record', 200, { ...projectRecords[0], scope: { project: OLD } });
  ui.find('Move to…').click();
  assert.equal(ui.document.getElementById('move-to').querySelectorAll('option').find(node => node.getAttribute('selected') !== null)?.value, OLD);
});

test('projects that appear later also keep their sidebar place', async () => {
  const { projects, records } = projectSet();
  const ui = setup({ session: true, hash: '#page=all' });
  await flush();
  await ui.answer('/visualizer/api/context', 200, { records: records.slice(0, 2), projects: projects.slice(0, 2), review_count: 0, server: { version: '2.10.0' }, total: 2 });
  const refresh = async list => {
    for (const [id, timer] of [...ui.timers.entries()]) if (timer.delay === 15000) { ui.timers.delete(id); timer.fn(); }
    await flush();
    for (const request of ui.pending('/visualizer/api/context')) if (request.body?.all_projects && !request.body.view) { request.done = true; request.reply(200, { records: list, projects: projects.slice(0, 4), review_count: 0, server: { version: '2.10.0' }, total: list.length }); }
    await flush();
  };
  await refresh(records.slice(0, 4));
  const before = navNames(ui).slice(1, 5);
  await refresh(records.slice(0, 4).map(record => record.id === 4 ? { ...record, updated_at: new Date(Date.now() + 60000).toISOString() } : record));
  assert.deepEqual(navNames(ui).slice(1, 5), before);
});

test('an Add memory draft survives the session ending and signing in again', async () => {
  const ui = await signedIn();
  ui.find('Add memory').click();
  ui.document.getElementById('edit-title').value = 'Short answers';
  ui.document.getElementById('edit-text').value = 'Prefer short answers.';
  for (const [id, timer] of [...ui.timers.entries()]) if (timer.delay === 15000) { ui.timers.delete(id); timer.fn(); }
  await flush();
  await ui.answer('/visualizer/api/context', 401);
  assert.match(ui.text(), /unsaved text is kept/);
  ui.app.querySelector('#token').value = TOKEN;
  ui.app.querySelector('form.signin-box').dispatch('submit');
  await ui.answer('/visualizer/api/session', 200, {});
  assert.equal(ui.document.getElementById('edit-text')?.value, 'Prefer short answers.');
  assert.equal(ui.document.getElementById('edit-title')?.value, 'Short answers');
  const where = ui.document.getElementById('edit-project');
  assert.equal(where.querySelectorAll('option').find(node => node.getAttribute('selected') !== null)?.value, PROJECT, 'still for the project, not every project');
});

test('an empty store offers Add memory', async () => {
  const ui = setup({ session: true, hash: '#page=all' });
  await flush();
  await ui.answer('/visualizer/api/context', 200, { records: [], projects: [], review_count: 0, server: { version: '2.10.0' }, total: 0 });
  await ui.answer('/visualizer/api/context', 200, { records: [], projects: [], total: 0 });
  assert.ok(ui.find('Add memory'));
});

test('Add memory opened during an edit starts blank', async () => {
  const ui = await signedIn();
  await openSquash(ui);
  ui.find('Edit').click();
  ui.document.getElementById('edit-text').value = 'Discard me.';
  ui.find('Add memory').click();
  assert.equal(ui.document.getElementById('edit-text').value, '');
  assert.equal(ui.document.getElementById('edit-title').value, '');
});

test('a long history stays drawn while it reloads after a save', async () => {
  const ui = await signedIn();
  const long = { ...projectRecords[0], revision: 7, content: 'Version 7.' };
  ui.rows('Squash merge only').click();
  await ui.answer('/visualizer/api/record', 200, long);
  for (let revision = 6; revision >= 2; revision--) await ui.answer('/visualizer/api/record', 200, { ...long, revision, content: `Version ${revision}.` });
  assert.ok(ui.find('Show older versions'));
  ui.find('Edit').click();
  ui.document.getElementById('edit-text').value = 'Version 8.';
  ui.find('Save').click();
  await ui.answer('/visualizer/api/update', 200, { id: 1, revision: 8 });
  await ui.answer('/visualizer/api/record', 200, { ...long, revision: 8, content: 'Version 8.' });
  assert.match(ui.app.querySelector('aside').textContent, /Version 8\./, 'the panel still draws');
});

test('a project without a durable ID does not offer Add memory', async () => {
  const LEGACY = 'old-folder-name';
  const records = [memory(1, 'Old rule', { scope: { project: LEGACY } })];
  const ui = setup({ session: true, hash: `#page=project&project=${LEGACY}` });
  await flush();
  await ui.answer('/visualizer/api/context', 200, { records, projects: [LEGACY], review_count: 0, server: { version: '2.10.0' }, total: 1 });
  await ui.answer('/visualizer/api/context', 200, { records, projects: [LEGACY], total: 1 });
  assert.ok(!ui.find('Add memory'), 'it could only save to every project');
});

test('the open project stays in the sidebar after its last memory leaves', async () => {
  const OTHER = 'git:github.com/example/emptied';
  const ui = setup({ session: true, hash: `#page=project&project=${encodeURIComponent(OTHER)}` });
  await flush();
  await ui.answer('/visualizer/api/context', 200, { records: projectRecords, projects: [PROJECT], review_count: 0, server: { version: '2.10.0' }, total: 4 });
  assert.ok(navNames(ui).includes('emptied'));
});

test('a failed move retries to the same place with the same request', async () => {
  const OTHER = 'git:github.com/example/tempus';
  const ui = setup({ session: true, hash: `#page=project&project=${encodeURIComponent(PROJECT)}` });
  await flush();
  await ui.answer('/visualizer/api/context', 200, { records: projectRecords, projects: [PROJECT, OTHER], review_count: 1, server: { version: '2.10.0' }, total: 4 });
  await ui.answer('/visualizer/api/context', 200, { records: projectRecords, projects: [PROJECT, OTHER], total: 4 });
  await openSquash(ui);
  ui.find('Move to…').click();
  const where = ui.document.getElementById('move-to');
  where.value = OTHER;
  where.dispatch('change');
  ui.find('Move').click();
  await flush();
  const first = ui.pending('/visualizer/api/move')[0];
  first.done = true; first.fail();
  await flush();
  ui.find('Move').click();
  await flush();
  const retry = ui.pending('/visualizer/api/move')[0];
  assert.equal(retry?.body.project, OTHER);
  assert.equal(retry.body.request_id, first.body.request_id);
});

test('a long earlier version can be read in full before restoring it', async () => {
  const ui = await signedIn();
  const older = `${'Older rule text. '.repeat(15)}The changed instruction is at the end.`;
  ui.rows('Squash merge only').click();
  await ui.answer('/visualizer/api/record', 200, { ...projectRecords[0], revision: 2, content: 'Current text.' });
  await ui.answer('/visualizer/api/record', 200, { ...projectRecords[0], content: older });
  const more = ui.find('Show all');
  assert.equal(more?.getAttribute('aria-expanded'), 'false', 'a long version starts short');
  more.click();
  assert.equal(ui.find('Show less')?.getAttribute('aria-expanded'), 'true');
  assert.match(ui.app.querySelector('aside').textContent, /The changed instruction is at the end\./);
});

test('an opened long version stays open across a refresh', async () => {
  const ui = await signedIn();
  const older = `${'Older rule text. '.repeat(15)}End.`;
  ui.rows('Squash merge only').click();
  await ui.answer('/visualizer/api/record', 200, { ...projectRecords[0], revision: 2, content: 'Current text.' });
  await ui.answer('/visualizer/api/record', 200, { ...projectRecords[0], content: older });
  ui.find('Show all').click();
  for (const [id, timer] of [...ui.timers.entries()]) if (timer.delay === 15000) { ui.timers.delete(id); timer.fn(); }
  await flush();
  for (const request of ui.pending('/visualizer/api/context')) { request.done = true; request.reply(200, { records: projectRecords, projects: [PROJECT], review_count: 2, server: { version: '2.10.0' }, total: 4 }); }
  await flush();
  assert.ok(ui.find('Show less'), 'still open');
});

test('cancelling Add memory over an open memory forgets that memory in the address', async () => {
  const ui = await signedIn();
  await openSquash(ui);
  assert.match(ui.location.hash, /memory=1/);
  ui.find('Add memory').click();
  ui.find('Cancel').click();
  assert.doesNotMatch(ui.location.hash, /memory=1/);
});

test('Move to changes or lifts a device or OS restriction', async () => {
  const records = [memory(1, 'Worktrees are noise', { scope: { device: 'mac', platform: 'macos' } })];
  const ui = setup({ session: true, hash: '#page=all' });
  await flush();
  await ui.answer('/visualizer/api/context', 200, { records, projects: [PROJECT], review_count: 0, server: { version: '2.10.1' }, total: 1 });
  await ui.answer('/visualizer/api/context', 200, { records, projects: [PROJECT], total: 1 });
  await ui.answer('/visualizer/api/pairings', 200, []);
  await ui.answer('/visualizer/api/devices', 200, [{ id: 7, device: 'mac', label: 'Studio Mac', created_at: today, revoked_at: null, last_seen_at: today }, { id: 8, device: 'pc', created_at: today, revoked_at: null, last_seen_at: today }]);
  ui.rows('Worktrees are noise').click();
  await ui.answer('/visualizer/api/record', 200, records[0]);
  ui.find('Move to…').click();
  const selected = id => ui.document.getElementById(id).querySelectorAll('option').find(node => node.getAttribute('selected') !== null)?.value;
  assert.equal(selected('move-device'), 'mac', 'starts at the current device');
  assert.equal(selected('move-platform'), 'macos');
  assert.deepEqual(ui.document.getElementById('move-device').querySelectorAll('option').map(node => node.textContent), ['Every device', 'Studio Mac', 'pc']);
  const choose = (id, value) => { const field = ui.document.getElementById(id); field.value = value; field.dispatch('change'); };
  choose('move-device', '');
  choose('move-platform', '');
  ui.find('Move').click();
  await flush();
  const moved = ui.pending('/visualizer/api/move')[0];
  assert.deepEqual([moved.body.project, moved.body.device, moved.body.platform], [null, null, null], 'applies on every device');
});

test('a handoff can be moved like any memory', async () => {
  const ui = await signedIn();
  ui.rows('Shipped 2.8.5. Next: redesign.').click();
  await ui.answer('/visualizer/api/record', 200, projectRecords[3]);
  assert.ok(ui.find('Move to…'));
  assert.doesNotMatch(ui.app.querySelector('aside').textContent, /Handoffs stay/);
});

// ---------- memory view data fixes ----------

const OUTSIDE = 'git:github.com/example/outside-index';
const fire = async (ui, delay) => { for (const [id, timer] of [...ui.timers.entries()]) if (timer.delay === delay) { ui.timers.delete(id); timer.fn(); } await flush(); };
const replyAll = async (ui, url, data) => { await flush(); for (const request of ui.pending(url)) { request.done = true; request.reply(200, data); } await flush(); };

test('Archived, New and Search list projects the index leaves out', async () => {
  const ui = await signedIn();
  ui.hashChange('#page=archived');
  await flush();
  const archived = ui.pending('/visualizer/api/context').find(request => request.body?.view === 'archived');
  archived.done = true; archived.reply(200, { records: [memory(30, 'Last one archived', { archived: true, scope: { project: OUTSIDE } })], projects: [PROJECT], total: 1 });
  await flush();
  assert.ok(ui.rows('Last one archived'), 'an archived-only project still shows on Archived');
  assert.ok(ui.app.querySelectorAll('h2.sec-label').some(node => node.textContent === 'outside-index'));

  const fresh = setup({ session: true, hash: '#page=new' });
  await flush();
  await fresh.answer('/visualizer/api/context', 200, { records: [], projects: [PROJECT], review_count: 1, server: { version: '2.11.1' }, total: 0 });
  await fresh.answer('/visualizer/api/context', 200, { records: [memory(31, 'Brand-new project idea', { confirmed: false, purpose: 'observation', scope: { project: OUTSIDE } })], projects: [PROJECT], total: 1 });
  assert.ok(fresh.rows('Brand-new project idea'), "a new project's first suggestion shows on New");

  const search = setup({ session: true, hash: '#page=search&query=idea' });
  await flush();
  await search.answer('/visualizer/api/context', 200, { records: [], projects: [PROJECT], review_count: 0, server: { version: '2.11.1' }, total: 0 });
  await search.answer('/visualizer/api/search', 200, { records: [memory(32, 'Found outside the index', { scope: { project: OUTSIDE } })] });
  assert.ok(search.rows('Found outside the index'), 'a match in an unlisted project shows');
});

test('a kept edit draft opens as a conflict when the memory changed while signed out', async () => {
  const ui = await signedIn();
  await openSquash(ui);
  ui.find('Edit').click();
  ui.document.getElementById('edit-text').value = 'Squash merge only. Always.';
  await fire(ui, 15000);
  await ui.answer('/visualizer/api/context', 401);
  ui.app.querySelector('#token').value = TOKEN;
  ui.app.querySelector('form.signin-box').dispatch('submit');
  await ui.answer('/visualizer/api/session', 200, {});
  await ui.answer('/visualizer/api/record', 200, { ...projectRecords[0], revision: 2, content: 'Squash or rebase.' });
  assert.match(ui.text(), /changed this while you were editing\. Nothing was overwritten\./);
  assert.match(ui.text(), /Squash or rebase\./);
  assert.match(ui.text(), /Squash merge only\. Always\./);
  ui.find('Keep yours').click();
  await flush();
  const save = ui.pending('/visualizer/api/update')[0];
  assert.equal(save.body.expected_revision, 2, 'saving yours is an explicit choice against the newer revision');
  assert.equal(save.body.content, 'Squash merge only. Always.');
});

test('leaving the page with an unsaved edit asks first', async () => {
  const ui = await signedIn();
  assert.equal(ui.beforeUnload().defaultPrevented, false, 'nothing to lose, no prompt');
  await openSquash(ui);
  ui.find('Edit').click();
  ui.document.getElementById('edit-text').value = 'Squash merge only. Always.';
  assert.equal(ui.beforeUnload().defaultPrevented, true);
});

test('a failed poll does not rebuild the edit form', async () => {
  const ui = await signedIn();
  await openSquash(ui);
  ui.find('Edit').click();
  const field = ui.document.getElementById('edit-text');
  field.value = 'Squash merge only. Always.';
  await fire(ui, 5000);
  ui.pending('/visualizer/api/context').forEach(request => { request.done = true; request.fail(); });
  await flush();
  assert.equal(ui.document.getElementById('edit-text'), field, 'a page poll failure keeps the textarea');
  await fire(ui, 15000);
  ui.pending('/visualizer/api/context').forEach(request => { request.done = true; request.fail(); });
  await flush();
  assert.equal(ui.document.getElementById('edit-text'), field, 'an index poll failure keeps the textarea');
});

test('Back and Forward over an opened memory keep the list loaded', async () => {
  const ui = await signedIn();
  const list = `#page=project&project=${encodeURIComponent(PROJECT)}`;
  await openSquash(ui);
  const loads = () => ui.requests.filter(request => request.url === '/visualizer/api/context').length;
  const before = loads();
  ui.hashChange(list);
  await flush();
  assert.equal(ui.app.querySelector('aside'), null, 'Back closes the memory');
  assert.ok(ui.rows('Squash merge only'), 'the list stays drawn');
  assert.equal(loads(), before, 'nothing reloads');
  ui.hashChange(`${list}&memory=1`);
  await ui.answer('/visualizer/api/record', 200, projectRecords[0]);
  assert.ok(ui.app.querySelector('aside'), 'Forward reopens it');
  assert.equal(loads(), before);
});

test('closing a memory replaces its history entry, and Back from an edit asks first', async () => {
  const ui = await signedIn();
  await openSquash(ui);
  assert.equal(ui.history.length, 1);
  ui.find('Close').click();
  assert.equal(ui.history.length, 1, 'Back does not reopen the closed memory');
  assert.doesNotMatch(ui.location.hash, /memory=/);
  await openSquash(ui);
  ui.find('Edit').click();
  ui.document.getElementById('edit-text').value = 'Squash merge only. Always.';
  let asked = 0;
  ui.sandbox.window.confirm = () => { asked++; return false; };
  ui.hashChange(`#page=project&project=${encodeURIComponent(PROJECT)}`);
  await flush();
  assert.equal(asked, 1);
  assert.equal(ui.document.getElementById('edit-text').value, 'Squash merge only. Always.', 'declining keeps the edit');
  assert.match(ui.location.hash, /memory=1/, 'the address goes back to the memory');
});

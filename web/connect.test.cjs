const assert = require('node:assert/strict');
const { readFileSync } = require('node:fs');
const { test } = require('node:test');
const vm = require('node:vm');

function submit(value) {
  const address = { value };
  const error = { hidden: true, textContent: '' };
  let handler;
  let destination;
  vm.runInNewContext(readFileSync(`${__dirname}/public/connect.js`, 'utf8'), {
    URL,
    document: { getElementById: id => ({ 'server-address': address, 'server-error': error, 'server-form': { addEventListener: (_, fn) => { handler = fn; } } })[id] },
    localStorage: { getItem: () => null, setItem() {} },
    window: { location: { assign: url => { destination = url; } } },
  });
  handler({ preventDefault() {} });
  return { error, destination };
}

test('nonsense addresses explain the expected server address without navigation', () => {
  for (const value of ['', 'not a url', 'not-a-server', 'https://memory..example.com', 'https://-bad.example.com']) {
    const { error, destination } = submit(value);
    assert.equal(destination, undefined, value);
    assert.equal(error.hidden, false, value);
    assert.match(error.textContent, /server.*address/i, value);
  }
});

test('server links normalize common formats and keep local HTTP available', () => {
  for (const [value, origin] of [
    ['memory.example.com', 'https://memory.example.com'],
    ['https://memory.example.com/mcp', 'https://memory.example.com'],
    ['https://memory.example.com/visualizer/', 'https://memory.example.com'],
    ['http://localhost:8199', 'http://localhost:8199'],
    ['http://127.0.0.1:8199', 'http://127.0.0.1:8199'],
    ['http://[::1]:8199', 'http://[::1]:8199'],
    ['https://[fd00::1]', 'https://[fd00::1]'],
    ['https://private-server', 'https://private-server'],
    ['HTTPS://PRIVATE-SERVER', 'https://private-server'],
  ]) {
    assert.equal(submit(value).destination, `${origin}/visualizer/`, value);
  }
});

test('unsafe or unrelated links remain rejected', () => {
  for (const value of ['http://memory.example.com', 'https://user:pass@memory.example.com', 'https://memory.example.com/?token=x', 'https://memory.example.com/?', 'https://memory.example.com/#x', 'https://memory.example.com/other']) {
    assert.equal(submit(value).destination, undefined, value);
    assert.equal(submit(value).error.hidden, false, value);
  }
});

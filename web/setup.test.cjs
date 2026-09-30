const assert = require('node:assert/strict');
const { readFileSync } = require('node:fs');
const { test } = require('node:test');
const vm = require('node:vm');

function setup(writeText) {
  const osChoice = { value: 'macos', addEventListener(_, fn) { this.change = fn; } };
  const commands = [
    { textContent: 'codex plugin add grasshopper-macos@grasshopper-marketplace' },
    { textContent: 'claude plugin install grasshopper-macos@grasshopper-marketplace' },
  ];
  const button = { previousElementSibling: commands[0], addEventListener(_, fn) { this.click = fn; } };
  const cursorPlugin = { textContent: 'grasshopper-macos' };
  const archiveCommand = { textContent: '' };
  const copyStatus = { textContent: '' };
  let selected;
  vm.runInNewContext(readFileSync(`${__dirname}/public/setup.js`, 'utf8'), {
    navigator: { clipboard: { writeText } },
    window: { getSelection: () => ({ removeAllRanges() {}, addRange() {} }) },
    document: {
      getElementById: id => ({ 'connector-os': osChoice, 'cursor-plugin': cursorPlugin, 'archive-connect-command': archiveCommand, 'copy-status': copyStatus })[id],
      querySelectorAll: selector => selector === '[data-plugin-command]' ? commands : [button],
      createRange: () => ({ selectNodeContents: node => { selected = node.textContent; } }),
    },
  });
  return { osChoice, commands, button, cursorPlugin, archiveCommand, copyStatus, selected: () => selected };
}

test('OS choice rewrites all agent plugin names and preserves marketplace names', () => {
  const ui = setup(async () => {});
  for (const os of ['windows', 'linux', 'macos']) {
    ui.osChoice.value = os;
    ui.osChoice.change();
    for (const command of ui.commands) assert.match(command.textContent, new RegExp(`grasshopper-${os}@grasshopper-marketplace$`));
    assert.equal(ui.cursorPlugin.textContent, `grasshopper-${os}`);
    assert.equal(ui.archiveCommand.textContent.startsWith(os === 'windows' ? '.\\bin\\grasshopper.exe' : './bin/grasshopper'), true);
  }
});

test('Copy uses the selected OS command', async () => {
  let copied;
  const ui = setup(async value => { copied = value; });
  ui.osChoice.value = 'windows';
  ui.osChoice.change();
  await ui.button.click();
  assert.equal(copied, ui.commands[0].textContent);
  assert.equal(ui.button.textContent, 'Copied');
});

test('clipboard failure selects the command and explains manual copying', async () => {
  const ui = setup(async () => { throw new Error('permission denied'); });
  await ui.button.click();
  assert.equal(ui.selected(), ui.commands[0].textContent);
  assert.match(ui.copyStatus.textContent, /Use your browser.*Copy command/);
});

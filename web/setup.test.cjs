const assert = require('node:assert/strict');
const { readFileSync } = require('node:fs');
const { test } = require('node:test');
const vm = require('node:vm');

function setup(writeText, script = readFileSync(`${__dirname}/public/setup.js`, 'utf8'), areas = []) {
  const osChoice = { value: 'macos', addEventListener(_, fn) { this.change = fn; } };
  const commands = [
    { textContent: 'codex plugin add grasshopper-macos@grasshopper-marketplace' },
    { textContent: 'claude plugin install grasshopper-macos@grasshopper-marketplace' },
  ];
  const button = { textContent: 'Copy command', previousElementSibling: commands[0], addEventListener(_, fn) { this.click = fn; } };
  const secondButton = { textContent: 'Copy command', previousElementSibling: commands[1], addEventListener(_, fn) { this.click = fn; } };
  const serverChecksums = { href: '' };
  const cursorPlugin = { textContent: 'grasshopper-macos' };
  const archiveCommand = { textContent: '' };
  const copyStatus = { textContent: '' };
  const serverCommand = { textContent: './bin/grasshopper-server --quickstart' };
  const serverArchive = { href: '', textContent: '' };
  const timers = new Map();
  let timerID = 0;
  let selected;
  vm.runInNewContext(script.replaceAll('__GRASSHOPPER_RELEASE_VERSION__', '9.8.7'), {
    setTimeout(fn, delay) { timers.set(++timerID, {fn, delay}); return timerID; },
    clearTimeout(id) { timers.delete(id); },
    navigator: { clipboard: { writeText } },
    window: { addEventListener() {}, getSelection: () => ({ removeAllRanges() {}, addRange() {} }) },
    document: {
      getElementById: id => ({ 'connector-os': osChoice, 'cursor-plugin': cursorPlugin, 'archive-connect-command': archiveCommand, 'copy-status': copyStatus, 'server-quickstart-command': serverCommand, 'server-archive': serverArchive, 'server-checksums': serverChecksums })[id],
      querySelectorAll: selector => selector === '[data-plugin-command]' ? commands : selector === '.command pre' ? areas : [button, secondButton],
      createRange: () => ({ selectNodeContents: node => { selected = node.textContent; } }),
    },
  });
  return { osChoice, commands, button, secondButton, serverChecksums, cursorPlugin, archiveCommand, copyStatus, serverCommand, serverArchive, timers, selected: () => selected };
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


test('selected OS chooses its server archive and quickstart executable', () => {
  const ui = setup(async () => {});
  for (const [os,target,binary] of [['macos','darwin-arm64','./bin/grasshopper-server'],['windows','windows-amd64','.\\bin\\grasshopper-server.exe'],['linux','linux-amd64','./bin/grasshopper-server']]) {
    ui.osChoice.value = os; ui.osChoice.change();
    assert.equal(ui.serverCommand.textContent, `${binary} --quickstart`);
    assert.equal(ui.serverArchive.href, `https://github.com/MylesMCook/grasshopper/releases/download/v9.8.7/grasshopper-server-${target}-9.8.7.zip`);
  }
});

test('copy feedback resets after two seconds and on OS change', async () => {
  const ui = setup(async () => {}); await ui.button.click();
  const timer = [...ui.timers.values()][0]; assert.equal(timer.delay, 2000); timer.fn();
  assert.equal(ui.button.textContent, 'Copy command');
  await ui.button.click(); ui.osChoice.value='windows'; ui.osChoice.change();
  assert.equal(ui.button.textContent, 'Copy command'); assert.equal(ui.copyStatus.textContent, '');
  assert.equal(ui.timers.size, 0);
});

test('late clipboard success or failure after OS change cannot restore stale feedback or select old text', async () => {
  for (const fail of [false,true]) {
    let finish; const ui=setup(() => new Promise((resolve,reject) => { finish=fail?reject:resolve; }));
    const copying=ui.button.click(); ui.osChoice.value='windows';ui.osChoice.change();finish(fail?new Error('denied'):undefined);await copying;
    assert.equal(ui.button.textContent,'Copy command');assert.equal(ui.copyStatus.textContent,'');assert.equal(ui.selected(),undefined);assert.equal(ui.timers.size,0);
  }
});


test('site build injects VERSION into selected server archive URLs and leaves no marker', () => {
  const {execFileSync}=require('node:child_process');
  execFileSync('sh',[`${__dirname}/build.sh`]);
  const script=readFileSync(`${__dirname}/dist/setup.js`,'utf8');
  const version=readFileSync(`${__dirname}/../VERSION`,'utf8').trim();
  assert.equal(script.includes('__GRASSHOPPER_RELEASE_VERSION__'),false);
  const ui=setup(async()=>{},script);
  for(const [os,target] of [['macos','darwin-arm64'],['windows','windows-amd64'],['linux','linux-amd64']]) {
    ui.osChoice.value=os;ui.osChoice.change();
    assert.equal(ui.serverArchive.href,`https://github.com/MylesMCook/grasshopper/releases/download/v${version}/grasshopper-server-${target}-${version}.zip`);
    assert.equal(ui.serverChecksums.href,`https://github.com/MylesMCook/grasshopper/releases/download/v${version}/SHA256SUMS`);
  }
});

test('public navigation and privacy are consistent and later links stay below setup', () => {
  for(const file of ['index.html','setup/index.html','view/index.html']) {
    const html=readFileSync(`${__dirname}/public/${file}`,'utf8');
    const nav=html.match(/<nav[^>]*>(.*?)<\/nav>/s)[1];
    const labels=[...nav.matchAll(/<a[^>]*>(.*?)<\/a>/g)].map(match=>match[1]);
    assert.deepEqual(labels,['Home','Set up','Memory view']);
    assert.match(html,/<footer[^>]*>.*This site never reads your memories or access token\./s);
  }
  const html=readFileSync(`${__dirname}/public/setup/index.html`,'utf8');
  assert.ok(html.indexOf('for="connector-os"')<html.indexOf('id="server-title"'));
  assert.ok(html.indexOf('back-up-and-update-the-server')>html.indexOf('<footer'));
  assert.match(html,/href="http:\/\/127\.0\.0\.1:8106\/visualizer\/"/);
  assert.match(html,/grasshopper#need-a-server/);
});


test('copy status and label clear together after two seconds, without an older timer clearing newer feedback', async () => {
  const ui=setup(async()=>{});await ui.button.click();
  const oldTimer=[...ui.timers.values()][0];
  await ui.secondButton.click();
  assert.equal(ui.button.textContent,'Copy command');
  assert.equal(ui.timers.size,1);
  oldTimer.fn();assert.equal(ui.copyStatus.textContent,'Command copied.');assert.equal(ui.secondButton.textContent,'Copied');
  [...ui.timers.values()][0].fn();assert.equal(ui.copyStatus.textContent,'');assert.equal(ui.secondButton.textContent,'Copy command');
});

test('each copy click supersedes another button pending success or failure in either completion order', async () => {
  for(const order of [[0,1],[1,0]]) for(const olderFails of [true,false]) {
    const pending=[];const ui=setup(()=>new Promise((resolve,reject)=>pending.push({resolve,reject})));
    const older=ui.button.click();const newer=ui.secondButton.click();
    for(const index of order) {
      const fails=index===0?olderFails:!olderFails;
      if(fails)pending[index].reject(new Error('denied'));else pending[index].resolve();
      await (index===0?older:newer);
    }
    assert.equal(ui.button.textContent,'Copy command');
    assert.equal(ui.selected(),olderFails?undefined:ui.commands[1].textContent);
    assert.equal(ui.secondButton.textContent,olderFails?'Copied':'Select and copy');
    assert.match(ui.copyStatus.textContent,olderFails?/^Command copied\.$/:/^Command selected\./);
    assert.equal(ui.timers.size,1);
  }
});

test('checksum download is pinned to the same build release as every selected server archive', () => {
  const ui=setup(async()=>{});
  for(const os of ['macos','windows','linux']) {
    ui.osChoice.value=os;ui.osChoice.change();
    assert.equal(ui.serverChecksums.href,'https://github.com/MylesMCook/grasshopper/releases/download/v9.8.7/SHA256SUMS');
    assert.equal(new URL(ui.serverArchive.href).pathname.split('/').slice(0,-1).join('/'),new URL(ui.serverChecksums.href).pathname.split('/').slice(0,-1).join('/'));
  }
  const html=readFileSync(`${__dirname}/public/setup/index.html`,'utf8');
  assert.match(html,/<a id="server-checksums"/);
});

test('setup commands expose a named keyboard scroll area and retain their adjacent exact-copy control', () => {
  const html = readFileSync(`${__dirname}/public/setup/index.html`, 'utf8');
  const commands = [...html.matchAll(/<div class="command">(<pre[^>]*>.*?<\/pre>)(<button[^>]*class="copy-command"[^>]*>.*?<\/button>)<\/div>/gs)];
  assert.equal(commands.length, 7);
  for(const [,pre,button] of commands) {
    assert.match(pre,/tabindex="0"/);
    assert.match(pre,/aria-label="[^"]+ command"/);
    assert.match(button,/aria-label="Copy [^"]+ command"/);
    assert.equal(pre.match(/<code[^>]*>(.*?)<\/code>/s)[1].includes('\n'),false);
  }
});

test('OS download language identifies the selected server architecture', () => {
  const ui=setup(async()=>{});
  for (const [os,label] of [['macos','Download the macOS server (Apple silicon)'],['windows','Download the Windows server (x64)'],['linux','Download the Linux server (x64)']]) {
    ui.osChoice.value=os;ui.osChoice.change();assert.equal(ui.serverArchive.textContent,label);
  }
});

test('all public pages offer a skip target and header DOM follows brand, navigation, theme', () => {
  for (const path of ['index.html','setup/index.html','view/index.html','404.html']) {
    const html=readFileSync(`${__dirname}/public/${path}`,'utf8');
    assert.match(html,/<a class="skip-link" href="#content">Skip to content<\/a>/);
    assert.match(html,/<main id="content" tabindex="-1"/);
    const header=html.match(/<header.*?<\/header>/s)[0];
    assert.ok(header.indexOf('class="brand"')<header.indexOf('<nav'));
    assert.ok(header.indexOf('<nav')<header.indexOf('id="theme-toggle"'));
  }
});


test('command overflow cue disappears at the end and for a fitting command', () => {
  let cue=false,scroll;
  const pre={scrollWidth:500,clientWidth:200,scrollLeft:0,parentElement:{toggleAttribute(name,value){ assert.equal(name,'data-overflow');cue=value; }},addEventListener(name,fn){assert.equal(name,'scroll');scroll=fn;}};
  setup(async()=>{},readFileSync(`${__dirname}/public/setup.js`,'utf8'),[pre]);
  assert.equal(cue,true);
  pre.scrollLeft=300;scroll();assert.equal(cue,false);
  pre.scrollLeft=100;scroll();assert.equal(cue,true);
  pre.scrollLeft=0;pre.scrollWidth=200;scroll();assert.equal(cue,false);
});

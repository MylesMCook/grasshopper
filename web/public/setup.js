const osChoice = document.getElementById('connector-os');
const commands = document.querySelectorAll('[data-plugin-command]');
const cursorPlugin = document.getElementById('cursor-plugin');
const archiveCommand = document.getElementById('archive-connect-command');
const serverCommand = document.getElementById('server-quickstart-command');
const serverArchive = document.getElementById('server-archive');
const copyStatus = document.getElementById('copy-status');
// The site build replaces this marker with the repository's release version.
const releaseVersion = '__GRASSHOPPER_RELEASE_VERSION__';
const copyButtons = document.querySelectorAll('.copy-command');
const copyTimers = new Map();
const copyLabels = new Map(Array.from(copyButtons, button => [button, button.textContent]));
let copyGeneration = 0;

function resetCopy(button) {
  clearTimeout(copyTimers.get(button));
  copyTimers.delete(button);
  button.textContent = copyLabels.get(button);
}

function chooseOS() {
  if (!['macos', 'windows', 'linux'].includes(osChoice.value)) return;
  copyGeneration++;
  for (const button of copyButtons) resetCopy(button);
  copyStatus.textContent = '';
  const plugin = `grasshopper-${osChoice.value}`;
  for (const command of commands) command.textContent = command.textContent.replace(/grasshopper-(macos|windows|linux)/g, plugin);
  cursorPlugin.textContent = plugin;
  const windows = osChoice.value === 'windows';
  archiveCommand.textContent = `${windows ? '.\\bin\\grasshopper.exe' : './bin/grasshopper'} connect --agents claude --url https://your-private-server`;
  serverCommand.textContent = `${windows ? '.\\bin\\grasshopper-server.exe' : './bin/grasshopper-server'} --quickstart`;
  const target = { macos: 'darwin-arm64', windows: 'windows-amd64', linux: 'linux-amd64' }[osChoice.value];
  serverArchive.href = `https://github.com/MylesMCook/grasshopper/releases/download/v${releaseVersion}/grasshopper-server-${target}-${releaseVersion}.zip`;
  serverArchive.textContent = `Download server for ${{ macos: 'macOS', windows: 'Windows', linux: 'Linux' }[osChoice.value]}`;
}
osChoice.addEventListener('change', chooseOS);
chooseOS();

for (const button of copyButtons) {
  const command = button.previousElementSibling;
  button.addEventListener('click', async () => {
    const generation = copyGeneration;
    const text = command.textContent;
    resetCopy(button);
    try {
      await navigator.clipboard.writeText(text);
      if (generation !== copyGeneration) return;
      button.textContent = 'Copied';
      copyStatus.textContent = 'Command copied.';
    } catch {
      if (generation !== copyGeneration) return;
      const selection = window.getSelection();
      const range = document.createRange();
      range.selectNodeContents(command);
      selection.removeAllRanges();
      selection.addRange(range);
      button.textContent = 'Select and copy';
      copyStatus.textContent = 'Command selected. Use your browser’s Copy command to copy it.';
    }
    copyTimers.set(button, setTimeout(() => resetCopy(button), 2000));
  });
}

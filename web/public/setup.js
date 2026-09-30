const osChoice = document.getElementById('connector-os');
const commands = document.querySelectorAll('[data-plugin-command]');
const cursorPlugin = document.getElementById('cursor-plugin');
const archiveCommand = document.getElementById('archive-connect-command');
const copyStatus = document.getElementById('copy-status');

osChoice.addEventListener('change', () => {
  if (!['macos', 'windows', 'linux'].includes(osChoice.value)) return;
  const plugin = `grasshopper-${osChoice.value}`;
  for (const command of commands) {
    command.textContent = command.textContent.replace(/grasshopper-(macos|windows|linux)/g, plugin);
  }
  cursorPlugin.textContent = plugin;
  const binary = osChoice.value === 'windows' ? '.\\bin\\grasshopper.exe' : './bin/grasshopper';
  archiveCommand.textContent = `${binary} connect --agents claude --url https://your-private-server`;
});

for (const button of document.querySelectorAll('.copy-command')) {
  const command = button.previousElementSibling;
  button.addEventListener('click', async () => {
    try {
      await navigator.clipboard.writeText(command.textContent);
      button.textContent = 'Copied';
      copyStatus.textContent = 'Command copied.';
    } catch {
      const selection = window.getSelection();
      const range = document.createRange();
      range.selectNodeContents(command);
      selection.removeAllRanges();
      selection.addRange(range);
      button.textContent = 'Select and copy';
      copyStatus.textContent = 'Command selected. Use your browser’s Copy command to copy it.';
    }
  });
}

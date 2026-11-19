// Restore pinned public model/runtime assets into an ignored, task-local folder.
import {createHash, randomUUID} from 'node:crypto';
import {createReadStream, createWriteStream} from 'node:fs';
import {appendFile, mkdir, rename, rm} from 'node:fs/promises';
import {resolve, join} from 'node:path';
import {Readable} from 'node:stream';
import {pipeline} from 'node:stream/promises';
import {spawnSync} from 'node:child_process';

const root = resolve(process.argv[2] ?? '.build/embedding');
const model = join(root, 'model');
await mkdir(model, {recursive: true});
async function digest(path) {
  const hash = createHash('sha256');
  for await (const chunk of createReadStream(path)) hash.update(chunk);
  return hash.digest('hex');
}
async function restore(url, path, expected) {
  try { if (await digest(path) === expected) return; } catch (error) {
    if (error.code !== 'ENOENT') throw error;
  }
  const temporary = `${path}.${randomUUID()}.download`;
  try {
    const response = await fetch(url, {signal: AbortSignal.timeout(180000)});
    if (!response.ok) throw new Error(`Download failed: ${response.status} ${url}`);
    await pipeline(Readable.fromWeb(response.body), createWriteStream(temporary, {flags: 'wx'}));
    if (await digest(temporary) !== expected) throw new Error(`SHA-256 mismatch: ${url}`);
    // These are public, ignored dependency assets, never application state.
    await rename(temporary, path);
  } finally { await rm(temporary, {force: true}); }
}
const source = 'https://huggingface.co/onnx-community/granite-embedding-small-english-r2-ONNX/resolve/1dc7835ba0cb9c76a3618d0bf0c427c97671b3c8';
for (const [remote, local, sha] of [
  ['onnx/model.onnx', 'model.onnx', 'cddb145cd1147ec24a3908b2ca2602b98b20a3d198365cff270b7cb26c98179e'],
  ['onnx/model.onnx_data', 'model.onnx_data', '86a3a705d4598615894d89540ea71a3d9bbdb17a315e79edcd5dfc737222834b'],
  ['tokenizer.json', 'tokenizer.json', 'feeb83348dcb033bc6b9d2e1f7906ca9eb2d122845000c9416d894d7c2927149'],
]) await restore(`${source}/${remote}`, join(model, local), sha);
// ONNX Runtime 1.30.0 comes from Grasshopper's own 2.11.3 server archive, because
// Microsoft's release downloads stopped resolving. That archive carries the upstream
// library, LICENSE and ThirdPartyNotices.txt unchanged (checked against the original
// osx-arm64 archive). Both the archive and the library inside it are pinned.
const platform = {
  'darwin-arm64': ['osx-arm64', 'darwin-arm64', 'libonnxruntime.dylib',
    '9847befefc20cdc808135ed1114f82882596fe9d14be48005e350baa4fde825a', 'bffaa6ef856dba2c26f09c7c0e7018fd1562625f3485292c7ad28b83748c6879'],
  'linux-x64': ['linux-x64', 'linux-amd64', 'libonnxruntime.so',
    '0a4b6612dea9a811d8578661d9d0316c100af573886d9ed440e30f86652d76f5', '245a6f8c38127551057a1cd1ffd59f0a186a227ade4f3492dea2494eb565542e'],
  'win32-x64': ['win-x64', 'windows-amd64', 'onnxruntime.dll',
    '8206401d04e4686b5f1675f966400e5b7986aa6c2a6a81573eac933c4fc84444', '7e39e2bdbba836d98071ef28620735ba36a47c554cf794585269aecc50fab0da'],
}[`${process.platform}-${process.arch}`];
if (!platform) throw new Error(`Unsupported native test platform: ${process.platform}-${process.arch}`);
const [target, release, library, archiveSha, librarySha] = platform;
const name = `onnxruntime-${target}-1.30.0`;
const archiveName = `grasshopper-server-${release}-2.11.3.zip`;
const archive = join(root, archiveName);
await restore(`https://github.com/MylesMCook/grasshopper/releases/download/v2.11.3/${archiveName}`, archive, archiveSha);
// Each archive entry and where the upstream release layout keeps it.
const files = [
  [`runtime/${library}`, `lib/${library}`],
  ['licenses/ONNX-Runtime-LICENSE', 'LICENSE'],
  ['licenses/ONNX-Runtime-ThirdPartyNotices.txt', 'ThirdPartyNotices.txt'],
];
const entries = files.map(([entry]) => entry);
const staging = join(root, `${name}.${randomUUID()}.extract`);
await mkdir(staging);
try {
  // Windows' own bsdtar reads zip files; Git Bash's GNU tar on PATH does not.
  const extracted = process.platform === 'win32'
    ? spawnSync(join(process.env.SystemRoot ?? 'C:\\Windows', 'System32', 'tar.exe'), ['-xf', archive, '-C', staging, ...entries], {stdio: 'inherit'})
    : spawnSync('unzip', ['-q', archive, ...entries, '-d', staging], {stdio: 'inherit'});
  if (extracted.status !== 0) throw new Error('Runtime extraction failed');
  if (await digest(join(staging, 'runtime', library)) !== librarySha) throw new Error(`SHA-256 mismatch: ${library}`);
  await rm(join(root, name), {recursive: true, force: true});
  await mkdir(join(root, name, 'lib'), {recursive: true});
  for (const [entry, local] of files) await rename(join(staging, entry), join(root, name, local));
} finally { await rm(staging, {recursive: true, force: true}); }
const runtime = join(root, name, 'lib', library);
const environment = `GRASSHOPPER_EMBED_TEST_ROOT=${model}\nGRASSHOPPER_ONNX_RUNTIME_LIBRARY=${runtime}\n`;
if (process.env.GITHUB_ENV) await appendFile(process.env.GITHUB_ENV, environment);
process.stdout.write(environment);

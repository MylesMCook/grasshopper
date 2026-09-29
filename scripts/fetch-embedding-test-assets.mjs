// Restore pinned public model/runtime assets into an ignored, task-local folder.
import {createHash} from 'node:crypto';
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
  const temporary = path + '.download';
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
const platform = {
  'darwin-arm64': ['osx-arm64', 'tgz', 'libonnxruntime.dylib', '6ebb5062a934537c352937821f9fe9718e7de1a2db1122a93dd363ffd53a7012'],
  'linux-x64': ['linux-x64', 'tgz', 'libonnxruntime.so', 'a5ed5a3cac51fbb2e90da632ae43d19212faaa20e76484e62bcb7c23ddb3b3fd'],
  'win32-x64': ['win-x64', 'zip', 'onnxruntime.dll', 'c6ba983baf5681af108599675d2a89c2d145512d02de28aed0bff177cd0ba949'],
}[`${process.platform}-${process.arch}`];
if (!platform) throw new Error(`Unsupported native test platform: ${process.platform}-${process.arch}`);
const [target, extension, library, sha] = platform;
const name = `onnxruntime-${target}-1.30.0`;
const archive = join(root, `${name}.${extension}`);
await restore(`https://github.com/microsoft/onnxruntime/releases/download/v1.30.0/${name}.${extension}`, archive, sha);
const extracted = spawnSync('tar', ['-xf', archive, '-C', root], {stdio: 'inherit'});
if (extracted.status !== 0) throw new Error('Runtime extraction failed');
const runtime = join(root, name, 'lib', library);
const environment = `GRASSHOPPER_EMBED_TEST_ROOT=${model}\nGRASSHOPPER_ONNX_RUNTIME_LIBRARY=${runtime}\n`;
if (process.env.GITHUB_ENV) await appendFile(process.env.GITHUB_ENV, environment);
process.stdout.write(environment);

// Grasshopper for Pi: shared memory across agents and machines.
//
// One file serves two installs. `pi install git:github.com/MylesMCook/grasshopper`
// loads it as a Pi package. The Grasshopper client also writes a copy into
// Pi's extensions folder on machines without the package, with the client
// configuration path filled in below. Either way it runs the self-updated
// client, which follows the server's version, so memory behavior never drifts
// from the server. Turn the client-written copy off with
// `grasshopper pi remove` instead of editing it.
import { execFile } from "node:child_process";
import { createHash } from "node:crypto";
import { chmodSync, existsSync, mkdirSync, readdirSync, readFileSync, renameSync, rmSync, writeFileSync } from "node:fs";
import { homedir } from "node:os";
import { dirname, join, resolve, sep } from "node:path";
import { inflateRawSync } from "node:zlib";

// The client replaces this line with its configuration path when it writes
// the extension. The package copy finds the configuration itself.
const writtenConfig: string = "";

const releases = "https://github.com/MylesMCook/grasshopper/releases";
const exe = process.platform === "win32" ? ".exe" : "";
const unavailable =
  "Grasshopper memory is unavailable in this session. Continue without it; `grasshopper check` diagnoses the connection. No memory write was acknowledged.";
const loaded = /^Grasshopper context loaded/m;
// Both installs can be present while the client catches up with the package;
// the first one Pi loads serves the session.
const claimed = Symbol.for("grasshopper.pi");

// configPath mirrors the client's own lookup (client.ConfigPath).
function configPath(): string {
  if (writtenConfig) return writtenConfig;
  if (process.env.GRASSHOPPER_CLIENT_CONFIG) return resolve(process.env.GRASSHOPPER_CLIENT_CONFIG);
  const home = homedir();
  const legacy =
    process.platform === "win32"
      ? join(process.env.APPDATA || join(home, "AppData", "Roaming"), "grasshopper", "client.json")
      : process.platform === "darwin"
        ? join(home, "Library", "Application Support", "grasshopper", "client.json")
        : join(process.env.XDG_CONFIG_HOME || join(home, ".config"), "grasshopper", "client.json");
  if (process.platform !== "win32") return legacy;
  const current = join(home, ".grasshopper", "client.json");
  const present = (path: string) => existsSync(path) || existsSync(path + ".pairing");
  return !present(current) && present(legacy) ? legacy : current;
}

// The self-updated client every agent on this machine runs.
function managedClient(config: string): string | undefined {
  const dir = join(dirname(config), "client");
  try {
    const version = readFileSync(join(dir, "current"), "utf8").trim();
    const program = join(dir, "versions", version, "grasshopper" + exe);
    return existsSync(program) ? program : undefined;
  } catch {
    return undefined;
  }
}

function run(program: string, args: string[], input?: string, timeout = 8000): Promise<{ ok: boolean; out: string; err: string }> {
  return new Promise((done) => {
    const child = execFile(program, args, { timeout, maxBuffer: 1 << 20, windowsHide: true }, (error, out, err) =>
      done({ ok: !error, out: String(out), err: String(err) }),
    );
    child.stdin?.end(input ?? "");
  });
}

// Startup context comes from the hook Codex uses: the shared memory policy
// plus scoped memories. Pi loads AGENTS.md files itself.
async function startupContext(program: string | undefined, config: string, cwd: string): Promise<string> {
  if (!program) return unavailable;
  const result = await run(program, ["hook", "--harness", "codex", "--config", config], JSON.stringify({ hook_event_name: "SessionStart", cwd }));
  try {
    return JSON.parse(result.out)?.hookSpecificOutput?.additionalContext || unavailable;
  } catch {
    return unavailable;
  }
}

// A machine that runs only Pi has no client yet. Download the latest client
// archive, check it against the release's SHA256SUMS, and unpack it; the
// connected client then installs the version that matches the server.
async function bootstrapClient(config: string): Promise<string> {
  const root = join(dirname(config), "pi-bootstrap");
  // An approval takes two runs of connect; reuse the client the first one got.
  const kept = (existsSync(root) ? readdirSync(root) : [])
    .filter((name) => /^\d+\.\d+\.\d+$/.test(name))
    .map((name) => join(root, name, "bin", "grasshopper" + exe))
    .find((program) => existsSync(program));
  if (kept) return kept;
  const target = { darwin: "darwin-arm64", win32: "windows-amd64", linux: "linux-amd64" }[process.platform as string];
  const arch = { darwin: "arm64", win32: "x64", linux: "x64" }[process.platform as string];
  if (!target || process.arch !== arch) throw new Error(`no Grasshopper client release for ${process.platform}-${process.arch}`);
  const fetchOk = async (url: string) => {
    const response = await fetch(url, { redirect: "follow" });
    if (!response.ok || !response.url.startsWith("https://")) throw new Error(`${url}: HTTP ${response.status}`);
    return Buffer.from(await response.arrayBuffer());
  };
  const sums = (await fetchOk(`${releases}/latest/download/SHA256SUMS`)).toString("utf8");
  const line = sums.split("\n").map((l) => l.trim().split(/\s+\*?/)).find(([, name]) => new RegExp(`^grasshopper-client-${target}-\\d+\\.\\d+\\.\\d+\\.zip$`).test(name ?? ""));
  if (!line) throw new Error("the latest release lists no client for this machine");
  const [want, archive] = line;
  const version = archive.slice(`grasshopper-client-${target}-`.length, -".zip".length);
  const zipped = await fetchOk(`${releases}/download/v${version}/${archive}`);
  if (createHash("sha256").update(zipped).digest("hex") !== want.toLowerCase()) throw new Error(`${archive} does not match SHA256SUMS`);
  const staging = join(root, `.tmp-${process.pid}`);
  rmSync(staging, { recursive: true, force: true });
  unzip(zipped, staging);
  const final = join(root, version);
  rmSync(final, { recursive: true, force: true });
  renameSync(staging, final);
  return join(final, "bin", "grasshopper" + exe);
}

// unzip extracts a client archive: stored or deflated entries, no paths
// outside dest.
function unzip(data: Buffer, dest: string): void {
  let end = data.length - 22;
  while (end >= 0 && data.readUInt32LE(end) !== 0x06054b50) end--;
  if (end < 0) throw new Error("client archive is not a zip file");
  let at = data.readUInt32LE(end + 16);
  const count = data.readUInt16LE(end + 10);
  const base = resolve(dest) + sep;
  for (let i = 0; i < count; i++) {
    if (data.readUInt32LE(at) !== 0x02014b50) throw new Error("client archive is damaged");
    const method = data.readUInt16LE(at + 10);
    const size = data.readUInt32LE(at + 20);
    const nameLength = data.readUInt16LE(at + 28);
    const skip = nameLength + data.readUInt16LE(at + 30) + data.readUInt16LE(at + 32);
    const local = data.readUInt32LE(at + 42);
    const name = data.toString("utf8", at + 46, at + 46 + nameLength);
    at += 46 + skip;
    const path = resolve(base, name);
    if (!path.startsWith(base)) throw new Error(`client archive entry escapes its folder: ${name}`);
    if (name.endsWith("/")) {
      mkdirSync(path, { recursive: true });
      continue;
    }
    const start = local + 30 + data.readUInt16LE(local + 26) + data.readUInt16LE(local + 28);
    const raw = data.subarray(start, start + size);
    const body = method === 0 ? raw : method === 8 ? inflateRawSync(raw) : undefined;
    if (!body) throw new Error(`client archive uses an unsupported method for ${name}`);
    mkdirSync(dirname(path), { recursive: true });
    writeFileSync(path, body, { mode: 0o600 });
    if (name.startsWith("bin/")) chmodSync(path, 0o700);
  }
}

function removeClientCopy(): void {
  const dir = process.env.PI_CODING_AGENT_DIR || join(homedir(), ".pi", "agent");
  const copy = join(dir, "extensions", "grasshopper.ts");
  try {
    const text = readFileSync(copy, "utf8");
    if (text.startsWith("// Written by Grasshopper") || text.startsWith("// Grasshopper for Pi")) rmSync(copy);
  } catch {
    // No copy, or one Pi's owner wrote: leave it.
  }
}

export default function (pi: any) {
  if ((globalThis as any)[claimed]) return;
  (globalThis as any)[claimed] = true;
  pi.on("session_shutdown", () => {
    delete (globalThis as any)[claimed];
  });

  // A client older than 2.14.0 still writes its own copy, which does not
  // check the claim above. The package removes that copy so Pi loads it no
  // more; if it already loaded this session, the package stands down for this
  // session instead of starting a second hook and MCP server.
  if (!writtenConfig) {
    removeClientCopy();
    if (pi.getMcpServers?.().some((server: any) => server.name === "grasshopper")) return;
  }

  const config = configPath();
  const program = managedClient(config);
  if (program) {
    pi.registerMcpServer("grasshopper", {
      command: program,
      args: ["bridge", "--config", config],
      exposure: "direct",
      description: "Shared memory across agents and machines: context, search, get, store, archive.",
    });
  }

  let memory: Promise<string> | undefined;
  let cwd = process.cwd();
  const refresh = (ctx: any) => {
    cwd = ctx.cwd ?? cwd;
    memory = startupContext(program, config, cwd);
    memory.then((text) => {
      if (!ctx.hasUI) return;
      const state = !program ? "not connected: /grasshopper connect" : loaded.test(text) ? "memory loaded" : "memory unavailable";
      ctx.ui.setStatus("grasshopper", `grasshopper: ${state}`);
    });
    return memory;
  };

  pi.on("session_start", (_event: any, ctx: any) => {
    refresh(ctx);
  });
  // Compaction drops what the model read earlier; reload memory so the
  // session picks up anything other agents saved meanwhile.
  pi.on("session_compact", (_event: any, ctx: any) => {
    refresh(ctx);
  });
  pi.on("before_agent_start", async (event: any, ctx: any) => {
    event.systemPromptOptions.sections.grasshopper_memory = await (memory ?? refresh(ctx));
  });

  pi.registerCommand("grasshopper", {
    description: "Grasshopper memory: status, refresh, connect [server link]. Also /remember <text> and /handoff",
    getArgumentCompletions: (prefix: string) =>
      ["status", "refresh", "connect"].filter((v) => v.startsWith(prefix)).map((v) => ({ value: v, label: v })),
    handler: async (args: string, ctx: any) => {
      const [verb = "status", ...rest] = args.trim().split(/\s+/).filter(Boolean);
      if (verb === "refresh") {
        const text = await refresh(ctx);
        ctx.ui.notify(loaded.test(text) ? "Grasshopper memory reloaded." : unavailable, loaded.test(text) ? "info" : "warning");
        return;
      }
      if (verb === "connect") return connect(rest[0], ctx);
      if (!program) {
        ctx.ui.notify("Grasshopper is not connected on this machine. Run /grasshopper connect <your server link>.", "warning");
        return;
      }
      const result = await run(program, ["check", "--config", config], undefined, 20000);
      ctx.ui.notify((result.out + result.err).trim() || "grasshopper check printed nothing.", result.ok ? "info" : "warning");
    },
  });

  // /remember and /handoff become ordinary prompts, so they work in every
  // mode and queue normally while the agent is busy. They are input rewrites
  // rather than commands because a command cannot start a turn in print mode.
  pi.on("input", (event: any) => {
    const match = /^\/(remember|handoff)(?:\s+([\s\S]*))?$/.exec(event.text.trim());
    if (!match) return { action: "continue" };
    const [, verb, rest = ""] = match;
    if (verb === "remember") {
      if (!rest.trim()) return { action: "continue" };
      return { action: "transform", text: `Remember this in Grasshopper (I am asking you to save it): ${rest.trim()}` };
    }
    const note = rest.trim() ? ` Note from me: ${rest.trim()}` : "";
    return { action: "transform", text: `Write the Grasshopper handoff for this work now, following the memory policy's handoff rules. Check Git state first.${note}` };
  });

  // connect runs the client's own approval flow. The owner approves the
  // device in the memory view; no token passes through Pi.
  async function connect(url: string | undefined, ctx: any) {
    let client = program;
    if (!client) {
      ctx.ui.notify("Getting the Grasshopper client...", "info");
      try {
        client = await bootstrapClient(config);
      } catch (error) {
        ctx.ui.notify(`Grasshopper client download failed: ${(error as Error).message}`, "error");
        return;
      }
    }
    const args = ["connect", "--json", "--config", config, ...(url ? ["--url", url] : [])];
    const result = await run(client, args, undefined, 60000);
    let reply: any;
    try {
      reply = JSON.parse(result.out.trim().split("\n").pop() || "");
    } catch {
      ctx.ui.notify(`Grasshopper connect failed: ${(result.err || result.out).trim()}`, "error");
      return;
    }
    if (reply.status === "missing_address") {
      ctx.ui.notify("Run /grasshopper connect <your server link>. A base, memory-view, or /mcp link works.", "warning");
      return;
    }
    if (reply.status === "approval_pending") {
      const lines = [reply.next_step, reply.approval_url && `Approve at: ${reply.approval_url}`, reply.device && `Device: ${reply.device}`, reply.code && `Code: ${reply.code}`, "Then run /grasshopper connect again."];
      ctx.ui.notify(lines.filter(Boolean).join("\n"), "info");
      return;
    }
    if (reply.status !== "connected") {
      ctx.ui.notify(reply.next_step || `Grasshopper connect: ${reply.status}`, "warning");
      return;
    }
    // Install the client that matches the server, then reload so this
    // session gets memory and the tools.
    const update = await run(client, ["update", "--config", config], undefined, 300000);
    if (!update.ok) {
      ctx.ui.notify(`Connected, but installing the matching client failed: ${(update.err || update.out).trim()}`, "error");
      return;
    }
    ctx.ui.notify("Grasshopper connected. Reloading.", "info");
    await ctx.reload();
  }
}

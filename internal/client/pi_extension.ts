// Written by Grasshopper to connect Pi to shared memory. The Grasshopper
// client rewrites this file when it updates; turn the connection off with
// `grasshopper pi remove` instead of editing it.
import { execFile } from "node:child_process";
import { existsSync, readFileSync } from "node:fs";
import { dirname, join } from "node:path";

const config: string = __GRASSHOPPER_CONFIG__;
const unavailable =
  "Grasshopper memory is unavailable in this session. Continue without it; `grasshopper check` diagnoses the connection. No memory write was acknowledged.";

// The self-updated client follows the server's version, so Pi always runs
// the client every other agent on this machine runs.
function managedClient(): string | undefined {
  const dir = join(dirname(config), "client");
  try {
    const version = readFileSync(join(dir, "current"), "utf8").trim();
    const program = join(dir, "versions", version, process.platform === "win32" ? "grasshopper.exe" : "grasshopper");
    return existsSync(program) ? program : undefined;
  } catch {
    return undefined;
  }
}

// Startup context comes from the same hook Codex uses: the shared memory
// policy plus scoped memories. Pi loads AGENTS.md files itself.
function startupContext(program: string | undefined, cwd: string): Promise<string> {
  if (!program) return Promise.resolve(unavailable);
  return new Promise((resolve) => {
    const child = execFile(
      program,
      ["hook", "--harness", "codex", "--config", config],
      { timeout: 8000, maxBuffer: 1 << 20, windowsHide: true },
      (_error, stdout) => {
        try {
          resolve(JSON.parse(stdout)?.hookSpecificOutput?.additionalContext || unavailable);
        } catch {
          resolve(unavailable);
        }
      },
    );
    child.stdin?.end(JSON.stringify({ hook_event_name: "SessionStart", cwd }));
  });
}

export default function (pi: any) {
  const program = managedClient();
  if (program) {
    pi.registerMcpServer("grasshopper", {
      command: program,
      args: ["bridge", "--config", config],
      exposure: "direct",
      description: "Shared memory across agents and machines: context, search, get, store, archive.",
    });
  }
  let memory: Promise<string> | undefined;
  pi.on("session_start", (_event: any, ctx: any) => {
    memory = startupContext(program, ctx.cwd);
  });
  pi.on("before_agent_start", async (event: any, ctx: any) => {
    memory ??= startupContext(program, ctx.cwd);
    event.systemPromptOptions.sections.grasshopper_memory = await memory;
  });
}

import fs from "node:fs";

function readStdin(): string {
  return fs.readFileSync(0, "utf8");
}

function handleClaude(): number {
  const event = JSON.parse(readStdin()) as Record<string, unknown>;
  void event;
  process.stdout.write("{}");
  return 0;
}

function handleCodex(): number {
  const payload = process.argv[3];
  if (!payload) {
    process.stderr.write("missing notify payload\n");
    return 1;
  }
  const event = JSON.parse(payload) as Record<string, unknown>;
  void event;
  return 0;
}

function main(): number {
  const hookName = process.argv[2];
  if (!hookName) {
    process.stderr.write("usage: main.ts <hook-name>\n");
    return 1;
  }
  if (hookName === "notify") {
    return handleCodex();
  }
  return handleClaude();
}

process.exit(main());

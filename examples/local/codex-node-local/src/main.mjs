import fs from "node:fs";

function readStdin() {
  return fs.readFileSync(0, "utf8");
}

function handleClaude() {
  const event = JSON.parse(readStdin());
  void event;
  process.stdout.write("{}");
  return 0;
}

function handleCodex() {
  const payload = process.argv[3];
  if (!payload) {
    process.stderr.write("missing notify payload\n");
    return 1;
  }
  const event = JSON.parse(payload);
  void event;
  return 0;
}

function main() {
  const hookName = process.argv[2];
  if (!hookName) {
    process.stderr.write("usage: main.mjs <hook-name>\n");
    return 1;
  }
  if (hookName === "notify") {
    return handleCodex();
  }
  return handleClaude();
}

process.exit(main());

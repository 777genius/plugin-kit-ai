"use strict";

const assert = require("node:assert/strict");
const crypto = require("node:crypto");
const fs = require("node:fs");
const os = require("node:os");
const path = require("node:path");
const test = require("node:test");

const script = path.resolve(__dirname, "..", "scripts", "platform-proof.js");
const { frozenReleaseAsset, lifecycleCommands, npmInvocation, parseBootstrapMode, parseLifecycle } = require(script);

test("platform proof rejects lifecycle values other than exact true or false", () => {
  for (const value of ["TRUE", "False", "1", "yes"]) {
    assert.throws(() => parseLifecycle(value), /lifecycle must be exactly true or false/);
  }
  assert.equal(parseLifecycle("true"), true);
  assert.equal(parseLifecycle("false"), false);
});

test("Windows npm invocation executes npm-cli.js without a shell", () => {
  const execPath = "C:\\Program Files\\nodejs\\node.exe";
  const tarball = "C:\\proof workspace\\universal-agent-plugins.tgz";
  const invocation = npmInvocation(["install", "--save-exact", tarball], "win32", execPath);

  assert.equal(invocation.command, execPath);
  assert.deepEqual(invocation.args, [
    "C:\\Program Files\\nodejs\\node_modules\\npm\\bin\\npm-cli.js",
    "install",
    "--save-exact",
    tarball
  ]);
  assert.equal(Object.hasOwn(invocation, "shell"), false);
});

test("platform proof lifecycle commands use the default no-confirmation flow", () => {
  const commands = lifecycleCommands("C:\\proof workspace\\synthetic plugin");

  assert.equal(commands.length, 4);
  assert.deepEqual(commands.map((command) => command[0]), ["add", "add", "update", "remove"]);
  for (const command of commands) {
    assert.equal(command.includes("--yes"), false, `unexpected --yes in ${command.join(" ")}`);
  }
});

test("platform proof accepts only explicit bootstrap evidence modes", () => {
  assert.equal(parseBootstrapMode("local_frozen_asset"), "local_frozen_asset");
  assert.equal(parseBootstrapMode("public_release_download"), "public_release_download");
  assert.throws(() => parseBootstrapMode("download"), /bootstrap mode must be exactly/);
});

test("platform proof binds the local binary to the frozen release manifest and npm pin", (t) => {
  const root = fs.mkdtempSync(path.join(os.tmpdir(), "agentplugins-platform-manifest-"));
  t.after(() => fs.rmSync(root, { recursive: true, force: true }));
  const version = "0.1.0";
  const commit = "a".repeat(40);
  const target = "linux-amd64";
  const file = `agentplugins_${version}_linux_amd64`;
  const binary = Buffer.from("exact frozen binary");
  const pinned = {
    file,
    size: binary.length,
    sha256: crypto.createHash("sha256").update(binary).digest("hex")
  };
  fs.writeFileSync(path.join(root, file), binary);
  fs.writeFileSync(path.join(root, "release-manifest.json"), JSON.stringify({
    schema_version: 2,
    tag: `agentplugins-v${version}`,
    version,
    commit,
    assets: { [target]: pinned }
  }));
  assert.equal(frozenReleaseAsset(root, commit, version, target, pinned), path.join(root, file));
  assert.throws(
    () => frozenReleaseAsset(root, "b".repeat(40), version, target, pinned),
    /manifest does not match/
  );
  assert.throws(
    () => frozenReleaseAsset(root, commit, version, target, { ...pinned, sha256: "0".repeat(64) }),
    /manifest does not match/
  );
});

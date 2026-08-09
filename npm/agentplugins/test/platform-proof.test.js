"use strict";

const assert = require("node:assert/strict");
const path = require("node:path");
const test = require("node:test");

const script = path.resolve(__dirname, "..", "scripts", "platform-proof.js");
const { lifecycleCommands, npmInvocation, parseLifecycle } = require(script);

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

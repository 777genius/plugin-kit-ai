#!/usr/bin/env node
"use strict";

const crypto = require("node:crypto");
const fs = require("node:fs");
const os = require("node:os");
const path = require("node:path");
const { spawnSync } = require("node:child_process");

function fail(message) {
  throw new Error(message);
}

function run(command, args, options) {
  const result = spawnSync(command, args, { ...options, encoding: "utf8" });
  if (result.status !== 0) {
    fail(`${command} ${args.join(" ")} failed (${result.status}):\n${result.stdout || ""}${result.stderr || ""}`);
  }
  return result.stdout;
}

function jsonCommand(command, args, options) {
  const body = run(command, args, options);
  try {
    return JSON.parse(body);
  } catch (error) {
    fail(`command did not return JSON: ${body}\n${error.message}`);
  }
}

function sha256(file) {
  return crypto.createHash("sha256").update(fs.readFileSync(file)).digest("hex");
}

function mkdir(directory) {
  fs.mkdirSync(directory, { recursive: true });
}

function main() {
  const [tarballArg, version, expectedTarget, lifecycleArg, resultArg] = process.argv.slice(2);
  if (!tarballArg || !version || !expectedTarget || !lifecycleArg || !resultArg) {
    fail("usage: platform-proof.js <npm-tarball> <version> <os-arch> <true|false> <result-json>");
  }
  const osNames = { darwin: "darwin", linux: "linux", win32: "windows" };
  const archNames = { x64: "amd64", arm64: "arm64" };
  const actualTarget = `${osNames[process.platform] || process.platform}-${archNames[process.arch] || process.arch}`;
  if (actualTarget !== expectedTarget) {
    fail(`runner architecture mismatch: expected ${expectedTarget}, got ${actualTarget}`);
  }

  const root = fs.mkdtempSync(path.join(os.tmpdir(), "agentplugins-platform-proof-"));
  const project = path.join(root, "project");
  const home = path.join(root, "home");
  const cursor = path.join(home, ".cursor");
  const state = path.join(root, "state");
  const cache = path.join(root, "binary-cache");
  const synthetic = path.join(root, "synthetic-plugin");
  const temporary = path.join(root, "tmp");
  for (const directory of [project, cursor, synthetic, temporary]) mkdir(directory);
  fs.writeFileSync(path.join(cursor, "platform-proof-marker"), "synthetic isolated client root\n");
  fs.writeFileSync(path.join(synthetic, "plugin.json"), JSON.stringify({
    $schema: "https://agent-plugins.org/schemas/1.0.0/plugin.schema.json",
    name: "platform-proof-synthetic",
    version: "1.0.0",
    description: "Synthetic package used only by isolated release CI"
  }, null, 2) + "\n");
  fs.writeFileSync(path.join(project, "package.json"), JSON.stringify({
    name: "agentplugins-platform-proof-project",
    version: "1.0.0",
    private: true
  }, null, 2) + "\n");

  const env = { ...process.env };
  Object.assign(env, {
    HOME: home,
    USERPROFILE: home,
    APPDATA: path.join(root, "appdata"),
    LOCALAPPDATA: path.join(root, "localappdata"),
    XDG_CONFIG_HOME: path.join(root, "config"),
    XDG_CACHE_HOME: path.join(root, "xdg-cache"),
    AGENTPLUGINS_HOME: state,
    AGENTPLUGINS_CACHE_DIR: cache,
    NPM_CONFIG_CACHE: path.join(root, "npm-cache"),
    NPM_CONFIG_USERCONFIG: path.join(home, ".npmrc"),
    GIT_CONFIG_GLOBAL: path.join(home, ".gitconfig"),
    GIT_CONFIG_NOSYSTEM: "1",
    GIT_TERMINAL_PROMPT: "0",
    TMPDIR: temporary,
    TEMP: temporary,
    TMP: temporary
  });
  for (const name of ["CODEX_HOME", "CLAUDE_CONFIG_DIR", "CURSOR_CONFIG_DIR", "NPM_TOKEN", "NODE_AUTH_TOKEN"]) {
    delete env[name];
  }
  fs.writeFileSync(env.NPM_CONFIG_USERCONFIG, "registry=https://registry.npmjs.org/\n");

  const npm = process.platform === "win32" ? "npm.cmd" : "npm";
  const tarball = path.resolve(tarballArg);
  run(npm, ["install", "--ignore-scripts", "--no-audit", "--no-fund", "--save-exact", tarball], {
    cwd: project,
    env,
    shell: process.platform === "win32"
  });
  const packageRoot = path.join(project, "node_modules", "universal-agent-plugins");
  const pkg = JSON.parse(fs.readFileSync(path.join(packageRoot, "package.json"), "utf8"));
  if (pkg.version !== version || pkg.scripts?.preinstall || pkg.scripts?.install || pkg.scripts?.postinstall) {
    fail("installed tarball version or no-install-script invariant is invalid");
  }
  const manifest = JSON.parse(fs.readFileSync(path.join(packageRoot, "assets.json"), "utf8"));
  const pinned = manifest.assets?.[expectedTarget];
  if (!pinned || manifest.version !== version) fail(`tarball has no exact pin for ${expectedTarget}`);
  if (fs.existsSync(cache)) fail("binary cache was not cold before the launcher ran");

  const shim = path.join(project, "node_modules", ".bin", process.platform === "win32" ? "agentplugins.cmd" : "agentplugins");
  if (!fs.existsSync(shim)) fail("npm did not create the agentplugins executable shim");
  const launcher = path.join(packageRoot, "bin", "agentplugins.js");
  const commandOptions = { cwd: project, env };
  const invoke = (args) => run(process.execPath, [launcher, ...args], commandOptions);
  const invokeJSON = (args) => jsonCommand(process.execPath, [launcher, ...args, "--format", "json"], commandOptions);

  const versionOutput = invoke(["version"]).trim();
  if (versionOutput !== `agentplugins ${version}`) fail(`unexpected version output: ${versionOutput}`);
  const binaryName = process.platform === "win32" ? "agentplugins.exe" : "agentplugins";
  const binaryPath = path.join(cache, version, expectedTarget, binaryName);
  const stat = fs.lstatSync(binaryPath);
  if (!stat.isFile() || stat.isSymbolicLink() || stat.size !== pinned.size || sha256(binaryPath) !== pinned.sha256) {
    fail("bootstrapped binary does not match the tarball's exact size and SHA-256 pin");
  }
  const firstMtime = stat.mtimeMs;
  const list = invokeJSON(["list"]);
  const doctor = invokeJSON(["doctor"]);
  if (list.schema_version !== 1 || list.command !== "list" || list.data.installations.length !== 0) {
    fail("clean list contract failed");
  }
  if (doctor.schema_version !== 1 || doctor.command !== "doctor" || doctor.data.read_only !== true) {
    fail("read-only doctor contract failed");
  }
  const dryRun = invokeJSON(["add", synthetic, "--target", "cursor", "--dry-run"]);
  if (dryRun.schema_version !== 1 || dryRun.command !== "add" || dryRun.data.dry_run !== true) {
    fail("synthetic add dry-run contract failed");
  }
  if (fs.existsSync(path.join(state, "state-v2.json"))) fail("dry-run wrote lifecycle state");
  if (fs.readdirSync(cursor).join(",") !== "platform-proof-marker") fail("dry-run changed the synthetic client root");
  if (fs.lstatSync(binaryPath).mtimeMs !== firstMtime) fail("warm launcher invocation replaced the verified cache entry");

  const lifecycle = lifecycleArg === "true";
  if (lifecycle) {
    const add = invokeJSON(["add", synthetic, "--target", "cursor", "--yes"]);
    const complete = invokeJSON(["add", synthetic, "--target", "cursor", "--yes", "--activation-complete", "--auth-complete"]);
    const update = invokeJSON(["update", "platform-proof-synthetic", "--target", "cursor", "--yes"]);
    const remove = invokeJSON(["remove", "platform-proof-synthetic", "--target", "cursor", "--yes"]);
    if (add.data.result.mutated !== true || add.data.result.activation.authentication !== "not_checked" ||
        complete.data.result.mutated !== true || complete.data.result.activation.activation_attested !== true ||
        complete.data.result.activation.authentication_attested !== true || update.data.result.no_change !== true ||
        update.data.result.mutated !== false || remove.data.result.mutated !== true) {
      fail("isolated synthetic add/update/remove lifecycle contract failed");
    }
    const after = invokeJSON(["list"]);
    if (after.data.installations.length !== 0) fail("removed synthetic package remained in the active list");
  }

  const result = {
    schema_version: 1,
    target: expectedTarget,
    runner_platform: process.platform,
    runner_arch: process.arch,
    execution: "native-runtime-e2e",
    release_version: version,
    npm_tarball: path.basename(tarball),
    proofs: {
      npm_install_ignore_scripts: true,
      launcher_cold_bootstrap: true,
      embedded_sha256_and_size: true,
      released_binary_executed: true,
      version: true,
      list: true,
      doctor_read_only: true,
      synthetic_add_dry_run: true,
      warm_cache: true,
      isolated_add_update_remove: lifecycle
    }
  };
  mkdir(path.dirname(path.resolve(resultArg)));
  fs.writeFileSync(path.resolve(resultArg), JSON.stringify(result, null, 2) + "\n");
  fs.rmSync(root, { recursive: true, force: true });
  process.stdout.write(JSON.stringify(result, null, 2) + "\n");
}

try {
  main();
} catch (error) {
  process.stderr.write(`agentplugins platform proof: ${error.message}\n`);
  process.exitCode = 1;
}

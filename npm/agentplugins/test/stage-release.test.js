"use strict";

const assert = require("node:assert/strict");
const fsp = require("node:fs/promises");
const os = require("node:os");
const path = require("node:path");
const test = require("node:test");

const { detectPlatform, expectedAssetName } = require("../lib/platform");
const { stage, validatePackageMetadata } = require("../scripts/stage-release");

test("release staging embeds every exact platform asset hash", async (t) => {
  const root = await fsp.mkdtemp(path.join(os.tmpdir(), "agentplugins-stage-"));
  t.after(() => fsp.rm(root, { recursive: true, force: true }));
  const packageRoot = path.join(root, "package");
  const assetsRoot = path.join(root, "assets");
  await fsp.mkdir(packageRoot);
  await fsp.mkdir(assetsRoot);
  await fsp.writeFile(path.join(packageRoot, "package.json"), JSON.stringify({
    name: "universal-agent-plugins",
    version: "0.0.0-development",
    bin: { agentplugins: "bin/agentplugins.js" }
  }));
  const version = "0.1.0";
  for (const [platform, arch] of [["darwin", "x64"], ["darwin", "arm64"], ["linux", "x64"], ["linux", "arm64"], ["win32", "x64"], ["win32", "arm64"]]) {
    const info = detectPlatform(platform, arch);
    await fsp.writeFile(path.join(assetsRoot, expectedAssetName(version, info)), `${info.key}\n`);
  }
  const manifest = stage(packageRoot, assetsRoot, version);
  assert.equal(manifest.version, version);
  assert.equal(manifest.npm_package, "universal-agent-plugins");
  assert.equal(Object.keys(manifest.assets).length, 6);
  for (const asset of Object.values(manifest.assets)) {
    assert.match(asset.sha256, /^[0-9a-f]{64}$/);
    assert.ok(asset.size > 0);
  }
  const pkg = JSON.parse(await fsp.readFile(path.join(packageRoot, "package.json"), "utf8"));
  assert.equal(pkg.version, version);
});

test("npm distribution name is independent from the agentplugins binary name", () => {
  for (const name of [
    "agentplugins",
    "universal-agent-plugins",
    "agentplugins-cli",
    "@ilyazelenko/agentplugins",
    "@777genius/agentplugins"
  ]) {
    assert.equal(validatePackageMetadata({
      name,
      bin: { agentplugins: "bin/agentplugins.js" }
    }), name);
  }
});

test("release staging rejects unsafe package metadata", () => {
  for (const name of ["AgentPlugins", " agentplugins ", 123, null]) {
    assert.throws(
      () => validatePackageMetadata({ name, bin: { agentplugins: "bin/agentplugins.js" } }),
      /package name is invalid/
    );
  }
  assert.throws(
    () => validatePackageMetadata({ name: "agentplugins-cli", bin: { other: "bin/agentplugins.js" } }),
    /must expose the agentplugins binary/
  );
});

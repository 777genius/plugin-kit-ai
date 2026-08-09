"use strict";

const assert = require("node:assert/strict");
const crypto = require("node:crypto");
const fs = require("node:fs");
const fsp = require("node:fs/promises");
const os = require("node:os");
const path = require("node:path");
const test = require("node:test");

const { detectPlatform, expectedAssetName } = require("../lib/platform");
const { prepareRelease, verifyRelease } = require("../scripts/release-assets");
const { stage, validatePackageMetadata } = require("../scripts/stage-release");

const COMMIT = "a".repeat(40);

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
  prepareRelease(assetsRoot, `agentplugins-v${version}`, COMMIT);
  const manifest = stage(packageRoot, assetsRoot, version, COMMIT);
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

test("release verification fails closed on checksum, size, and manifest mismatch", async (t) => {
  const root = await fsp.mkdtemp(path.join(os.tmpdir(), "agentplugins-release-assets-"));
  t.after(() => fsp.rm(root, { recursive: true, force: true }));
  const version = "0.1.4";
  for (const [platform, arch] of [["darwin", "x64"], ["darwin", "arm64"], ["linux", "x64"], ["linux", "arm64"], ["win32", "x64"], ["win32", "arm64"]]) {
    const info = detectPlatform(platform, arch);
    await fsp.writeFile(path.join(root, expectedAssetName(version, info)), `${info.key}\n`);
  }
  prepareRelease(root, `agentplugins-v${version}`, COMMIT);
  assert.equal(verifyRelease(root, `agentplugins-v${version}`, COMMIT).gate_eligible, true);

  await fsp.writeFile(path.join(root, "ambiguous-extra-asset"), "unexpected");
  assert.throws(() => verifyRelease(root, `agentplugins-v${version}`, COMMIT), /must contain exactly/);
  await fsp.rm(path.join(root, "ambiguous-extra-asset"));

  const manifestPath = path.join(root, "release-manifest.json");
  const manifest = JSON.parse(await fsp.readFile(manifestPath, "utf8"));
  manifest.assets["linux-amd64"].size += 1;
  await fsp.writeFile(manifestPath, JSON.stringify(manifest));
  const checksumsPath = path.join(root, "checksums.txt");
  const manifestDigest = crypto.createHash("sha256").update(await fsp.readFile(manifestPath)).digest("hex");
  const checksums = (await fsp.readFile(checksumsPath, "utf8")).replace(
    /^[0-9a-f]{64}  release-manifest\.json$/m,
    `${manifestDigest}  release-manifest.json`
  );
  await fsp.writeFile(checksumsPath, checksums);
  assert.throws(() => verifyRelease(root, `agentplugins-v${version}`, COMMIT), /asset metadata mismatch/);

  prepareRelease(root, `agentplugins-v${version}`, COMMIT);
  await fsp.appendFile(path.join(root, `agentplugins_${version}_linux_amd64`), "changed");
  assert.throws(() => verifyRelease(root, `agentplugins-v${version}`, COMMIT), /checksum mismatch/);

  prepareRelease(root, `agentplugins-v${version}`, COMMIT);
  const wrongIdentity = JSON.parse(await fsp.readFile(manifestPath, "utf8"));
  wrongIdentity.commit = "b".repeat(40);
  await fsp.writeFile(manifestPath, JSON.stringify(wrongIdentity));
  assert.throws(() => verifyRelease(root, `agentplugins-v${version}`, COMMIT), /identity does not match/);
});

test("legacy manifests are audit-only and never gate eligible", async (t) => {
  const root = await fsp.mkdtemp(path.join(os.tmpdir(), "agentplugins-legacy-assets-"));
  t.after(() => fsp.rm(root, { recursive: true, force: true }));
  const version = "0.1.4";
  for (const [platform, arch] of [["darwin", "x64"], ["darwin", "arm64"], ["linux", "x64"], ["linux", "arm64"], ["win32", "x64"], ["win32", "arm64"]]) {
    const info = detectPlatform(platform, arch);
    await fsp.writeFile(path.join(root, expectedAssetName(version, info)), `${info.key}\n`);
  }
  const { assets } = prepareRelease(root, `agentplugins-v${version}`, COMMIT);
  await fsp.writeFile(path.join(root, "release-manifest.json"), JSON.stringify({
    schema_version: 1,
    tag: `agentplugins-v${version}`,
    commit: COMMIT
  }) + "\n");
  const names = [...Object.values(assets).map((asset) => asset.file), "release-manifest.json"];
  const checksums = names.map((name) => {
    const body = fs.readFileSync(path.join(root, name));
    return `${crypto.createHash("sha256").update(body).digest("hex")}  ${name}`;
  }).join("\n") + "\n";
  await fsp.writeFile(path.join(root, "checksums.txt"), checksums);
  assert.throws(() => verifyRelease(root, `agentplugins-v${version}`, COMMIT), /schema or version/);
  const audited = verifyRelease(root, `agentplugins-v${version}`, COMMIT, { allowLegacyManifest: true });
  assert.equal(audited.manifest_schema, 1);
  assert.equal(audited.gate_eligible, false);
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

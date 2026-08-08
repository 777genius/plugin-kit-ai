#!/usr/bin/env node
"use strict";

const crypto = require("node:crypto");
const fs = require("node:fs");
const path = require("node:path");

const { detectPlatform, expectedAssetName } = require("../lib/platform");

const VERSION = /^(0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*)(?:-[0-9A-Za-z.-]+)?(?:\+[0-9A-Za-z.-]+)?$/;
const PLATFORMS = [
  ["darwin", "x64"],
  ["darwin", "arm64"],
  ["linux", "x64"],
  ["linux", "arm64"],
  ["win32", "x64"],
  ["win32", "arm64"]
];

function sha256(file) {
  return crypto.createHash("sha256").update(fs.readFileSync(file)).digest("hex");
}

function stage(packageRoot, assetRoot, version) {
  if (!VERSION.test(version)) {
    throw new Error(`invalid release version: ${version}`);
  }
  const pkgPath = path.join(packageRoot, "package.json");
  const pkg = JSON.parse(fs.readFileSync(pkgPath, "utf8"));
  if (pkg.name !== "agentplugins") {
    throw new Error("staged npm package name must be agentplugins");
  }
  pkg.version = version;
  fs.writeFileSync(pkgPath, JSON.stringify(pkg, null, 2) + "\n");
  const assets = {};
  for (const [platform, arch] of PLATFORMS) {
    const info = detectPlatform(platform, arch);
    const file = expectedAssetName(version, info);
    const filePath = path.join(assetRoot, file);
    const stat = fs.lstatSync(filePath);
    if (!stat.isFile() || stat.isSymbolicLink() || stat.size <= 0) {
      throw new Error(`release asset is not a regular non-empty file: ${file}`);
    }
    assets[info.key] = { file, sha256: sha256(filePath), size: stat.size };
  }
  const manifest = {
    schema_version: 1,
    version,
    repository: "777genius/plugin-kit-ai",
    tag: `agentplugins-v${version}`,
    assets
  };
  fs.writeFileSync(path.join(packageRoot, "assets.json"), JSON.stringify(manifest, null, 2) + "\n");
  return manifest;
}

function main() {
  const [packageRoot, assetRoot, version] = process.argv.slice(2);
  if (!packageRoot || !assetRoot || !version) {
    throw new Error("usage: stage-release.js <package-root> <asset-root> <version>");
  }
  const manifest = stage(path.resolve(packageRoot), path.resolve(assetRoot), version);
  process.stdout.write(`Staged agentplugins ${manifest.version} with ${Object.keys(manifest.assets).length} pinned assets\n`);
}

if (require.main === module) {
  try {
    main();
  } catch (error) {
    process.stderr.write(`stage agentplugins npm release: ${error.message}\n`);
    process.exitCode = 1;
  }
}

module.exports = { stage };

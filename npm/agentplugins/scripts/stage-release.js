#!/usr/bin/env node
"use strict";

const crypto = require("node:crypto");
const fs = require("node:fs");
const path = require("node:path");

const { detectPlatform, expectedAssetName } = require("../lib/platform");
const { verifyRelease } = require("./release-assets");

const VERSION = /^(0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*)(?:-[0-9A-Za-z.-]+)?(?:\+[0-9A-Za-z.-]+)?$/;
const NPM_PACKAGE_NAME = /^(?:@[a-z0-9][a-z0-9._-]*\/)?[a-z0-9][a-z0-9._-]*$/;
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

function validatePackageMetadata(pkg) {
  if (!pkg || typeof pkg !== "object" || typeof pkg.name !== "string" ||
      !NPM_PACKAGE_NAME.test(pkg.name) || pkg.name.length > 214) {
    throw new Error("staged npm package name is invalid");
  }
  const packageName = pkg.name;
  if (!pkg.bin || pkg.bin.agentplugins !== "bin/agentplugins.js") {
    throw new Error("staged npm package must expose the agentplugins binary");
  }
  return packageName;
}

function stage(packageRoot, assetRoot, version, commit, options = {}) {
  if (!VERSION.test(version)) {
    throw new Error(`invalid release version: ${version}`);
  }
  verifyRelease(assetRoot, `agentplugins-v${version}`, commit, options);
  const pkgPath = path.join(packageRoot, "package.json");
  const pkg = JSON.parse(fs.readFileSync(pkgPath, "utf8"));
  const packageName = validatePackageMetadata(pkg);
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
    npm_package: packageName,
    repository: "777genius/plugin-kit-ai",
    tag: `agentplugins-v${version}`,
    assets
  };
  fs.writeFileSync(path.join(packageRoot, "assets.json"), JSON.stringify(manifest, null, 2) + "\n");
  return manifest;
}

function main() {
  const [packageRoot, assetRoot, version, commit, policy] = process.argv.slice(2);
  if (!packageRoot || !assetRoot || !version || !commit) {
    throw new Error("usage: stage-release.js <package-root> <asset-root> <version> <commit> [allow-legacy-v1]");
  }
  const manifest = stage(path.resolve(packageRoot), path.resolve(assetRoot), version, commit, {
    allowLegacyManifest: policy === "allow-legacy-v1"
  });
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

module.exports = { stage, validatePackageMetadata };

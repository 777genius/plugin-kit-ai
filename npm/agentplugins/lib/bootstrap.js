"use strict";

const crypto = require("node:crypto");
const fs = require("node:fs");
const fsp = require("node:fs/promises");
const https = require("node:https");
const os = require("node:os");
const path = require("node:path");

const { cacheRoot, detectPlatform, expectedAssetName } = require("./platform");

const VERSION = /^(0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*)(?:-[0-9A-Za-z.-]+)?(?:\+[0-9A-Za-z.-]+)?$/;
const DIGEST = /^[0-9a-f]{64}$/;
const MAX_REDIRECTS = 5;
const DOWNLOAD_TIMEOUT_MS = 30_000;
const LOCK_TIMEOUT_MS = 30_000;

function loadRelease(packageRoot, platformInfo) {
  const pkg = JSON.parse(fs.readFileSync(path.join(packageRoot, "package.json"), "utf8"));
  const manifest = JSON.parse(fs.readFileSync(path.join(packageRoot, "assets.json"), "utf8"));
  const version = String(pkg.version || "").trim();
  if (!VERSION.test(version) || version === "0.0.0-development") {
    throw new Error("this development npm package has no released binary; use an exact published version");
  }
  if (manifest.schema_version !== 1 || manifest.version !== version) {
    throw new Error("npm version and embedded binary manifest do not match");
  }
  if (!/^[A-Za-z0-9_.-]+\/[A-Za-z0-9_.-]+$/.test(String(manifest.repository || ""))) {
    throw new Error("embedded binary repository is invalid");
  }
  if (manifest.tag !== `agentplugins-v${version}`) {
    throw new Error("embedded release tag does not match npm version");
  }
  const asset = manifest.assets && manifest.assets[platformInfo.key];
  if (!asset || asset.file !== expectedAssetName(version, platformInfo) || !DIGEST.test(String(asset.sha256 || "")) || !Number.isSafeInteger(asset.size) || asset.size <= 0) {
    throw new Error(`npm package does not contain a valid binary pin for ${platformInfo.key}`);
  }
  if (path.basename(asset.file) !== asset.file) {
    throw new Error("embedded binary asset name is unsafe");
  }
  return { asset, manifest, version };
}

async function sha256File(file) {
  const hash = crypto.createHash("sha256");
  const stream = fs.createReadStream(file);
  for await (const chunk of stream) {
    hash.update(chunk);
  }
  return hash.digest("hex");
}

async function validCachedBinary(file, expectedHash) {
  try {
    const stat = await fsp.lstat(file);
    if (!stat.isFile() || stat.isSymbolicLink()) {
      return false;
    }
    return (await sha256File(file)) === expectedHash;
  } catch (error) {
    if (error && error.code === "ENOENT") {
      return false;
    }
    throw error;
  }
}

function validateDownloadURL(value) {
  const parsed = new URL(value);
  if (parsed.protocol !== "https:" || parsed.port) {
    throw new Error("binary download and every redirect must use an approved GitHub HTTPS host");
  }
  if (parsed.username || parsed.password) {
    throw new Error("binary download URL cannot contain credentials");
  }
  const pathAndQuery = `${parsed.pathname}${parsed.search}`;
  switch (parsed.hostname) {
    case "github.com":
      return { hostname: "github.com", path: pathAndQuery, url: parsed };
    case "release-assets.githubusercontent.com":
      return { hostname: "release-assets.githubusercontent.com", path: pathAndQuery, url: parsed };
    default:
      throw new Error("binary download and every redirect must use an approved GitHub HTTPS host");
  }
}

function requestApprovedTarget(target, requestOptions) {
  const options = {
    ...requestOptions,
    method: "GET",
    path: target.path,
    port: 443,
    protocol: "https:"
  };
  switch (target.hostname) {
    case "github.com":
      return https.get({ ...options, hostname: "github.com" });
    case "release-assets.githubusercontent.com":
      return https.get({ ...options, hostname: "release-assets.githubusercontent.com" });
    default:
      throw new Error("binary download host was not validated");
  }
}

async function downloadFile(value, destination, expected, options = {}, redirects = MAX_REDIRECTS) {
  const target = validateDownloadURL(value);
  await new Promise((resolve, reject) => {
    const requestOptions = {
      headers: {
        Accept: "application/octet-stream",
        "User-Agent": "agentplugins-npm-bootstrap"
      }
    };
    const request = typeof options.request === "function"
      ? options.request(target.url, requestOptions)
      : requestApprovedTarget(target, requestOptions);
    request.setTimeout(DOWNLOAD_TIMEOUT_MS, () => request.destroy(new Error("binary download timed out")));
    request.once("error", reject);
    request.once("response", (response) => {
      if ([301, 302, 303, 307, 308].includes(response.statusCode) && response.headers.location) {
        response.resume();
        if (redirects <= 0) {
          reject(new Error("too many binary download redirects"));
          return;
        }
        const next = new URL(response.headers.location, target.url).toString();
        downloadFile(next, destination, expected, options, redirects - 1).then(resolve, reject);
        return;
      }
      if (response.statusCode < 200 || response.statusCode > 299) {
        response.resume();
        reject(new Error(`binary download failed with HTTP ${response.statusCode}`));
        return;
      }
      const declaredLength = Number(response.headers["content-length"] || 0);
      if (declaredLength && declaredLength !== expected.size) {
        response.resume();
        reject(new Error("binary download size does not match embedded metadata"));
        return;
      }
      const output = fs.createWriteStream(destination, { flags: "wx", mode: 0o600 });
      const hash = crypto.createHash("sha256");
      let size = 0;
      let settled = false;
      let verified = false;
      let pendingError;
      const fail = (error) => {
        if (settled || pendingError) return;
        pendingError = error;
        response.destroy();
        output.destroy();
      };
      output.once("error", fail);
      response.once("error", fail);
      response.on("data", (chunk) => {
        size += chunk.length;
        if (size > expected.size) {
          fail(new Error("binary download exceeded embedded size"));
          return;
        }
        hash.update(chunk);
      });
      response.pipe(output);
      output.once("finish", () => {
        if (settled || pendingError) return;
        if (size !== expected.size || hash.digest("hex") !== expected.sha256) {
          fail(new Error("binary download failed embedded SHA-256 verification"));
          return;
        }
        verified = true;
      });
      output.once("close", () => {
        if (settled) return;
        settled = true;
        if (pendingError) {
          reject(pendingError);
          return;
        }
        if (!verified) {
          reject(new Error("binary download closed before verification completed"));
          return;
        }
        resolve();
      });
    });
  });
}

function delay(milliseconds) {
  return new Promise((resolve) => setTimeout(resolve, milliseconds));
}

async function acquireLock(target, options = {}) {
  const lockRoot = path.join(os.tmpdir(), "agentplugins-npm-locks");
  await fsp.mkdir(lockRoot, { recursive: true, mode: 0o700 });
  const name = crypto.createHash("sha256").update(target).digest("hex") + ".lock";
  const lockPath = path.join(lockRoot, name);
  const started = Date.now();
  const timeoutMs = options.timeoutMs ?? LOCK_TIMEOUT_MS;
  const pollMs = options.pollMs ?? 50;
  let released = false;
  while (true) {
    try {
      const handle = await fsp.open(lockPath, "wx", 0o600);
      try {
        await handle.writeFile(JSON.stringify({
          pid: process.pid,
          nonce: crypto.randomBytes(16).toString("hex")
        }) + "\n");
      } catch (error) {
        await handle.close().catch(() => {});
        await fsp.rm(lockPath, { force: true }).catch(() => {});
        throw error;
      }
      return async () => {
        if (released) return;
        released = true;
        let closeError;
        try {
          await handle.close();
        } catch (error) {
          closeError = error;
        }
        await fsp.rm(lockPath, { force: true });
        if (closeError) throw closeError;
      };
    } catch (error) {
      if (!error || error.code !== "EEXIST") throw error;
      if (Date.now() - started > timeoutMs) {
        throw new Error(`timed out waiting for the agentplugins binary cache lock at ${lockPath}; remove it only after confirming no agentplugins or npm process is running`);
      }
      await delay(pollMs);
    }
  }
}

async function installVerifiedBinary(downloaded, binaryPath, release, platformInfo) {
  const releaseLock = await acquireLock(binaryPath);
  try {
    if (await validCachedBinary(binaryPath, release.asset.sha256)) {
      return binaryPath;
    }
    try {
      const existing = await fsp.lstat(binaryPath);
      if (!existing.isFile() || existing.isSymbolicLink()) {
        throw new Error("binary cache target exists but is not a regular file; refusing to move or replace it");
      }
    } catch (error) {
      if (!error || error.code !== "ENOENT") throw error;
    }
    const parent = path.dirname(binaryPath);
    await fsp.mkdir(parent, { recursive: true, mode: 0o700 });
    const staging = path.join(parent, `.agentplugins-staging-${process.pid}-${crypto.randomBytes(6).toString("hex")}`);
    const quarantine = path.join(parent, `.agentplugins-replaced-${process.pid}-${crypto.randomBytes(6).toString("hex")}`);
    await fsp.copyFile(downloaded, staging, fs.constants.COPYFILE_EXCL);
    if (platformInfo.osName !== "windows") {
      await fsp.chmod(staging, 0o755);
    }
    if (!(await validCachedBinary(staging, release.asset.sha256))) {
      await fsp.rm(staging, { force: true });
      throw new Error("staged binary failed repeated SHA-256 verification");
    }
    let replaced = false;
    try {
      await fsp.rename(binaryPath, quarantine);
      replaced = true;
    } catch (error) {
      if (!error || error.code !== "ENOENT") {
        await fsp.rm(staging, { force: true });
        throw error;
      }
    }
    try {
      await fsp.rename(staging, binaryPath);
    } catch (error) {
      if (replaced) {
        await fsp.rename(quarantine, binaryPath).catch(() => {});
      }
      throw error;
    }
    await fsp.rm(quarantine, { force: true });
    if (!(await validCachedBinary(binaryPath, release.asset.sha256))) {
      throw new Error("committed binary failed repeated SHA-256 verification");
    }
    return binaryPath;
  } finally {
    await releaseLock();
  }
}

async function ensureBinary(options = {}) {
  const packageRoot = options.packageRoot || path.resolve(__dirname, "..");
  const platformInfo = detectPlatform(options.platform, options.arch);
  const release = loadRelease(packageRoot, platformInfo);
  const root = options.cacheRoot || cacheRoot(options.environment, options.platform);
  const binaryPath = path.join(root, release.version, platformInfo.key, platformInfo.binaryName);
  if (await validCachedBinary(binaryPath, release.asset.sha256)) {
    return { binaryPath, version: release.version, cacheHit: true };
  }
  const releaseBase = `https://github.com/${release.manifest.repository}/releases/download/${release.manifest.tag}`;
  const url = `${String(releaseBase).replace(/\/$/, "")}/${encodeURIComponent(release.asset.file)}`;
  const temporary = path.join(os.tmpdir(), `agentplugins-download-${process.pid}-${crypto.randomBytes(8).toString("hex")}`);
  try {
    await downloadFile(url, temporary, release.asset, options);
    await installVerifiedBinary(temporary, binaryPath, release, platformInfo);
    return { binaryPath, version: release.version, cacheHit: false };
  } catch (error) {
    const cold = !(await validCachedBinary(binaryPath, release.asset.sha256));
    const suffix = cold ? " No client or plugin files were changed." : "";
    throw new Error(`unable to obtain agentplugins ${release.version}: ${error.message}.${suffix}`);
  } finally {
    await fsp.rm(temporary, { force: true });
  }
}

function formatBootstrapError(error, version = "this exact version") {
  return [
    `agentplugins npm launcher: ${error.message}`,
    `The launcher never falls back to latest. Check network access or install ${version} from the matching GitHub release.`
  ].join(os.EOL);
}

module.exports = {
  acquireLock,
  downloadFile,
  ensureBinary,
  formatBootstrapError,
  loadRelease,
  sha256File,
  validCachedBinary
};

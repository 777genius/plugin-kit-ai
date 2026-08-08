"use strict";

const assert = require("node:assert/strict");
const crypto = require("node:crypto");
const fs = require("node:fs");
const fsp = require("node:fs/promises");
const http = require("node:http");
const os = require("node:os");
const path = require("node:path");
const test = require("node:test");

const { acquireLock, downloadFile, ensureBinary, loadRelease } = require("../lib/bootstrap");
const { cacheRoot, detectPlatform, expectedAssetName } = require("../lib/platform");

const VERSION = "0.1.0";
const BINARY = Buffer.from("#!/bin/sh\necho isolated-agentplugins-test\n");

async function fixturePackage(t, binary = BINARY) {
  const root = await fsp.mkdtemp(path.join(os.tmpdir(), "agentplugins-npm-package-"));
  t.after(() => fsp.rm(root, { recursive: true, force: true }));
  const platformInfo = detectPlatform("linux", "x64");
  const file = expectedAssetName(VERSION, platformInfo);
  await fsp.writeFile(path.join(root, "package.json"), JSON.stringify({ name: "universal-agent-plugins", version: VERSION }));
  await fsp.writeFile(path.join(root, "assets.json"), JSON.stringify({
    schema_version: 1,
    version: VERSION,
    npm_package: "universal-agent-plugins",
    repository: "777genius/plugin-kit-ai",
    tag: `agentplugins-v${VERSION}`,
    assets: {
      [platformInfo.key]: {
        file,
        sha256: crypto.createHash("sha256").update(binary).digest("hex"),
        size: binary.length
      }
    }
  }));
  return { binary, file, root };
}

async function listen(t, handler) {
  const server = http.createServer(handler);
  await new Promise((resolve) => server.listen(0, "127.0.0.1", resolve));
  t.after(() => new Promise((resolve) => server.close(resolve)));
  const address = server.address();
  return { server, url: `http://127.0.0.1:${address.port}` };
}

function requestThrough(endpoint) {
  return (_target, options) => http.get(endpoint, options);
}

test("cold, warm, corrupted, and concurrent cache paths stay verified", async (t) => {
  const fixture = await fixturePackage(t);
  let requests = 0;
  const endpoint = await listen(t, (request, response) => {
    requests += 1;
    response.writeHead(200, { "content-length": fixture.binary.length });
    response.end(fixture.binary);
  });
  const cache = await fsp.mkdtemp(path.join(os.tmpdir(), "agentplugins-npm-cache-parent-"));
  await fsp.rm(cache, { recursive: true, force: true });
  t.after(() => fsp.rm(cache, { recursive: true, force: true }));
  const options = {
    packageRoot: fixture.root,
    cacheRoot: cache,
    request: requestThrough(endpoint.url),
    platform: "linux",
    arch: "x64"
  };
  const [first, concurrent] = await Promise.all([ensureBinary(options), ensureBinary(options)]);
  assert.equal(await fsp.readFile(first.binaryPath, "utf8"), fixture.binary.toString());
  assert.equal(first.binaryPath, concurrent.binaryPath);
  const beforeWarm = requests;
  const warm = await ensureBinary(options);
  assert.equal(warm.cacheHit, true);
  assert.equal(requests, beforeWarm);
  await fsp.writeFile(warm.binaryPath, "corrupted");
  const repaired = await ensureBinary(options);
  assert.equal(await fsp.readFile(repaired.binaryPath, "utf8"), fixture.binary.toString());
  assert.ok(requests > beforeWarm);
});

test("offline cold cache fails without creating the cache", async (t) => {
  const fixture = await fixturePackage(t);
  const endpoint = await listen(t, (_request, response) => response.end(fixture.binary));
  const unavailable = endpoint.url;
  await new Promise((resolve) => endpoint.server.close(resolve));
  const cache = path.join(os.tmpdir(), `agentplugins-offline-${crypto.randomBytes(8).toString("hex")}`);
  await assert.rejects(
    ensureBinary({
      packageRoot: fixture.root,
      cacheRoot: cache,
      request: requestThrough(unavailable),
      platform: "linux",
      arch: "x64"
    }),
    /No client or plugin files were changed/
  );
  assert.equal(fs.existsSync(cache), false);
});

test("redirected public download never inherits GITHUB_TOKEN", async (t) => {
  const fixture = await fixturePackage(t);
  let authorization;
  const target = await listen(t, (request, response) => {
    authorization = request.headers.authorization;
    response.writeHead(200, { "content-length": fixture.binary.length });
    response.end(fixture.binary);
  });
  const redirect = await listen(t, (_request, response) => {
    response.writeHead(302, { location: `https://release-assets.githubusercontent.com/download/${fixture.file}` });
    response.end();
  });
  const previous = process.env.GITHUB_TOKEN;
  process.env.GITHUB_TOKEN = "must-not-leak";
  t.after(() => {
    if (previous === undefined) delete process.env.GITHUB_TOKEN;
    else process.env.GITHUB_TOKEN = previous;
  });
  const cache = path.join(os.tmpdir(), `agentplugins-redirect-${crypto.randomBytes(8).toString("hex")}`);
  t.after(() => fsp.rm(cache, { recursive: true, force: true }));
  await ensureBinary({
    packageRoot: fixture.root,
    cacheRoot: cache,
    request: (url, options) => http.get(url.hostname === "github.com" ? redirect.url : target.url, options),
    platform: "linux",
    arch: "x64"
  });
  assert.equal(authorization, undefined);
});

test("release metadata, platform names, and cache roots are exact", async (t) => {
  const fixture = await fixturePackage(t);
  const platformInfo = detectPlatform("linux", "x64");
  const release = loadRelease(fixture.root, platformInfo);
  assert.equal(release.version, VERSION);
  assert.equal(release.asset.file, `agentplugins_${VERSION}_linux_amd64`);
  assert.equal(detectPlatform("win32", "arm64").binaryName, "agentplugins.exe");
  assert.equal(cacheRoot({ XDG_CACHE_HOME: "/tmp/xdg" }, "linux", "/home/test"), "/tmp/xdg/agentplugins");
  assert.equal(cacheRoot({ LOCALAPPDATA: "C:\\cache" }, "win32", "C:\\home"), path.join("C:\\cache", "agentplugins", "Cache"));
});

test("package has no install scripts", async () => {
  const packageRoot = path.resolve(__dirname, "..");
  const pkg = JSON.parse(await fsp.readFile(path.join(packageRoot, "package.json"), "utf8"));
  assert.equal(pkg.engines.node, ">=22");
  assert.deepEqual(pkg.scripts, { test: "node --test" });
});

test("development metadata cannot download a release binary", async (t) => {
  const packageRoot = await fsp.mkdtemp(path.join(os.tmpdir(), "agentplugins-development-package-"));
  t.after(() => fsp.rm(packageRoot, { recursive: true, force: true }));
  await fsp.writeFile(path.join(packageRoot, "package.json"), JSON.stringify({
    name: "universal-agent-plugins",
    version: "0.0.0-development"
  }));
  await fsp.writeFile(path.join(packageRoot, "assets.json"), "{}\n");
  assert.throws(
    () => loadRelease(packageRoot, detectPlatform("linux", "x64")),
    /development npm package/
  );
});

test("embedded release manifest is pinned to the npm distribution name", async (t) => {
  const fixture = await fixturePackage(t);
  const pkgPath = path.join(fixture.root, "package.json");
  await fsp.writeFile(pkgPath, JSON.stringify({ name: "agentplugins-cli", version: VERSION }));
  assert.throws(
    () => loadRelease(fixture.root, detectPlatform("linux", "x64")),
    /npm package name and embedded binary manifest do not match/
  );
});

test("an old mtime never lets a contender steal a live cache lock", async () => {
  const target = path.join(os.tmpdir(), `agentplugins-live-lock-${crypto.randomBytes(8).toString("hex")}`);
  const release = await acquireLock(target);
  const lockName = crypto.createHash("sha256").update(target).digest("hex") + ".lock";
  const lockPath = path.join(os.tmpdir(), "agentplugins-npm-locks", lockName);
  await fsp.utimes(lockPath, new Date(0), new Date(0));
  let firstReleased = false;
  const contender = acquireLock(target).then((unlock) => {
    assert.equal(firstReleased, true, "contender stole a lock still held by a live process");
    return unlock;
  });
  await new Promise((resolve) => setTimeout(resolve, 100));
  firstReleased = true;
  await release();
  const releaseContender = await contender;
  await releaseContender();
});

test("a stale-looking lock is never auto-removed or stolen", async (t) => {
  const target = path.join(os.tmpdir(), `agentplugins-stale-lock-${crypto.randomBytes(8).toString("hex")}`);
  const lockName = crypto.createHash("sha256").update(target).digest("hex") + ".lock";
  const lockPath = path.join(os.tmpdir(), "agentplugins-npm-locks", lockName);
  await fsp.mkdir(path.dirname(lockPath), { recursive: true });
  const body = JSON.stringify({ pid: 999999, nonce: "0".repeat(32) }) + "\n";
  await fsp.writeFile(lockPath, body, { flag: "wx", mode: 0o600 });
  t.after(() => fsp.rm(lockPath, { force: true }));
  await assert.rejects(acquireLock(target, { timeoutMs: 30, pollMs: 5 }), /remove it only after confirming/);
  assert.equal(await fsp.readFile(lockPath, "utf8"), body);
});

test("download verification closes the destination before rejecting", async (t) => {
  const endpoint = await listen(t, (_request, response) => {
    response.writeHead(200, { "content-length": BINARY.length });
    response.end(BINARY);
  });
  const destination = path.join(os.tmpdir(), `agentplugins-bad-download-${crypto.randomBytes(8).toString("hex")}`);
  t.after(() => fsp.rm(destination, { force: true }));
  await assert.rejects(
    downloadFile("https://github.com/777genius/plugin-kit-ai/releases/download/test/binary", destination, {
      size: BINARY.length,
      sha256: "0".repeat(64)
    }, { request: requestThrough(endpoint.url) }),
    /SHA-256 verification/
  );
  await fsp.rm(destination);
  assert.equal(fs.existsSync(destination), false);
});

test("binary downloads reject non-GitHub hosts and custom ports before requesting", async () => {
  const destination = path.join(os.tmpdir(), `agentplugins-rejected-download-${crypto.randomBytes(8).toString("hex")}`);
  const expected = { size: BINARY.length, sha256: crypto.createHash("sha256").update(BINARY).digest("hex") };
  let requested = false;
  const options = {
    request: () => {
      requested = true;
      throw new Error("request must not run");
    }
  };
  await assert.rejects(
    downloadFile("https://example.com/agentplugins", destination, expected, options),
    /approved GitHub HTTPS host/
  );
  await assert.rejects(
    downloadFile("https://github.com:444/agentplugins", destination, expected, options),
    /approved GitHub HTTPS host/
  );
  assert.equal(requested, false);
  assert.equal(fs.existsSync(destination), false);
});

test("a redirect to an unapproved host is rejected before a second request", async (t) => {
  const endpoint = await listen(t, (_request, response) => {
    response.writeHead(302, { location: "https://example.com/agentplugins" });
    response.end();
  });
  const destination = path.join(os.tmpdir(), `agentplugins-hostile-redirect-${crypto.randomBytes(8).toString("hex")}`);
  t.after(() => fsp.rm(destination, { force: true }));
  let requests = 0;
  await assert.rejects(
    downloadFile("https://github.com/777genius/plugin-kit-ai/releases/download/test/binary", destination, {
      size: BINARY.length,
      sha256: crypto.createHash("sha256").update(BINARY).digest("hex")
    }, {
      request: (_target, options) => {
        requests += 1;
        return http.get(endpoint.url, options);
      }
    }),
    /approved GitHub HTTPS host/
  );
  assert.equal(requests, 1);
  assert.equal(fs.existsSync(destination), false);
});

test("a non-regular binary cache target is preserved", async (t) => {
  const fixture = await fixturePackage(t);
  const endpoint = await listen(t, (_request, response) => {
    response.writeHead(200, { "content-length": fixture.binary.length });
    response.end(fixture.binary);
  });
  const cache = await fsp.mkdtemp(path.join(os.tmpdir(), "agentplugins-nonregular-cache-"));
  t.after(() => fsp.rm(cache, { recursive: true, force: true }));
  const binaryPath = path.join(cache, VERSION, "linux-amd64", "agentplugins");
  await fsp.mkdir(binaryPath, { recursive: true });
  await fsp.writeFile(path.join(binaryPath, "owned.txt"), "keep");
  await assert.rejects(ensureBinary({
    packageRoot: fixture.root,
    cacheRoot: cache,
    request: requestThrough(endpoint.url),
    platform: "linux",
    arch: "x64"
  }), /not a regular file/);
  assert.equal(await fsp.readFile(path.join(binaryPath, "owned.txt"), "utf8"), "keep");
});

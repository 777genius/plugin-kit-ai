#!/usr/bin/env node

import { createHash, randomUUID } from "node:crypto";
import { spawnSync } from "node:child_process";
import { constants } from "node:fs";
import {
  chmod,
  copyFile,
  lstat,
  mkdir,
  readFile,
  realpath,
  rename,
  rm,
  stat,
  writeFile,
} from "node:fs/promises";
import { delimiter, dirname, isAbsolute, join, relative, resolve } from "node:path";
import { fileURLToPath } from "node:url";

const RUNTIME_SCHEMA_VERSION = 1;
const INSTALL_TIMEOUT_MS = 600_000;
const LOCK_STALE_MS = INSTALL_TIMEOUT_MS + 120_000;
const POLL_MS = 250;

function fail(message) {
  process.stderr.write(`agentplugins runtime: ${message}\n`);
  process.exit(1);
}

function sha256(body) {
  return `sha256:${createHash("sha256").update(body).digest("hex")}`;
}

function isSafeRelativePath(value) {
  if (typeof value !== "string" || value.length === 0 || isAbsolute(value)) return false;
  const normalized = value.replaceAll("\\", "/");
  return !normalized.startsWith("../") && !normalized.includes("/../") && normalized !== "..";
}

async function exists(path) {
  try {
    await stat(path);
    return true;
  } catch (error) {
    if (error?.code === "ENOENT") return false;
    throw error;
  }
}

async function readyRuntime(target, expected) {
  try {
    const marker = JSON.parse(await readFile(join(target, ".agentplugins-runtime.json"), "utf8"));
    if (
      marker.schema_version !== RUNTIME_SCHEMA_VERSION ||
      marker.lock_digest !== expected.lockDigest ||
      marker.package !== expected.config.package ||
      marker.version !== expected.config.version ||
      marker.omit_optional !== expected.config.omit_optional ||
      marker.entrypoint !== expected.config.entrypoint
    ) return null;
    const entrypoint = resolve(target, expected.config.entrypoint);
    const rel = relative(target, entrypoint);
    if (!isSafeRelativePath(rel) || !(await exists(entrypoint))) return null;
    return entrypoint;
  } catch (error) {
    if (error?.code === "ENOENT" || error instanceof SyntaxError) return null;
    throw error;
  }
}

async function acquireLock(lockPath, target, expected) {
  for (;;) {
    try {
      await mkdir(lockPath);
      await writeFile(
        join(lockPath, "owner.json"),
        `${JSON.stringify({ pid: process.pid, created_at: new Date().toISOString() })}\n`,
        { mode: 0o600 },
      );
      return;
    } catch (error) {
      if (error?.code !== "EEXIST") throw error;
      const ready = await readyRuntime(target, expected);
      if (ready) return ready;
      const lockStat = await stat(lockPath).catch(() => null);
      if (lockStat && Date.now() - lockStat.mtimeMs > LOCK_STALE_MS) {
        await rm(lockPath, { recursive: true, force: true });
        continue;
      }
      await new Promise((accept) => setTimeout(accept, POLL_MS));
    }
  }
}

async function installRuntime(runtimeRoot, pluginData, config, lockBody, lockDigest) {
  const key = lockDigest.slice("sha256:".length);
  const store = join(pluginData, "npm-runtime");
  const target = join(store, key);
  const expected = { config, lockDigest };
  await mkdir(store, { recursive: true, mode: 0o700 });
  const existing = await readyRuntime(target, expected);
  if (existing) return existing;

  const lockPath = `${target}.lock`;
  const acquired = await acquireLock(lockPath, target, expected);
  if (typeof acquired === "string") return acquired;

  const temporary = join(store, `.tmp-${key}-${process.pid}-${randomUUID()}`);
  try {
    await mkdir(temporary, { recursive: false, mode: 0o700 });
    await copyFile(join(runtimeRoot, "package.json"), join(temporary, "package.json"), constants.COPYFILE_EXCL);
    await writeFile(join(temporary, "package-lock.json"), lockBody, { mode: 0o600, flag: "wx" });
    const npm = process.platform === "win32" ? "npm.cmd" : "npm";
    const installArgs = ["ci", "--ignore-scripts", "--omit=dev"];
    if (config.omit_optional) installArgs.push("--omit=optional");
    installArgs.push("--no-audit", "--no-fund");
    const result = spawnSync(
      npm,
      installArgs,
      {
        cwd: temporary,
        env: {
          ...process.env,
          npm_config_cache: join(pluginData, "npm-cache"),
          npm_config_ignore_scripts: "true",
          npm_config_audit: "false",
          npm_config_fund: "false",
          npm_config_update_notifier: "false",
        },
        encoding: "utf8",
        stdio: ["ignore", "ignore", "pipe"],
        timeout: INSTALL_TIMEOUT_MS,
        windowsHide: true,
      },
    );
    if (result.error) throw result.error;
    if (result.status !== 0) {
      throw new Error((result.stderr || `npm ci exited with ${result.status}`).trim());
    }
    const entrypoint = resolve(temporary, config.entrypoint);
    const rel = relative(temporary, entrypoint);
    if (!isSafeRelativePath(rel) || !(await exists(entrypoint))) {
      throw new Error(`locked package did not provide ${config.entrypoint}`);
    }
    await writeFile(
      join(temporary, ".agentplugins-runtime.json"),
      `${JSON.stringify({
        schema_version: RUNTIME_SCHEMA_VERSION,
        lock_digest: lockDigest,
        package: config.package,
        version: config.version,
        omit_optional: config.omit_optional,
        entrypoint: config.entrypoint,
      })}\n`,
      { mode: 0o600, flag: "wx" },
    );
    await rm(target, { recursive: true, force: true });
    await rename(temporary, target);
    const installed = await readyRuntime(target, expected);
    if (!installed) throw new Error("installed runtime failed its ready check");
    return installed;
  } finally {
    await rm(temporary, { recursive: true, force: true });
    await rm(lockPath, { recursive: true, force: true });
  }
}

async function main() {
  const runtimeRoot = dirname(fileURLToPath(import.meta.url));
  const pluginDataValue = process.env.PLUGIN_DATA;
  if (!pluginDataValue || !isAbsolute(pluginDataValue)) {
    throw new Error("the client did not provide an absolute PLUGIN_DATA directory");
  }
  await mkdir(pluginDataValue, { recursive: true, mode: 0o700 });
  const pluginDataStat = await lstat(pluginDataValue);
  if (!pluginDataStat.isDirectory() || pluginDataStat.isSymbolicLink()) {
    throw new Error("PLUGIN_DATA must be a real directory, not a symlink");
  }
  await chmod(pluginDataValue, 0o700);
  const pluginData = await realpath(pluginDataValue);
  const configBody = await readFile(join(runtimeRoot, "runtime.json"));
  const config = JSON.parse(configBody.toString("utf8"));
  const lockBody = await readFile(join(runtimeRoot, "package-lock.json"));
  const lockDigest = sha256(lockBody);
  if (
    config.schema_version !== RUNTIME_SCHEMA_VERSION ||
    typeof config.package !== "string" ||
    typeof config.version !== "string" ||
    typeof config.omit_optional !== "boolean" ||
    !isSafeRelativePath(config.entrypoint) ||
    config.package_lock_sha256 !== lockDigest
  ) throw new Error("runtime.json does not match the locked npm runtime");

  const entrypoint = await installRuntime(runtimeRoot, pluginData, config, lockBody, lockDigest);
  const materializedRoot = join(pluginData, "npm-runtime", lockDigest.slice("sha256:".length));
  const runtimeBin = join(materializedRoot, "node_modules", ".bin");
  if (!(await exists(runtimeBin))) {
    throw new Error("locked package did not provide node_modules/.bin");
  }
  const pathKey = Object.keys(process.env).find((key) => key.toLowerCase() === "path") || "PATH";
  const inheritedPath = process.env[pathKey];
  const childEnv = {
    ...process.env,
    [pathKey]: inheritedPath ? `${runtimeBin}${delimiter}${inheritedPath}` : runtimeBin,
  };
  const child = spawnSync(process.execPath, [entrypoint, ...process.argv.slice(2)], {
    env: childEnv,
    stdio: "inherit",
    windowsHide: true,
  });
  if (child.error) throw child.error;
  process.exit(child.status ?? 1);
}

main().catch((error) => fail(error instanceof Error ? error.message : String(error)));

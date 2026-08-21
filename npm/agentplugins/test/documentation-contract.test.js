"use strict";

const assert = require("node:assert/strict");
const fs = require("node:fs");
const path = require("node:path");
const test = require("node:test");

const documents = [
  ["root README", path.resolve(__dirname, "../../../README.md")],
  ["npm README", path.resolve(__dirname, "../README.md")]
];

const allowedTargets = new Set(["codex", "chatgpt", "cursor", "copilot", "vscode", "kiro"]);
const exactSourcePattern = /^(?:github:)?[A-Za-z0-9][A-Za-z0-9-]*\/[A-Za-z0-9][A-Za-z0-9._-]*@[0-9a-f]{40}\/\/[A-Za-z0-9._/-]+$/;

function bashCommands(markdown) {
  const commands = [];
  for (const match of markdown.matchAll(/```(?:bash|shell)\s*\n([\s\S]*?)```/g)) {
    for (const line of match[1].split(/\r?\n/)) {
      const command = line.trim();
      if (command && !command.startsWith("#")) commands.push(command);
    }
  }
  return commands;
}

function commandArgument(command, flag) {
  const words = command.split(/\s+/);
  const index = words.indexOf(flag);
  return index < 0 ? "" : words[index + 1] || "";
}

test("public Agentplugins documentation keeps copyable commands within the CLI contract", () => {
  for (const [label, filename] of documents) {
    const markdown = fs.readFileSync(filename, "utf8");
    const commands = bashCommands(markdown).filter((command) => command.startsWith("npx universal-agent-plugins "));
    assert.ok(commands.length > 0, `${label} has no copyable universal-agent-plugins commands`);

    for (const command of commands) {
      assert.doesNotMatch(command, /(?:^|\s)--yes(?:\s|$)/, `${label}: --yes is not a public option`);
      const target = commandArgument(command, "--target");
      if (target) {
        assert.match(target, /^[a-z]+(?:,[a-z]+)*$/, `${label}: malformed --target value in ${command}`);
        const values = target.split(",");
        assert.equal(new Set(values).size, values.length, `${label}: duplicate target in ${command}`);
        for (const value of values) assert.ok(allowedTargets.has(value), `${label}: unknown target ${value}`);
      }

      const words = command.split(/\s+/);
      for (const word of words) {
        if (!word.includes("@") || !word.includes("//")) continue;
        assert.match(word, exactSourcePattern, `${label}: direct GitHub source must use a full lowercase SHA`);
      }
    }

    for (const verb of ["add", "update", "repair", "remove", "switch"]) {
      assert.ok(commands.some((command) => command.startsWith(`npx universal-agent-plugins ${verb} `)), `${label}: missing ${verb} example`);
    }
    for (const command of commands.filter((value) => value.startsWith("npx universal-agent-plugins switch "))) {
      assert.ok(commandArgument(command, "--to"), `${label}: switch example requires --to`);
      assert.equal(commandArgument(command, "--target"), "", `${label}: switch must not use --target`);
    }
    assert.ok(commands.some((command) => command.includes("--target codex,cursor,kiro")), `${label}: missing explicit three-target example`);
  }
});

test("public documentation keeps the package, binary, manifest, and safety contracts", () => {
  for (const [label, filename] of documents) {
    const markdown = fs.readFileSync(filename, "utf8");
    assert.match(markdown, /`universal-agent-plugins`[\s\S]{0,160}(?:public )?npm package/i, `${label}: npm package identity missing`);
    assert.match(markdown, /installs? the `agentplugins` binary/i, `${label}: installed binary identity missing`);
    assert.match(markdown, /same (?:installer and )?lifecycle manager|not separate engines/i, `${label}: shared engine relationship missing`);
    assert.match(markdown, /signed\s+(?:\n)?(?:\[[^\]]+\]\([^\n]+\)|Universal Agent Plugins Directory)/i, `${label}: signed Directory contract missing`);
    assert.doesNotMatch(markdown, /pinned\s+(?:legacy\s+)?catalog|catalog\s+v[12]|first\s+catalog/i, `${label}: stale catalog contract`);
    assert.match(markdown, /root `plugin\.json` is the install authority/i, `${label}: plugin.json authority missing`);
    assert.match(markdown, /`plugin\.yaml`[\s\S]{0,100}legacy[\s\S]{0,100}authoring input only/i, `${label}: legacy plugin.yaml boundary missing`);
    assert.match(markdown, /silently override `plugin\.json`|silent(?:ly)?[^.\n]{0,60}override/i, `${label}: manifest override prohibition missing`);
    assert.match(markdown, /preflight/i, `${label}: preflight missing`);
    assert.match(markdown, /`--dry-run`[\s\S]{0,120}(?:read-only|without\s+writing)/i, `${label}: dry-run behavior missing`);
    assert.match(markdown, /publisher[\s\S]{0,120}source|source[\s\S]{0,120}publisher/i, `${label}: source provenance missing`);
    assert.match(markdown, /rollback|rolls? (?:the group )?back/i, `${label}: rollback boundary missing`);
    assert.match(markdown, /manual\s+activation|manual-activation/i, `${label}: manual activation boundary missing`);
    assert.match(markdown, /OAuth[\s\S]{0,160}(?:prompt|consent)[\s\S]{0,160}user-controlled/i, `${label}: OAuth prompt boundary missing`);
    assert.match(markdown, /not every|not as a claim that every/i, `${label}: verification scope caveat missing`);
  }
});

test("copyable direct-source examples use a marked replacement full SHA", () => {
  const placeholder = "0123456789abcdef0123456789abcdef01234567";
  for (const [label, filename] of documents) {
    const markdown = fs.readFileSync(filename, "utf8");
    assert.ok(markdown.includes(`owner/repo@${placeholder}//plugins/my-plugin`), `${label}: full-SHA source example missing`);
    assert.ok(markdown.includes("add ./my-plugin --target cursor"), `${label}: local source example missing`);
    assert.match(markdown, new RegExp(`replace[\\s\\S]{0,180}${placeholder}|${placeholder}[\\s\\S]{0,180}replace`, "i"), `${label}: SHA replacement instruction missing`);
    assert.doesNotMatch(markdown, /@commit(?:\/\/|\b)/i, `${label}: literal @commit is not copyable`);
  }
});

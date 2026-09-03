"use strict";

const assert = require("node:assert/strict");
const fs = require("node:fs");
const path = require("node:path");
const test = require("node:test");

test("npm facade metadata exactly names the UAP product endpoints", () => {
  const pkg = JSON.parse(fs.readFileSync(path.resolve(__dirname, "../package.json"), "utf8"));
  assert.equal(pkg.homepage, "https://777genius.github.io/universal-agent-plugins/");
  assert.deepEqual(pkg.repository, {
    type: "git",
    url: "git+https://github.com/777genius/universal-agent-plugins.git",
    directory: "npm/agentplugins"
  });
  assert.deepEqual(pkg.bugs, { url: "https://github.com/777genius/universal-agent-plugins/issues" });
});

test("detached package execution sentinel", (t) => {
  const expectedRoot = process.env.AGENTPLUGINS_DETACHED_ASSERT_ROOT;
  if (!expectedRoot) {
    t.skip("only asserted by the detached staged-package subprocess");
    return;
  }
  assert.equal(
    fs.realpathSync(path.resolve(__dirname, "..")),
    fs.realpathSync(path.resolve(expectedRoot))
  );
  assert.equal(fs.existsSync(path.resolve(__dirname, "../../../README.md")), false);
});

const documents = [
  ["root README", path.resolve(__dirname, "../../../README.md")],
  ["npm README", path.resolve(__dirname, "../README.md")]
];

function packagedDocuments() {
  const available = documents.filter(([, filename]) => fs.existsSync(filename));
  assert.ok(available.length > 0, "no packaged Agentplugins documentation found");
  return available;
}

const allowedTargets = new Set([
  "codex", "chatgpt", "cursor", "copilot", "vscode", "kiro",
  "claude", "gemini", "opencode", "cline", "windsurf"
]);
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
  for (const [label, filename] of packagedDocuments()) {
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

test("public documentation states the facade and engine ownership boundary", () => {
  const packaged = fs.readFileSync(documents[1][1], "utf8");
  const boundaryDocuments = [["npm README", packaged]];
  if (fs.existsSync(documents[0][1])) {
    boundaryDocuments.unshift(["root README", fs.readFileSync(documents[0][1], "utf8")]);
  }
  for (const [label, markdown] of boundaryDocuments) {
    assert.match(markdown, /product home[\s\S]{0,100}(?:npm facade|facade source)/i, `: facade product ownership missing`);
    assert.match(markdown, /plugin-kit-ai[\s\S]{0,180}(?:Go implementation engine|implementation engine)/i, `: implementation engine ownership missing`);
    assert.match(markdown, /not duplicated/i, `: no-duplicate-engine boundary missing`);
    assert.match(markdown, /installs? the `agentplugins` binary/i, `: installed binary identity missing`);
  }
});

test("package documentation labels client evidence historical and commit-pins its source", () => {
  const markdown = fs.readFileSync(documents[1][1], "utf8");
  assert.match(markdown, /historical lifecycle evidence collected for[\s\S]{0,80}0\.1\.22/i);
  assert.match(markdown, /not evidence for the current npm release/i);
  assert.match(markdown, /https:\/\/github\.com\/777genius\/plugin-kit-ai\/blob\/4b25a45e1574bab7a4f49e48905a3b3b2647e917\/docs\/AGENTPLUGINS_CLIENT_E2E\.md/);
  assert.doesNotMatch(markdown, /plugin-kit-ai\/blob\/(?:main|master)\/docs\/AGENTPLUGINS_CLIENT_E2E\.md/);
});

test("copyable direct-source examples use a marked replacement full SHA", () => {
  const placeholder = "0123456789abcdef0123456789abcdef01234567";
  for (const [label, filename] of [documents[1]]) {
    const markdown = fs.readFileSync(filename, "utf8");
    assert.ok(markdown.includes(`owner/repo@${placeholder}//plugins/my-plugin`), `${label}: full-SHA source example missing`);
    assert.ok(markdown.includes("add ./my-plugin --target cursor"), `${label}: local source example missing`);
    assert.match(markdown, new RegExp(`replace[\\s\\S]{0,180}${placeholder}|${placeholder}[\\s\\S]{0,180}replace`, "i"), `${label}: SHA replacement instruction missing`);
    assert.doesNotMatch(markdown, /@commit(?:\/\/|\b)/i, `${label}: literal @commit is not copyable`);
  }
});

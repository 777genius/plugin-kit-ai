"use strict";

const assert = require("node:assert/strict");
const crypto = require("node:crypto");
const fs = require("node:fs");
const path = require("node:path");
const test = require("node:test");

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

test("public documentation keeps the package, binary, manifest, and safety contracts", () => {
  for (const [label, filename] of packagedDocuments()) {
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
  for (const [label, filename] of packagedDocuments()) {
    const markdown = fs.readFileSync(filename, "utf8");
    assert.ok(markdown.includes(`owner/repo@${placeholder}//plugins/my-plugin`), `${label}: full-SHA source example missing`);
    assert.ok(markdown.includes("add ./my-plugin --target cursor"), `${label}: local source example missing`);
    assert.match(markdown, new RegExp(`replace[\\s\\S]{0,180}${placeholder}|${placeholder}[\\s\\S]{0,180}replace`, "i"), `${label}: SHA replacement instruction missing`);
    assert.doesNotMatch(markdown, /@commit(?:\/\/|\b)/i, `${label}: literal @commit is not copyable`);
  }
});

test("checked-in client E2E evidence remains exact and auditable", () => {
  const evidenceDoc = path.resolve(__dirname, "../../../docs/AGENTPLUGINS_CLIENT_E2E.md");
  const transcriptPath = path.resolve(__dirname, "../../../docs/evidence/agentplugins-client-e2e-2026-08-30.json");
  const markdown = fs.readFileSync(evidenceDoc, "utf8");
  const transcriptBytes = fs.readFileSync(transcriptPath);
  const transcript = JSON.parse(transcriptBytes.toString("utf8"));
  const digest = crypto.createHash("sha256").update(transcriptBytes).digest("hex");

  assert.equal(transcript.schema_version, 1);
  assert.match(transcript.installer.commit, /^[0-9a-f]{40}$/);
  assert.match(transcript.installer.tree, /^[0-9a-f]{40}$/);
  assert.match(transcript.installer.binary_sha256, /^[0-9a-f]{64}$/);
  assert.equal(
    transcript.package.selector,
    "ChromeDevTools/chrome-devtools-mcp@cb39d1d835c3baa3eff87501cd8c1de020604789"
  );
  assert.ok(markdown.includes(transcript.installer.commit), "evidence doc lost tested installer commit");
  assert.ok(markdown.includes(transcript.installer.tree), "evidence doc lost tested installer tree");
  assert.ok(markdown.includes(digest), "evidence doc lost exact transcript digest");

  const add = transcript.transcript.find((entry) => entry.step === "add");
  const repair = transcript.transcript.find((entry) => entry.step === "repair");
  const remove = transcript.transcript.find((entry) => entry.step === "remove");
  const postRemove = transcript.transcript.find((entry) => entry.step === "post_remove");
  const targets = ["claude", "gemini", "opencode", "cline", "windsurf"];
  assert.equal(add.acquisition_count, 1);
  for (const target of targets) {
    assert.equal(add.targets[target].outcome, "passed", `add evidence missing ${target}`);
    assert.equal(repair.targets[target], "passed", `repair evidence missing ${target}`);
    assert.equal(remove.targets[target], "external_completed", `remove evidence missing ${target}`);
  }
  assert.equal(postRemove.checks.agentplugins_installation_count, 0);
  assert.equal(transcript.claim_boundary.lifecycle_e2e, true);
  for (const claim of ["browser_tool_runtime_e2e", "model_turn_e2e", "login_e2e", "oauth_e2e"]) {
    assert.equal(transcript.claim_boundary[claim], false, `evidence overclaims ${claim}`);
  }
});

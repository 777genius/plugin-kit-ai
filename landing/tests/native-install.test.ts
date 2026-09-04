import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { describe, it } from 'node:test';
import { dirname, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';
import { applyCliInvocation, getCliInvocation } from '../utils/cliInvocation.ts';

const root = resolve(dirname(fileURLToPath(import.meta.url)), '..');
const locales = ['en', 'es', 'fr', 'ru', 'zh'];

describe('native-first installation', () => {
  it('keeps the native Go CLI as the default command invocation', () => {
    assert.equal(getCliInvocation(), 'agentplugins');
    assert.equal(
      applyCliInvocation('universal-agent-plugins add context7'),
      'agentplugins add context7',
    );
    assert.equal(
      applyCliInvocation('universal-agent-plugins add context7', 'npx universal-agent-plugins'),
      'npx universal-agent-plugins add context7',
    );
  });

  it('publishes the same native and zero-install channels in every locale', () => {
    for (const locale of locales) {
      const content = JSON.parse(
        readFileSync(resolve(root, 'content', `${locale}.json`), 'utf8'),
      ) as {
        installChannels: Array<{
          id: string;
          command?: string;
          invocation?: string;
          recommended?: boolean;
        }>;
      };
      const selected = content.installChannels.filter((channel) =>
        ['brew', 'script', 'npm'].includes(channel.id),
      );
      assert.deepEqual(
        selected.map((channel) => channel.id),
        ['brew', 'npm', 'script'],
        locale,
      );
      assert.equal(selected.filter((channel) => channel.recommended).length, 1, locale);
      assert.equal(selected[0]?.recommended, true, locale);
      assert.equal(selected[0]?.command, 'brew install 777genius/agentplugins/agentplugins');
      assert.equal(selected[0]?.invocation, 'agentplugins');
      assert.equal(selected[1]?.invocation, 'npx universal-agent-plugins');
      assert.equal(selected[2]?.invocation, '$HOME/.local/bin/agentplugins');
      assert.ok(
        selected.every((channel) => !channel.command?.includes('plugin-kit-ai')),
        locale,
      );
    }
  });
});

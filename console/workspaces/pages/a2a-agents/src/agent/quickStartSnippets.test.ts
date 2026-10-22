/**
 * Copyright (c) 2026, WSO2 LLC. (https://www.wso2.com).
 *
 * WSO2 LLC. licenses this file to you under the Apache License,
 * Version 2.0 (the "License"); you may not use this file except
 * in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing,
 * software distributed under the License is distributed on an
 * "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
 * KIND, either express or implied.  See the License for the
 * specific language governing permissions and limitations
 * under the License.
 */


import { describe, expect, it } from 'vitest';
import {
  ENDPOINT_COMMENT,
  SNIPPET_LANGUAGES,
  UNMANAGED_COMMENT,
  buildQuickStartSnippet,
} from './quickStartSnippets';

const langs = SNIPPET_LANGUAGES;
const base = {
  cardUrl: 'https://gw/orders/.well-known/agent-card.json',
  endpointUrl: 'https://gw/orders/rpc',
  auth: null,
};
const withKey = { ...base, auth: { header: 'X-API-Key', value: 'k1' } };

describe('buildQuickStartSnippet', () => {
  it.each(langs)('%s names the card URL and endpoint', (l) => {
    const s = buildQuickStartSnippet(l, base);
    expect(s).toContain(base.cardUrl);
    expect(s).toContain(base.endpointUrl);
  });

  it.each(langs)('%s sends the auth header when given and none otherwise', (l) => {
    const withAuth = buildQuickStartSnippet(l, withKey);
    expect(withAuth).toContain('X-API-Key');
    expect(withAuth).toContain('k1');
    if (l === 'curl') {
      expect(withAuth.match(/-H 'X-API-Key: k1'/g)).toHaveLength(2);
    }
    const without = buildQuickStartSnippet(l, base);
    expect(without).not.toContain('X-API-Key');
    expect(without).not.toContain('Authorization');
  });

  it.each(langs)('%s emits the unmanaged note only with the flag and no auth', (l) => {
    expect(buildQuickStartSnippet(l, { ...base, authUnmanaged: true })).toContain(
      UNMANAGED_COMMENT,
    );
    expect(buildQuickStartSnippet(l, base)).not.toContain("does not manage this agent's security");
    expect(buildQuickStartSnippet(l, { ...withKey, authUnmanaged: true })).not.toContain(
      "does not manage this agent's security",
    );
  });

  it('curl posts a v1 JSON-RPC SendMessage', () => {
    const s = buildQuickStartSnippet('curl', base);
    expect(s).toContain(`-X POST '${base.endpointUrl}'`);
    expect(s).toContain('"method": "SendMessage"');
    expect(s).toContain('"role": "ROLE_USER"');
    expect(s).toContain('A2A-Version: 1.0');
  });

  it('curl shell-quotes a single quote', () =>
    expect(
      buildQuickStartSnippet('curl', { ...base, auth: { header: 'X-API-Key', value: "it's" } }),
    ).toContain(`'X-API-Key: it'\\''s'`));

  it.each(['python', 'typescript'] as const)('%s escapes quotes', (l) =>
    expect(
      buildQuickStartSnippet(l, { ...base, auth: { header: 'X-API-Key', value: 'a"b' } }),
    ).toContain('"a\\"b"'));

  it.each(langs)('%s never puts a card value in a comment', (l) => {
    const lines = buildQuickStartSnippet(l, {
      ...base,
      endpointUrl: 'https://x/rpc evil()//',
      authUnmanaged: true,
    }).split('\n');
    for (const line of lines.filter((x) => x.startsWith('#') || x.startsWith('//'))) {
      expect(line).not.toContain('x/rpc');
      expect(line).not.toContain('evil');
    }
  });

  it('typescript escapes line separators', () => {
    const s = buildQuickStartSnippet('typescript', { ...base, endpointUrl: 'https://x/\u2028y\u2029z' });
    expect(s).not.toMatch(/[\u2028\u2029]/);
    expect(s).toContain('\\u2028');
    expect(s).toContain('\\u2029');
  });

  it('typescript has no top-level await and ends with the main footer', () => {
    const s = buildQuickStartSnippet('typescript', base);
    expect(s).not.toMatch(/^\S.*\bawait\b/m);
    expect(
      s.trimEnd().endsWith('main().catch((err) => {\n  console.error(err);\n  process.exitCode = 1;\n});'),
    ).toBe(true);
  });

  it('emits the constant endpoint comment before the endpoint variable', () => {
    expect(buildQuickStartSnippet('python', base)).toContain(
      `# ${ENDPOINT_COMMENT}\nJSONRPC_URL = "${base.endpointUrl}"`,
    );
    expect(buildQuickStartSnippet('typescript', base)).toContain(
      `// ${ENDPOINT_COMMENT}\nconst jsonRpcUrl = "${base.endpointUrl}";`,
    );
  });

  it('builds with an empty card URL', () =>
    expect(buildQuickStartSnippet('python', { ...base, cardUrl: '' })).toContain('CARD_URL = ""'));
});

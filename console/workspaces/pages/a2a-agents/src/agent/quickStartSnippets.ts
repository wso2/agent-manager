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


export type SnippetLanguage = 'python' | 'typescript' | 'curl';

export const SNIPPET_LANGUAGES: readonly SnippetLanguage[] = ['python', 'typescript', 'curl'];

export const SNIPPET_LABELS: Record<SnippetLanguage, string> = {
  python: 'Python',
  typescript: 'TypeScript',
  curl: 'curl',
};

export const SNIPPET_HIGHLIGHT: Record<SnippetLanguage, string> = {
  python: 'python',
  typescript: 'typescript',
  curl: 'bash',
};

export const ENDPOINT_COMMENT = 'The card routes JSON-RPC calls to this endpoint.';
export const UNMANAGED_COMMENT =
  "The platform does not manage this agent's security; add whatever auth its agent card declares.";
export const API_KEY_PLACEHOLDER = '<YOUR_API_KEY>';
export const ACCESS_TOKEN_PLACEHOLDER = '<ACCESS_TOKEN>';

export interface SnippetAuth {
  header: string;
  value: string;
}

export interface SnippetInputs {
  cardUrl: string;
  endpointUrl: string;
  auth: SnippetAuth | null;
  authUnmanaged?: boolean;
}

const pythonLiteral = (v: string): string => JSON.stringify(v);

const typescriptLiteral = (v: string): string =>
  JSON.stringify(v).replace(/\u2028/g, '\\u2028').replace(/\u2029/g, '\\u2029');

const shellLiteral = (v: string): string => `'${v.replace(/'/g, "'\\''")}'`;

const showsUnmanagedNote = (inputs: SnippetInputs): boolean =>
  inputs.authUnmanaged === true && inputs.auth === null;

function pythonSnippet(inputs: SnippetInputs): string {
  const { auth } = inputs;
  const clientArgs = auth ? `headers={${pythonLiteral(auth.header)}: ${pythonLiteral(auth.value)}}` : '';
  const lines = [
    '# pip install a2a-sdk',
    'import asyncio',
    'import uuid',
    '',
    'import httpx',
    'from a2a.client import ClientConfig, create_client',
    'from a2a.types import Message, Part, Role, SendMessageRequest',
    '',
    `CARD_URL = ${pythonLiteral(inputs.cardUrl)}`,
    `# ${ENDPOINT_COMMENT}`,
    `JSONRPC_URL = ${pythonLiteral(inputs.endpointUrl)}`,
  ];
  if (showsUnmanagedNote(inputs)) {
    lines.push(`# ${UNMANAGED_COMMENT}`);
  }
  lines.push(
    '',
    'async def main() -> None:',
    `    async with httpx.AsyncClient(${clientArgs}) as http:`,
    '        client = await create_client(',
    '            CARD_URL, ClientConfig(httpx_client=http), relative_card_path="/"',
    '        )',
    '        message = Message(',
    '            message_id=str(uuid.uuid4()),',
    '            role=Role.ROLE_USER,',
    '            parts=[Part(text="Hello")],',
    '        )',
    '        async for event in client.send_message(SendMessageRequest(message=message)):',
    '            print(event)',
    '',
    '',
    'asyncio.run(main())',
  );
  return lines.join('\n');
}

function typescriptSnippet(inputs: SnippetInputs): string {
  const { auth } = inputs;
  const headersLiteral = auth
    ? `{ ${typescriptLiteral(auth.header)}: ${typescriptLiteral(auth.value)} }`
    : '{}';
  const lines = [
    '// npm install @a2a-js/sdk',
    'import { Role, type SendMessageRequest } from "@a2a-js/sdk";',
    'import {',
    '  ClientFactory,',
    '  ClientFactoryOptions,',
    '  DefaultAgentCardResolver,',
    '} from "@a2a-js/sdk/client";',
    '',
    `const cardUrl = ${typescriptLiteral(inputs.cardUrl)};`,
    `// ${ENDPOINT_COMMENT}`,
    `const jsonRpcUrl = ${typescriptLiteral(inputs.endpointUrl)};`,
  ];
  if (showsUnmanagedNote(inputs)) {
    lines.push(`// ${UNMANAGED_COMMENT}`);
  }
  lines.push(
    `const headers: Record<string, string> = ${headersLiteral};`,
    '',
    'const fetchWithHeaders: typeof fetch = (input, init) =>',
    '  fetch(input, { ...init, headers: { ...Object.fromEntries(new Headers(init?.headers)), ...headers } });',
    '',
    'async function main(): Promise<void> {',
    '  const factory = new ClientFactory(',
    '    ClientFactoryOptions.createFrom(ClientFactoryOptions.default, {',
    '      cardResolver: new DefaultAgentCardResolver({ fetchImpl: fetchWithHeaders }),',
    '    }),',
    '  );',
    '  const client = await factory.createFromUrl(cardUrl, "");',
    '',
    '  const request: SendMessageRequest = {',
    '    tenant: "",',
    '    message: {',
    '      messageId: crypto.randomUUID(),',
    '      contextId: "",',
    '      taskId: "",',
    '      role: Role.ROLE_USER,',
    '      parts: [',
    '        { content: { $case: "text", value: "Hello" }, metadata: undefined, filename: "", mediaType: "text/plain" },',
    '      ],',
    '      metadata: undefined,',
    '      extensions: [],',
    '      referenceTaskIds: [],',
    '    },',
    '    configuration: undefined,',
    '    metadata: undefined,',
    '  };',
    '  const result = await client.sendMessage(request, { serviceParameters: headers });',
    '  console.log(JSON.stringify(result, null, 2));',
    '}',
    '',
    'main().catch((err) => {',
    '  console.error(err);',
    '  process.exitCode = 1;',
    '});',
  );
  return lines.join('\n');
}

const CURL_MESSAGE_BODY = {
  jsonrpc: '2.0',
  id: 1,
  method: 'SendMessage',
  params: {
    message: {
      messageId: 'msg-1',
      role: 'ROLE_USER',
      parts: [{ text: 'Hello' }],
    },
  },
};

function curlSnippet(inputs: SnippetInputs): string {
  const { auth } = inputs;
  const authHeader = auth ? [`  -H ${shellLiteral(`${auth.header}: ${auth.value}`)}`] : [];
  const continued = (parts: string[]): string => parts.join(' \\\n');
  const lines: string[] = [];
  if (showsUnmanagedNote(inputs)) {
    lines.push(`# ${UNMANAGED_COMMENT}`);
  }
  lines.push(
    '# Fetch the agent card',
    continued([`curl -s ${shellLiteral(inputs.cardUrl)}`, ...authHeader]),
    '',
    '# Send a message (JSON-RPC SendMessage)',
    continued([
      `curl -s -X POST ${shellLiteral(inputs.endpointUrl)}`,
      "  -H 'Content-Type: application/json'",
      "  -H 'A2A-Version: 1.0'",
      ...authHeader,
      `  -d ${shellLiteral(JSON.stringify(CURL_MESSAGE_BODY, null, 2))}`,
    ]),
  );
  return lines.join('\n');
}

export function buildQuickStartSnippet(language: SnippetLanguage, inputs: SnippetInputs): string {
  switch (language) {
    case 'python':
      return pythonSnippet(inputs);
    case 'typescript':
      return typescriptSnippet(inputs);
    case 'curl':
      return curlSnippet(inputs);
  }
}

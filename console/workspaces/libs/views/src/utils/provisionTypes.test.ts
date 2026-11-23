/**
 * Copyright (c) 2026, WSO2 LLC. (https://www.wso2.com).
 *
 * WSO2 LLC. licenses this file to you under the Apache License,
 * Version 2.0 (the "License"); you may not use this file except
 * in compliance with the License.
 * You may obtain a copy of the License at
 *
 * http://www.apache.org/licenses/LICENSE-2.0
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
  displayAgentSubType,
  displayProvisionTypes,
  isExternalAgent,
  servesA2A,
} from './provisionTypes';

describe('displayProvisionTypes', () => {
  it.each([
    [undefined, 'Unknown'],
    ['', 'Unknown'],
    ['external', 'External'],
    ['internal', 'Platform'],
    ['other', undefined],
  ])('%j -> %j', (input, expected) => {
    expect(displayProvisionTypes(input)).toBe(expected);
  });
});

describe('displayAgentSubType', () => {
  it.each([
    ['custom-api', 'Custom API'],
    ['chat-api', 'Chat'],
    ['a2a-agent', 'A2A'],
    ['mystery', undefined],
    [undefined, undefined],
  ])('%j -> %j', (input, expected) => {
    expect(displayAgentSubType(input)).toBe(expected);
  });
});

describe('isExternalAgent / servesA2A', () => {
  it.each([
    [undefined, false, false],
    [{}, false, false],
    [{ agentType: { type: 'external-agent-api' } }, true, true],
    [{ agentType: { type: 'agent-api', subType: 'a2a-agent' } }, false, true],
    [{ agentType: { type: 'agent-api', subType: 'chat-api' } }, false, false],
  ])('%j -> external=%s, a2a=%s', (agent, external, a2a) => {
    expect(isExternalAgent(agent)).toBe(external);
    expect(servesA2A(agent)).toBe(a2a);
  });
});

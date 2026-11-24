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
import { buildConfigureTabHref } from './configureTabLink';

describe('buildConfigureTabHref', () => {
  it('builds the agent configure path with the tab query', () => {
    expect(buildConfigureTabHref('acme', 'proj', 'bot', 'llm')).toBe(
      '/org/acme/project/proj/agents/bot/configure?tab=llm',
    );
  });

  it('adds the environment when one is given', () => {
    expect(buildConfigureTabHref('acme', 'proj', 'bot', 'tools', 'dev')).toBe(
      '/org/acme/project/proj/agents/bot/configure?tab=tools&env=dev',
    );
  });

});

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

import { useState } from 'react';
import { Box, Stack, ToggleButton, ToggleButtonGroup, Typography } from '@wso2/oxygen-ui';
import { CodeBlock } from '@agent-management-platform/shared-component';
import {
  buildQuickStartSnippet,
  SNIPPET_HIGHLIGHT,
  SNIPPET_LABELS,
  SNIPPET_LANGUAGES,
  type SnippetInputs,
  type SnippetLanguage,
} from './quickStartSnippets';

export const QUICK_START_FIELD_ID = 'a2a-quick-start';

export interface A2AQuickStartProps {
  /** An empty endpointUrl means the agent has no JSON-RPC endpoint to call. */
  inputs: SnippetInputs;
}

export function A2AQuickStart({ inputs }: A2AQuickStartProps) {
  const [language, setLanguage] = useState<SnippetLanguage>('python');

  if (inputs.endpointUrl === '') {
    return (
      <Typography variant="body2">
        This agent declares no JSON-RPC endpoint, so there is no quick start.
      </Typography>
    );
  }

  return (
    <Stack spacing={1}>
      <ToggleButtonGroup
        aria-label="Snippet language"
        exclusive
        size="small"
        value={language}
        onChange={(_, next: SnippetLanguage | null) => {
          if (next !== null) setLanguage(next);
        }}
      >
        {SNIPPET_LANGUAGES.map((option) => (
          <ToggleButton key={option} value={option}>
            {SNIPPET_LABELS[option]}
          </ToggleButton>
        ))}
      </ToggleButtonGroup>
      <Box>
        <CodeBlock
          code={buildQuickStartSnippet(language, inputs)}
          language={SNIPPET_HIGHLIGHT[language]}
          fieldId={QUICK_START_FIELD_ID}
        />
      </Box>
    </Stack>
  );
}

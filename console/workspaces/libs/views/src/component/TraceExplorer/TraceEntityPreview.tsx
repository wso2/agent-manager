/**
 * Copyright (c) 2025, WSO2 LLC. (https://www.wso2.com).
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

import { Span } from '@agent-management-platform/types';
import { Typography } from '@wso2/oxygen-ui';
import { ArrowRight } from '@wso2/oxygen-ui-icons-react';
import { useMemo } from 'react';

interface TraceEntityPreviewProps {
  span: Span;
  maxLength?: number;
}

interface ParsedEntityData {
  input?: string;
  output?: string;
}

const TRUNCATE_LENGTH = 75;

function parseEntityData(span: Span): ParsedEntityData {
  const result: ParsedEntityData = {};

  try {
    const inputAttr = span?.attributes?.['traceloop.entity.input'];
    if (inputAttr && typeof inputAttr === 'string') {
      const parsed = JSON.parse(inputAttr);
      result.input = parsed?.inputs?.input;
    }
  } catch {
    // Ignore parsing errors for input
  }

  try {
    const outputAttr = span?.attributes?.['traceloop.entity.output'];
    if (outputAttr && typeof outputAttr === 'string') {
      const parsed = JSON.parse(outputAttr);
      result.output = parsed?.outputs?.output;
    }
  } catch {
    // Ignore parsing errors for output
  }

  return result;
}

function truncateText(text: string, maxLength: number): string {
  return text.length > maxLength ? `${text.slice(0, maxLength)}...` : text;
}

export function TraceEntityPreview({
  span,
  maxLength = TRUNCATE_LENGTH,
}: TraceEntityPreviewProps) {
  const { input, output } = useMemo(() => parseEntityData(span), [span]);

  if (!input && !output) {
    return null;
  }

  return (
    <Typography
      component="span"
      variant="caption"
      sx={{
        pt: 1,
      }}
    >
      {input && truncateText(input, maxLength)}
      {input && output && (
        <>
          &nbsp;
          <ArrowRight size={12} />
          &nbsp;
        </>
      )}
      {output && truncateText(output, maxLength)}
    </Typography>
  );
}


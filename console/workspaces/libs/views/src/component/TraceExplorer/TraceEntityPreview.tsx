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


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

import {
  Alert,
  Button,
  Card,
  CardContent,
  FormControlLabel,
  Stack,
  Switch,
  Typography,
} from "@wso2/oxygen-ui";
import { CodeBlock } from "@agent-management-platform/shared-component";
import type {
  ShadowProviderSpec,
  ShadowRunnerScanResponse,
} from "@agent-management-platform/types";
import { buildScanCommand, type ScanCommandInput, type ScanCommandOptions } from "../scanCommand";

/** The local time a scan token expires at, or "" for an unreadable date. */
export function expiryTime(expiresAt: string | undefined): string {
  const d = expiresAt ? new Date(expiresAt) : null;
  return d && !Number.isNaN(d.getTime()) ? d.toLocaleTimeString() : "";
}

/** The AI review switch and, under it, the LLM judge switch it enables. */
export function ScanOptions({
  aiReview,
  llmJudge,
  onChange,
  disabled,
}: ScanCommandOptions & {
  onChange: (next: ScanCommandOptions) => void;
  disabled?: boolean;
}) {
  return (
    <Stack spacing={1.5}>
      <FormControlLabel
        disabled={disabled}
        control={
          <Switch
            checked={aiReview}
            onChange={(e) =>
              onChange({ aiReview: e.target.checked, llmJudge: e.target.checked && llmJudge })
            }
          />
        }
        label={
          <Stack>
            <Typography variant="body2">AI review</Typography>
            <Typography variant="caption" color="text.secondary">
              Your LLM checks the matches the rules cannot prove. The command asks for your LLM API
              key.
            </Typography>
          </Stack>
        }
      />
      <FormControlLabel
        sx={{ pl: 4 }}
        disabled={disabled || !aiReview}
        control={
          <Switch
            checked={llmJudge}
            onChange={(e) => onChange({ aiReview, llmJudge: e.target.checked })}
          />
        }
        label={
          <Stack>
            <Typography variant="body2">LLM judge</Typography>
            <Typography variant="caption" color="text.secondary">
              A second, smaller model must agree before an agent is published. Needs AI review.
            </Typography>
          </Stack>
        }
      />
    </Stack>
  );
}

/**
 * The scan command of a GitHub platform, next to its form.
 * It follows the form until the platform is saved; then it shows the issued
 * command once (it holds a scan token, so it lives only in the caller's state).
 */
export function ScanCommandPanel({
  spec,
  draft,
  aiReview,
  llmJudge,
  onChange,
  issued,
  disabled,
  error,
  onRetry,
}: ScanCommandOptions & {
  spec: ShadowProviderSpec;
  /** The settings and endpoints the form would save. */
  draft: ScanCommandInput;
  onChange: (next: ScanCommandOptions) => void;
  issued: ShadowRunnerScanResponse | null;
  disabled?: boolean;
  /** The platform is saved but the command could not be issued. */
  error?: string | null;
  onRetry?: () => void;
}) {
  const expires = expiryTime(issued?.expiresAt);
  return (
    <Card variant="outlined">
      <CardContent>
        <Stack spacing={2}>
          <Typography variant="subtitle1">Scan command</Typography>
          <Typography variant="body2" color="text.secondary">
            The rules publish an agent only when they prove it. Choose what happens to the matches
            they cannot prove.
          </Typography>
          <ScanOptions
            aiReview={aiReview}
            llmJudge={llmJudge}
            onChange={onChange}
            disabled={disabled || !!issued}
          />
          <CodeBlock
            code={buildScanCommand(spec, draft, { aiReview, llmJudge }, issued ?? undefined)}
            language="bash"
            // Placeholders do not run: copy only the issued command.
            showCopyButton={!!issued}
            fieldId="shadow-scan-command"
            analyticsId="shadow-scan-command"
          />
          {issued ? (
            <Alert severity="info">
              Shown once: copy it now. It is valid for 1 hour{expires ? `, until ${expires}` : ""};
              after that, get a new one from the platform&apos;s Generate command.
            </Alert>
          ) : (
            <Typography variant="caption" color="text.secondary">
              Updates as you fill the form. The scan token, scan ID, Agent Manager address and
              scanner image are filled in when you save.
            </Typography>
          )}
          {error && (
            <Alert
              severity="error"
              action={
                onRetry && (
                  <Button color="inherit" size="small" onClick={onRetry} disabled={disabled}>
                    Try again
                  </Button>
                )
              }
            >
              {error}
            </Alert>
          )}
        </Stack>
      </CardContent>
    </Card>
  );
}

export default ScanCommandPanel;

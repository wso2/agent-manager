/**
 * Copyright (c) 2026, WSO2 LLC. (https://www.wso2.com).
 *
 * WSO2 LLC. licenses this file to you under the Apache License,
 * Version 2.0 (the "License"); you may not use this file except
 * in compliance with the License. You may obtain a copy of the License at
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

import { type ChangeEvent, type ReactNode, useId } from "react";
import { Alert, Box, ListingTable, Stack, Typography } from "@wso2/oxygen-ui";
import { TextInput } from "@agent-management-platform/views";
import type { EnvVarReferenceRow } from "../../utils/mcpEnvVarSpec";
import { ballerinaConfigurableFor } from "../../utils/ballerinaConfigurables";
import { BallerinaConfigurablesNotice } from "../BallerinaConfigurablesNotice/BallerinaConfigurablesNotice";

export type { EnvVarReferenceRow };

interface EnvironmentVariablesReferenceProps {
  /** Heading text; override when a page shows more than one of these blocks. */
  title?: string;
  /** Explanatory text rendered under the heading. */
  description: ReactNode;
  rows: readonly EnvVarReferenceRow[];
  /** Render in an info alert by default, or as plain drawer/page content. */
  variant?: "alert" | "plain";
  /**
   * Called when a variable name is edited. When omitted the names are shown
   * read-only.
   */
  onNameChange?: (key: string, value: string) => void;
  /** Optional content (e.g. code snippets) rendered below the table. */
  children?: ReactNode;
  /** Heading of the name column, e.g. "Configurable Name" for Ballerina configurables. */
  nameColumnLabel?: string;
}

/**
 * Shared "Environment Variables References" panel used by the agent
 * configuration detail pages (LLM providers, MCP servers). Lists the
 * environment variables injected into the agent deployment and, when
 * {@link onNameChange} is provided, lets the user rename them to match their
 * code.
 */
export function EnvironmentVariablesReference({
  title = "Environment Variables References",
  description,
  rows,
  variant = "alert",
  onNameChange,
  children,
  nameColumnLabel = "Variable Name",
}: EnvironmentVariablesReferenceProps) {
  const editable = Boolean(onNameChange);
  const noticeId = useId();
  // Any BAL_CONFIG_VAR_* variable binds a Ballerina configurable the program
  // must declare (it fails to start otherwise), so show the declarations.
  const configurableNames = rows
    .map((row) => ballerinaConfigurableFor(row.name))
    .filter((name): name is string => !!name);
  const content = (
    <>
      <Typography variant="body2" fontWeight={600} sx={{ mb: 1 }}>
        {title}
      </Typography>
      <Typography variant="body2" sx={{ mb: 2 }}>
        {description}
      </Typography>

      <Stack spacing={1}>
        <ListingTable.Container>
          <ListingTable density="compact">
            <ListingTable.Head>
              <ListingTable.Row>
                <ListingTable.Cell>
                  {nameColumnLabel}{" "}
                  {editable && (
                    <Typography component="span" variant="caption" color="text.secondary">
                      (editable)
                    </Typography>
                  )}
                </ListingTable.Cell>
                <ListingTable.Cell>Description</ListingTable.Cell>
              </ListingTable.Row>
            </ListingTable.Head>
            <ListingTable.Body>
              {rows.map((row) => (
                <ListingTable.Row key={row.key}>
                  <ListingTable.Cell>
                    <TextInput
                      value={row.name}
                      onChange={
                        onNameChange
                          ? (e: ChangeEvent<HTMLInputElement>) =>
                            onNameChange(row.key, e.target.value)
                          : undefined
                      }
                      copyable
                      copyTooltipText={`Copy ${row.name}`}
                      size="small"
                      slotProps={
                        editable ? undefined : { input: { readOnly: true } }
                      }
                    />
                  </ListingTable.Cell>
                  <ListingTable.Cell>
                    <Typography variant="body2" color="text.secondary">
                      {row.description}
                    </Typography>
                  </ListingTable.Cell>
                </ListingTable.Row>
              ))}
            </ListingTable.Body>
          </ListingTable>
        </ListingTable.Container>
        <BallerinaConfigurablesNotice
          configurableNames={configurableNames}
          fieldId={`env-var-reference-configurables-${noticeId}`}
        />
        {children}
      </Stack>
    </>
  );

  if (variant === "plain") {
    return <Box>{content}</Box>;
  }

  return (
    <Alert severity="info" sx={{ mt: 2 }}>
      {content}
    </Alert>
  );
}

/*
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
 * KIND, either express or implied. See the License for the
 * specific language governing permissions and limitations
 * under the License.
 */

import { useMemo } from "react";
import {
  Chip,
  Collapse,
  Form,
  IconButton,
  Stack,
  Typography,
} from "@wso2/oxygen-ui";
import { ChevronDown, ChevronUp } from "@wso2/oxygen-ui-icons-react";
import SwaggerSpecViewer from "../SwaggerSpecViewer/SwaggerSpecViewer";
import type { ResourceViewItem } from "./ResourceRow";

type ExpandableResourceRowProps = {
  resource: ResourceViewItem;
  isOpen: boolean;
  selected?: boolean;
  operationSpec?: Record<string, unknown> | null;
  showSummary?: boolean;
  noSummaryText?: string;
  onRowClick?: () => void;
  onToggleOpen: () => void;
};

export default function ExpandableResourceRow({
  resource,
  isOpen,
  selected,
  operationSpec,
  showSummary = true,
  noSummaryText = "No summary available.",
  onRowClick,
  onToggleOpen,
}: ExpandableResourceRowProps) {
  const fallbackOperationSpec = useMemo(() => {
    const path = String(resource.path ?? "").trim();
    const method = String(resource.method || "get").toLowerCase().trim();
    const supportedMethods = new Set([
      "get",
      "post",
      "put",
      "delete",
      "patch",
      "head",
      "options",
    ]);
    if (!path || !supportedMethods.has(method)) {
      return null;
    }

    const operation: Record<string, unknown> = {
      responses: {
        default: {
          description: "No response details available.",
        },
      },
    };

    if (resource.summary) {
      operation.summary = resource.summary;
    }

    return {
      openapi: "3.0.3",
      info: {
        title: `${resource.method?.toUpperCase() ?? "GET"} ${path}`,
        version: "1.0.0",
      },
      paths: {
        [path]: {
          [method]: operation,
        },
      },
    };
  }, [resource.method, resource.path, resource.summary]);

  const resolvedOperationSpec = operationSpec ?? fallbackOperationSpec;
  const method = String(resource.method ?? "GET").toUpperCase();
  const path = String(resource.path ?? "");
  const methodColor =
    method === "GET"
      ? "info"
      : method === "POST"
        ? "success"
        : method === "DELETE"
          ? "error"
          : "default";

  return (
    <Form.CardButton
      selected={selected}
      onClick={onRowClick}
      sx={{ width: "100%", justifyContent: "flex-start", textAlign: "left" }}
    >
      <Form.CardContent sx={{ width: "100%", minWidth: 0 }}>
        <Stack spacing={1}>
          <Stack
            direction="row"
            spacing={1}
            alignItems="center"
            sx={{ minWidth: 0 }}
          >
            <Chip
              label={method}
              size="small"
              variant="outlined"
              color={methodColor}
              sx={{ minWidth: 56, justifyContent: "center", flexShrink: 0 }}
            />
            <Stack spacing={0.25} sx={{ flex: 1, minWidth: 0 }}>
              <Typography
                variant="body2"
                sx={{
                  fontFamily: "monospace",
                  fontWeight: 500,
                  minWidth: 0,
                  maxWidth: "100%",
                  overflow: "hidden",
                  textOverflow: "ellipsis",
                  whiteSpace: "nowrap",
                }}
              >
                {path}
              </Typography>
              {showSummary && resource.summary && (
                <Typography
                  variant="caption"
                  color="text.secondary"
                  sx={{
                    minWidth: 0,
                    maxWidth: "100%",
                    overflow: "hidden",
                    textOverflow: "ellipsis",
                    whiteSpace: "nowrap",
                    display: "block",
                  }}
                >
                  {resource.summary}
                </Typography>
              )}
            </Stack>
            <IconButton
              size="small"
              aria-label={isOpen ? "Collapse details" : "Expand details"}
              onClick={(event) => {
                event.stopPropagation();
                onToggleOpen();
              }}
            >
              {isOpen ? <ChevronUp size={18} /> : <ChevronDown size={18} />}
            </IconButton>
          </Stack>

          <Collapse in={isOpen} timeout="auto" unmountOnExit>
       
              {resolvedOperationSpec ? (
                <SwaggerSpecViewer
                  spec={resolvedOperationSpec}
                  hideInfoSection
                  hideServers
                  hideAuthorizeButton
                  hideTagHeaders={false}
                  hideOperationHeader={false}
                  docExpansion="full"
                  defaultModelsExpandDepth={-1}
                  displayRequestDuration={false}
                />
              ) : (
                <Stack spacing={0.5} sx={{ minWidth: 0 }}>
                  <Typography
                    variant="body2"
                    sx={{
                      fontWeight: 600,
                      minWidth: 0,
                      maxWidth: "100%",
                      whiteSpace: "nowrap",
                      overflow: "hidden",
                      textOverflow: "ellipsis",
                    }}
                  >
                    {method} {path}
                  </Typography>
                  <Typography
                    variant="caption"
                    color="text.secondary"
                    sx={{
                      minWidth: 0,
                      maxWidth: "100%",
                      whiteSpace: "nowrap",
                      overflow: "hidden",
                      textOverflow: "ellipsis",
                      display: "block",
                    }}
                  >
                    {resource.summary || noSummaryText}
                  </Typography>
                </Stack>
              )}

          </Collapse>
        </Stack>
      </Form.CardContent>
    </Form.CardButton>
  );
}

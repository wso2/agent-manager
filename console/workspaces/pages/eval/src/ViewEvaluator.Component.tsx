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

import React from "react";
import { generatePath, Link, useParams } from "react-router-dom";
import { absoluteRouteMap } from "@agent-management-platform/types";
import { useGetCustomEvaluator } from "@agent-management-platform/api-client";
import { PageLayout } from "@agent-management-platform/views";
import {
  Box,
  Button,
  Chip,
  Skeleton,
  Stack,
  Typography,
  useColorScheme,
} from "@wso2/oxygen-ui";
import { Pencil, ArrowLeft } from "@wso2/oxygen-ui-icons-react";
import Editor, { type Monaco } from "@monaco-editor/react";

const VIEW_DARK_THEME = "view-dark";
const VIEW_LIGHT_THEME = "view-light";

function defineViewThemes(monaco: Monaco) {
  monaco.editor.defineTheme(VIEW_DARK_THEME, {
    base: "vs-dark",
    inherit: true,
    rules: [],
    colors: {},
  });
  monaco.editor.defineTheme(VIEW_LIGHT_THEME, {
    base: "vs",
    inherit: true,
    rules: [],
    colors: {},
  });
}

export const ViewEvaluatorComponent: React.FC = () => {
  const { agentId, orgId, projectId, evaluatorId } = useParams<{
    agentId: string;
    orgId: string;
    projectId: string;
    evaluatorId: string;
  }>();

  const { mode: colorSchemeMode } = useColorScheme();

  const { data: evaluator, isLoading } = useGetCustomEvaluator({
    orgName: orgId!,
    identifier: evaluatorId!,
  });

  const evaluatorsRouteMap = agentId
    ? absoluteRouteMap.children.org.children.projects.children.agents
        .children.evaluation.children.evaluators
    : absoluteRouteMap.children.org.children.projects.children.evaluators;

  const routeParams = agentId
    ? { orgId, projectId, agentId }
    : { orgId, projectId };

  const backHref = generatePath(evaluatorsRouteMap.path, routeParams);

  const editHref = generatePath(evaluatorsRouteMap.children.edit.path, {
    ...routeParams,
    evaluatorId,
  });

  if (isLoading) {
    return (
      <PageLayout title="Evaluator" disableIcon>
        <Stack spacing={2}>
          <Skeleton variant="rounded" height={40} />
          <Skeleton variant="rounded" height={200} />
        </Stack>
      </PageLayout>
    );
  }

  if (!evaluator) {
    return (
      <PageLayout title="Evaluator" disableIcon>
        <Typography>Evaluator not found.</Typography>
        <Button component={Link} to={backHref} startIcon={<ArrowLeft />}>
          Back to Evaluators
        </Button>
      </PageLayout>
    );
  }

  return (
    <PageLayout
      title={evaluator.displayName}
      disableIcon
      actions={
        <Stack direction="row" spacing={1}>
          <Button component={Link} to={backHref} variant="text" startIcon={<ArrowLeft />}>
            Back
          </Button>
          <Button component={Link} to={editHref} variant="contained" startIcon={<Pencil size={16} />}>
            Edit
          </Button>
        </Stack>
      }
    >
      <Stack spacing={3}>
        <Stack direction="row" spacing={1}>
          <Chip
            label={evaluator.type === "code" ? "Code" : "LLM Judge"}
            color={evaluator.type === "code" ? "info" : "warning"}
            size="small"
          />
          <Chip
            label={evaluator.level.charAt(0).toUpperCase() + evaluator.level.slice(1)}
            variant="outlined"
            color="primary"
            size="small"
          />
          {(evaluator.tags ?? []).map((tag) => (
            <Chip key={tag} label={tag} size="small" variant="outlined" />
          ))}
        </Stack>

        <Typography variant="body1">{evaluator.description}</Typography>

        <Box>
          <Typography variant="subtitle2" gutterBottom>
            {evaluator.type === "code" ? "Source Code" : "Prompt Template"}
          </Typography>
          <Box
            sx={{
              border: 1,
              borderColor: "divider",
              borderRadius: 1,
              overflow: "hidden",
            }}
          >
            <Editor
              height="400px"
              language={evaluator.type === "code" ? "python" : "plaintext"}
              theme={colorSchemeMode === "dark" ? VIEW_DARK_THEME : VIEW_LIGHT_THEME}
              value={evaluator.source}
              beforeMount={defineViewThemes}
              options={{
                readOnly: true,
                minimap: { enabled: false },
                scrollBeyondLastLine: false,
                fontSize: 14,
                lineNumbers: "on",
                automaticLayout: true,
              }}
            />
          </Box>
        </Box>

      </Stack>
    </PageLayout>
  );
};

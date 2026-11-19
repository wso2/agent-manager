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

import { useCallback } from "react";
import { Box, Chip, Divider, Stack, Typography, Form } from "@wso2/oxygen-ui";
import { generatePath, useNavigate, useParams } from "react-router-dom";
import {
  sampleAgents,
  useAgentWizardReturn,
} from "@agent-management-platform/shared-component";
import { ConsoleAction, useTrack } from "@agent-management-platform/api-client";
import { absoluteRouteMap } from "@agent-management-platform/types";

/**
 * Samples on the "Add a New Agent" page (a panel at the addNewAgentOptions
 * mount point). A sample seeds the Platform-Hosted Agent form directly through
 * the `sample` query param.
 */
export const NewAgentSampleCard = () => {
  const navigate = useNavigate();
  const { track } = useTrack();
  const { wizardState } = useAgentWizardReturn();
  const { orgId, projectId } = useParams<{ orgId: string; projectId?: string }>();

  const onSelectSample = useCallback((sampleId: string) => {
    track(ConsoleAction.WizardStepViewed, {
      wizard: "create-agent",
      step: "from-sample",
      step_index: 1,
    });
    const sourcePath = generatePath(
      absoluteRouteMap.children.org.children.projects.children.newAgent.children.create
        .children.source.path,
      { orgId: orgId ?? "", projectId: projectId ?? "default" },
    );
    navigate(`${sourcePath}?sample=${encodeURIComponent(sampleId)}`, { state: wizardState });
  }, [navigate, orgId, projectId, track, wizardState]);

  return (
    <Box>
      <Divider sx={{ mb: 3 }} />
      <Box display="flex" flexDirection="column" gap={1} mb={2}>
        <Typography variant="h3">Start Quickly with a Sample</Typography>
        <Typography variant="body2" color="text.primary">
          Choose a sample agent to configure and deploy as a Platform-Hosted Agent.
        </Typography>
      </Box>
      <Box display="flex" flexWrap="wrap" gap={2}>
        {sampleAgents.map((sample) => (
          <Form.CardButton
            key={sample.id}
            onClick={() => onSelectSample(sample.id)}
            sx={{
              flex: "1 1 220px",
              minWidth: 200,
              px: 2,
              py: 1.5,
              display: "flex",
              flexDirection: "column",
              alignItems: "flex-start",
              // Override ButtonBase's centered content: on equal-height cards,
              // pin title/description to the top and chips to the bottom so
              // both line up across cards regardless of description length.
              justifyContent: "space-between",
              gap: 0.5,
              textAlign: "left",
            }}
          >
            <Box display="flex" flexDirection="column" gap={1}>
              <Box display="flex" alignItems="center" gap={1}>
                <sample.icon size={20} strokeWidth={1.5} />
                <Typography variant="body1" fontWeight={600}>
                  {sample.title}
                </Typography>
              </Box>
              <Typography
                variant="body2"
                color="text.disabled"
                sx={{
                  width: "100%",
                  overflow: "hidden",
                }}
              >
                {sample.description}
              </Typography>
            </Box>
            <Stack direction="row" spacing={0.75} flexWrap="wrap" useFlexGap mt={1}>
              {sample.thirdPartyServices.map((service) => (
                <Chip
                  key={service}
                  label={service}
                  size="small"
                  sx={{ bgcolor: "action.selected", color: "text.primary" }}
                />
              ))}
            </Stack>
          </Form.CardButton>
        ))}
      </Box>
    </Box>
  );
};

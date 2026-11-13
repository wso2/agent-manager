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
import {
  useListLLMProviderConsumers,
  useOrgAgentDisplayNames,
} from "@agent-management-platform/api-client";
import {
  absoluteRouteMap,
  type LLMProviderConsumerItem,
} from "@agent-management-platform/types";
import {
  Alert,
  Box,
  Button,
  Chip,
  CircularProgress,
  ListingTable,
  Typography,
} from "@wso2/oxygen-ui";
import { generatePath, useNavigate } from "react-router-dom";
import { ArrowRight, Users } from "@wso2/oxygen-ui-icons-react";

interface LLMProviderConsumersTabProps {
  orgName: string | undefined;
  providerId: string | undefined;
}

function consumerLink(
  orgId: string,
  consumer: LLMProviderConsumerItem,
): string | undefined {
  if (consumer.consumerType === "agent") {
    return generatePath(
      absoluteRouteMap.children.org.children.projects.children.agents.children
        .configure.path,
      {
        orgId,
        projectId: consumer.projectName,
        agentId: consumer.consumerName,
      },
    );
  }
  // Monitors require envId which we don't have
  return undefined;
}

export const LLMProviderConsumersTab: React.FC<
  LLMProviderConsumersTabProps
> = ({ orgName, providerId }) => {
  const navigate = useNavigate();

  const { data, isLoading, error } = useListLLMProviderConsumers({
    orgName,
    providerId,
  });
  const { resolveAgentName, resolveProjectName } = useOrgAgentDisplayNames({ orgName });

  // Monitors aren't agents and will never resolve to anything here, so they
  // naturally fall back to their raw consumerName/projectName.
  const displayNameFor = (consumer: LLMProviderConsumerItem) =>
    resolveAgentName(consumer.projectName, consumer.consumerName);
  const projectDisplayNameFor = (consumer: LLMProviderConsumerItem) =>
    resolveProjectName(consumer.projectName, consumer.consumerName);

  if (isLoading) {
    return (
      <Box sx={{ display: "flex", justifyContent: "center", py: 6 }}>
        <CircularProgress />
      </Box>
    );
  }

  if (error) {
    return (
      <Alert severity="error" sx={{ mt: 2 }}>
        Failed to load consumers.
      </Alert>
    );
  }

  const consumers = data?.consumers ?? [];

  if (consumers.length === 0) {
    return (
      <ListingTable.Container>
        <ListingTable.EmptyState
          illustration={<Users size={64} />}
          title="No consumers yet"
          description="Agents and monitors that use an LLM proxy pointing to this provider will appear here."
        />
      </ListingTable.Container>
    );
  }

  return (
    <Box sx={{ pt: 2 }}>
      <ListingTable>
        <ListingTable.Head>
          <ListingTable.Row>
            <ListingTable.Cell>Name</ListingTable.Cell>
            <ListingTable.Cell>Type</ListingTable.Cell>
            <ListingTable.Cell>Project</ListingTable.Cell>
            <ListingTable.Cell width="140px" />
          </ListingTable.Row>
        </ListingTable.Head>
        <ListingTable.Body>
          {consumers.map((consumer, idx) => {
            const href = orgName
              ? consumerLink(orgName, consumer)
              : undefined;
            return (
              <ListingTable.Row
                key={idx}
                hover
                sx={{
                  "& .row-action": { visibility: "hidden" },
                  "&:hover .row-action, &:focus-within .row-action": { visibility: "visible" },
                }}
              >
                <ListingTable.Cell>
                  <Typography variant="body2" sx={{ fontWeight: 500 }}>
                    {displayNameFor(consumer)}
                  </Typography>
                </ListingTable.Cell>
                <ListingTable.Cell>
                  {consumer.consumerType === "agent" ? (
                    <Chip
                      label="Agent"
                      size="small"
                      variant="outlined"
                      color="primary"
                    />
                  ) : (
                    <Chip
                      label="Monitor"
                      size="small"
                      variant="outlined"
                      color="default"
                    />
                  )}
                </ListingTable.Cell>
                <ListingTable.Cell>
                  <Typography variant="body2" color="text.secondary">
                    {projectDisplayNameFor(consumer)}
                  </Typography>
                </ListingTable.Cell>
                <ListingTable.Cell align="right" padding="none" sx={{ pr: 1.5 }}>
                  {href && (
                    <Button
                      className="row-action"
                      variant="outlined"
                      size="small"
                      endIcon={<ArrowRight size={14} />}
                      onClick={() => navigate(href)}
                      aria-label={`Go to ${consumer.consumerType} ${displayNameFor(consumer)}`}
                      sx={{ minWidth: 120 }}
                    >
                      {`Go to ${consumer.consumerType}`}
                    </Button>
                  )}
                </ListingTable.Cell>
              </ListingTable.Row>
            );
          })}
        </ListingTable.Body>
      </ListingTable>
    </Box>
  );
};

export default LLMProviderConsumersTab;

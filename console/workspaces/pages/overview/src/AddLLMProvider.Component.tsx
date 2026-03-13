/**
 * Copyright (c) 2025, WSO2 LLC. (https://www.wso2.com).
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

import React, { useCallback, useState } from "react";
import { PageLayout } from "@agent-management-platform/views";
import {
  Box,
  Button,
  Form,
  FormControl,
  FormLabel,
  MenuItem,
  Select,
  Stack,
  Tab,
  Tabs,
  TextField,
  Typography,
} from "@wso2/oxygen-ui";
import { DoorClosedLocked } from "@wso2/oxygen-ui-icons-react";
import { generatePath, useParams } from "react-router-dom";
import { absoluteRouteMap } from "@agent-management-platform/types";
import {
  useListEnvironments,
  useListLLMProviders,
} from "@agent-management-platform/api-client";
import {
  GuardrailsSection,
  type GuardrailSelection,
} from "@agent-management-platform/llm-providers";

export const AddLLMProviderComponent: React.FC = () => {
  const { orgId, projectId, agentId } = useParams<{
    orgId: string;
    projectId: string;
    agentId: string;
  }>();

  const [name, setName] = useState("");
  const [description, setDescription] = useState("");
  const [selectedEnvIndex, setSelectedEnvIndex] = useState(0);
  const [providerByEnv, setProviderByEnv] = useState<
    Record<string, string | null>
  >({});
  const [guardrails, setGuardrails] = useState<GuardrailSelection[]>([]);

  const backHref =
    orgId && projectId && agentId
      ? generatePath(
          absoluteRouteMap.children.org.children.projects.children.agents
            .children.configure.path,
          { orgId, projectId, agentId },
        )
      : "#";

  const { data: environments = [] } = useListEnvironments({
    orgName: orgId,
  });
  const { data: providersData } = useListLLMProviders({
    orgName: orgId,
  });
  const providers = providersData?.providers ?? [];

  const handleAddGuardrail = useCallback((guardrail: GuardrailSelection) => {
    setGuardrails((prev) => {
      if (
        prev.some(
          (g) => g.name === guardrail.name && g.version === guardrail.version,
        )
      )
        return prev;
      return [...prev, guardrail];
    });
  }, []);

  const handleRemoveGuardrail = useCallback(
    (gName: string, gVersion: string) => {
      setGuardrails((prev) =>
        prev.filter((g) => !(g.name === gName && g.version === gVersion)),
      );
    },
    [],
  );

  const handleSave = useCallback(() => {
    // TODO: Implement save logic
  }, []);

  return (
    <PageLayout
      title="Add LLM Provider"
      backHref={backHref}
      disableIcon
      backLabel="Back to Configure"
    >
      <Stack spacing={3}>
        <Form.Section>
          <Form.Header>Basic Details</Form.Header>
          <Form.Stack spacing={2}>
            <FormControl fullWidth>
              <FormLabel>Name</FormLabel>
              <TextField
                fullWidth
                size="small"
                value={name}
                onChange={(e) => setName(e.target.value)}
                placeholder="e.g. OpenAI GPT5"
              />
            </FormControl>
            <FormControl fullWidth>
              <FormLabel>Description</FormLabel>
              <TextField
                fullWidth
                size="small"
                multiline
                minRows={3}
                value={description}
                onChange={(e) => setDescription(e.target.value)}
                placeholder="Describe the LLM provider"
              />
            </FormControl>
          </Form.Stack>
        </Form.Section>

        <Form.Section>
          <Form.Header>LLM Model provider</Form.Header>
          <Tabs
            value={selectedEnvIndex}
            onChange={(_, v: number) => setSelectedEnvIndex(v)}
            sx={{ mb: 2 }}
          >
            {environments.map((env, idx) => (
              <Tab
                key={env.name}
                label={env.displayName ?? env.name}
                value={idx}
              />
            ))}
          </Tabs>
          <FormControl fullWidth size="small">
            <FormLabel>Select Provider</FormLabel>
            <Select
              value={
                selectedEnvIndex < environments.length
                  ? providerByEnv[
                      environments[selectedEnvIndex]?.name ?? ""
                    ] ?? ""
                  : ""
              }
              onChange={(e) => {
                const envName = environments[selectedEnvIndex]?.name ?? "";
                setProviderByEnv((prev) => ({
                  ...prev,
                  [envName]: e.target.value || null,
                }));
              }}
              displayEmpty
              renderValue={(value) => {
                const provider = providers.find(
                  (p) => p.uuid === value || p.id === value,
                );
                return (
                  <Box
                    sx={{
                      display: "flex",
                      alignItems: "center",
                      gap: 1,
                    }}
                  >
                    <DoorClosedLocked size={20} />
                    <Typography variant="body2">
                      {provider?.name ?? "Select provider"}
                    </Typography>
                  </Box>
                );
              }}
            >
              <MenuItem value="">
                <em>Select provider</em>
              </MenuItem>
              {providers.map((p) => (
                <MenuItem key={p.uuid} value={p.uuid}>
                  {p.name}
                </MenuItem>
              ))}
            </Select>
          </FormControl>
          <GuardrailsSection
            guardrails={guardrails}
            onAddGuardrail={handleAddGuardrail}
            onRemoveGuardrail={handleRemoveGuardrail}
          />
        </Form.Section>

        {/* Actions */}
        <Box sx={{ display: "flex", gap: 1, justifyContent: "flex-end" }}>
          <Button component="a" href={backHref} variant="outlined">
            Cancel
          </Button>
          <Button variant="contained" onClick={handleSave}>
            Save
          </Button>
        </Box>
      </Stack>
    </PageLayout>
  );
};

export default AddLLMProviderComponent;

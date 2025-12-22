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

import { Chip, Divider, Stack, Tab, Tabs, Typography } from "@wso2/oxygen-ui";
import { Span, LLMData, AgentData, ToolDefinition, ToolData } from "@agent-management-platform/types";
import { BasicInfoSection } from "./spanDetails/BasicInfoSection";
import { AttributesSection } from "./spanDetails/AttributesSection";
import { useEffect, useState } from "react";
import { ToolsSection } from "./spanDetails/ToolsSection";
import { FadeIn } from "@agent-management-platform/views";
import { Overview } from "./spanDetails/Overview";

interface SpanDetailsPanelProps {
  span: Span | null;
}

// Helper function to extract tools from data based on span kind
function getTools(span: Span): ToolDefinition[] | string[] | undefined {
  const { kind, data } = span.ampAttributes || {};
  if (kind === 'llm' && data) {
    return (data as LLMData).tools;
  } else if (kind === 'agent' && data) {
    return (data as AgentData).tools;
  }
  return undefined;
}

// Helper function to check if span has overview content
function hasOverviewContent(span: Span): boolean {
  const { kind, data, input, output } = span.ampAttributes || {};
  
  // Check for input/output
  if (input || output) {
    return true;
  }
  
  // Check for agent name or system prompt
  if (kind === 'agent' && data) {
    const agentData = data as AgentData;
    if (agentData.name || agentData.systemPrompt) {
      return true;
    }
  }
  
  // Check for tool name
  if (kind === 'tool' && data) {
    const toolData = data as ToolData;
    if (toolData.name) {
      return true;
    }
  }
  
  return false;
}

export function SpanDetailsPanel({ span }: SpanDetailsPanelProps) {
  const [selectedTab, setSelectedTab] = useState<string>("overview");

  useEffect(() => {
    if (!span) return;
    
    // Check if there's overview content (input/output/name/systemPrompt)
    if (hasOverviewContent(span)) {
      setSelectedTab("overview");
    }
    // for tools
    else if (getTools(span)) {
      setSelectedTab("tools");
    }
    // for attributes
    else if (span?.attributes) {
      setSelectedTab("attributes");
    }
  }, [span]);

  if (!span) {
    return null;
  }

  const tools = getTools(span);
  const hasOverview = hasOverviewContent(span);

  return (
    <Stack spacing={2} sx={{ height: "100%" }}>
      <Stack spacing={2} px={1}>
        <Stack direction="row" spacing={1}>
          <Typography variant="h4">{span.name}</Typography>{" "}
          {span.ampAttributes?.kind && (
            <Chip
              size="small"
              variant="outlined"
              label={span.ampAttributes?.kind.toUpperCase()}
            />
          )}
        </Stack>
        <BasicInfoSection span={span} />
      </Stack>
      <Tabs
        variant="fullWidth"
        value={selectedTab}
        onChange={(_event, newValue) => setSelectedTab(newValue)}
      >
        <Tab label="Overview" value="overview" disabled={!hasOverview} />
        {tools && <Tab label="Tools" value="tools" />}
        {span?.attributes && <Tab label="Attributes" value="attributes" />}
      </Tabs>
      <Divider />
      <Stack spacing={2} px={1} sx={{ overflowY: "auto", flexGrow: 1 }}>
        {selectedTab === "attributes" && (
          <FadeIn>
            <AttributesSection attributes={span?.attributes} />
          </FadeIn>
        )}
        {selectedTab === "tools" && (
          <FadeIn>
            <ToolsSection tools={tools || []} />
          </FadeIn>
        )}
        {selectedTab === "overview" && (
          <FadeIn>
            <Overview ampAttributes={span.ampAttributes} />
          </FadeIn>
        )}
      </Stack>
    </Stack>
  );
}

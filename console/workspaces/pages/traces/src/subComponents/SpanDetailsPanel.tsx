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

import { Box, Chip, Stack, Tab, Tabs, Typography } from "@wso2/oxygen-ui";
import { Span } from "@agent-management-platform/types";
import { BasicInfoSection } from "./spanDetails/BasicInfoSection";
import { AttributesSection } from "./spanDetails/AttributesSection";
import { useEffect, useState } from "react";
import { ToolsSection } from "./spanDetails/ToolsSection";
import { FadeIn, SpanIcon } from "@agent-management-platform/views";
import { Overview } from "./spanDetails/Overview";

interface SpanDetailsPanelProps {
  span: Span | null;
}

export function SpanDetailsPanel({ span }: SpanDetailsPanelProps) {
  const [selectedTab, setSelectedTab] = useState<string>("overview");

  useEffect(() => {
    // for chat
    if (span?.ampAttributes?.input || span?.ampAttributes?.output) {
      setSelectedTab("overview");
    }
    // for tools
    else if (span?.ampAttributes?.tools) {
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



  return (
    <Stack spacing={2} sx={{ height: "100%" }}>
      <Stack spacing={2} px={1}>
        <Stack direction="row" spacing={1}>
          <Box color="primary.main">
            <SpanIcon span={span} />
          </Box>
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
        <Tab label="Overview" value="overview" />
        {span?.ampAttributes?.tools && <Tab label="Tools" value="tools" />}
        {span?.attributes && <Tab label="Attributes" value="attributes" />}
      </Tabs>
      <Stack spacing={2} px={1} sx={{ overflowY: "auto", flexGrow: 1 }}>
        {selectedTab === "attributes" && (
          <FadeIn>
            <AttributesSection attributes={span?.attributes} />
          </FadeIn>
        )}
        {selectedTab === "tools" && (
          <FadeIn>
            <ToolsSection tools={span?.ampAttributes?.tools || []} />
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

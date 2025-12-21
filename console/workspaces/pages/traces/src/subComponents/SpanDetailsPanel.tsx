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

import { Box, Divider } from "@wso2/oxygen-ui";
import { Span } from "@agent-management-platform/types";
import { BasicInfoSection } from "./spanDetails/BasicInfoSection";
import { TimingSection } from "./spanDetails/TimingSection";
import { StatusSection } from "./spanDetails/StatusSection";
import { AttributesSection } from "./spanDetails/AttributesSection";

interface SpanDetailsPanelProps {
  span: Span | null;
}

export function SpanDetailsPanel({ span }: SpanDetailsPanelProps) {
  if (!span) {
    return null;
  }

  return (
    <>
      <Box
        sx={{
          overflowY: "auto",
          gap: 1,
          overflowX: "visible",
          display: "flex",
          flexDirection: "column",
          height: "calc(100vh - 80px)",
        }}
      >
        <BasicInfoSection span={span} />
        <Divider />
        <TimingSection span={span} />
        <Divider />
        <StatusSection span={span} />
        {span.attributes && Object.keys(span.attributes).length > 0 && (
          <>
            <Divider />
            <AttributesSection attributes={span.attributes} />
          </>
        )}
      </Box>
    </>
  );
}

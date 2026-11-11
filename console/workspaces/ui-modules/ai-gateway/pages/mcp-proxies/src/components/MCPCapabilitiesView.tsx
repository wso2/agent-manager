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
 * KIND, either express or implied. See the License for the
 * specific language governing permissions and limitations
 * under the License.
 */

import { useState } from "react";
import {
  Accordion,
  AccordionDetails,
  AccordionSummary,
  Chip,
  Stack,
  Typography,
} from "@wso2/oxygen-ui";
import { ChevronDown } from "@wso2/oxygen-ui-icons-react";

type CapabilityItem = Record<string, unknown>;

export interface MCPCapabilitiesViewProps {
  tools?: CapabilityItem[];
  resources?: CapabilityItem[];
  prompts?: CapabilityItem[];
  sectionTitleVariant?: "subtitle1" | "h6";
  // Access-control status for a tool, e.g. from the ACL policy — Resources/Prompts
  // aren't gated by it here, so this only annotates the Tools section.
  getToolStatus?: (tool: CapabilityItem) => "allowed" | "denied" | undefined;
}

export function MCPCapabilitiesView({
  tools = [],
  resources = [],
  prompts = [],
  sectionTitleVariant = "subtitle1",
  getToolStatus,
}: MCPCapabilitiesViewProps) {
  return (
    <Stack>
      <CapabilitySection
        title="Tools"
        items={tools}
        titleVariant={sectionTitleVariant}
        getItemStatus={getToolStatus}
      />
      <CapabilitySection
        title="Resources"
        items={resources}
        titleVariant={sectionTitleVariant}
      />
      <CapabilitySection
        title="Prompts"
        items={prompts}
        titleVariant={sectionTitleVariant}
      />
    </Stack>
  );
}

function CapabilitySection({
  title,
  items,
  titleVariant,
  getItemStatus,
}: {
  title: string;
  items: CapabilityItem[];
  titleVariant: "subtitle1" | "h6";
  getItemStatus?: (item: CapabilityItem) => "allowed" | "denied" | undefined;
}) {
  const [open, setOpen] = useState(false);
  const [expandedItem, setExpandedItem] = useState("");

  return (
    <Accordion
      expanded={open}
      onChange={(_, expanded) => setOpen(expanded)}
      disableGutters
    >
      <AccordionSummary expandIcon={<ChevronDown size={18} />}>
        <Stack direction="row" spacing={1} alignItems="center">
          <Typography variant={titleVariant} fontWeight={600}>
            {title}
          </Typography>
          <Chip label={`Total: ${items.length}`} size="small" variant="outlined" />
        </Stack>
      </AccordionSummary>
      <AccordionDetails>
        <Stack>
          {items.length === 0 ? (
            <Typography variant="body2" color="text.secondary">
              No {title.toLowerCase()} found.
            </Typography>
          ) : null}
          {items.map((item, index) => {
            const name = getItemName(item) ?? `${title.slice(0, -1)} ${index + 1}`;
            const key = `${name}-${index}`;
            const status = getItemStatus?.(item);
            return (
              <Accordion
                key={key}
                expanded={expandedItem === key}
                onChange={(_, expanded) => setExpandedItem(expanded ? key : "")}
                disableGutters
                variant="outlined"
              >
                <AccordionSummary expandIcon={<ChevronDown size={16} />}>
                  <Stack direction="row" spacing={1} alignItems="center">
                    <Typography variant="subtitle2" fontWeight={600}>
                      {name}
                    </Typography>
                    {status && (
                      <Chip
                        label={status === "allowed" ? "Allowed" : "Denied"}
                        size="small"
                        variant="outlined"
                        color={status === "allowed" ? "success" : "default"}
                      />
                    )}
                  </Stack>
                </AccordionSummary>
                <AccordionDetails>
                  <MCPItemDetails item={item} />
                </AccordionDetails>
              </Accordion>
            );
          })}
        </Stack>
      </AccordionDetails>
    </Accordion>
  );
}

function MCPItemDetails({
  item,
}: {
  item: CapabilityItem;
}) {
  const value = getItemDescription(item) ?? getItemUri(item);
  if (value) {
    return (
      <Typography variant="body2" color="text.secondary">
        {value}
      </Typography>
    );
  }

  return (
    <Typography variant="body2" color="text.secondary">
      -
    </Typography>
  );
}

function getItemName(item: CapabilityItem | undefined): string | undefined {
  if (!item) return undefined;
  const value = item.name ?? item.title ?? item.uri ?? item.id;
  if (typeof value !== "string") return undefined;
  const normalizedValue = value.trim();
  return normalizedValue ? normalizedValue : undefined;
}

function getItemDescription(item: CapabilityItem | undefined): string | undefined {
  if (!item) return undefined;
  const value = item.description;
  return typeof value === "string" && value.trim() ? value : undefined;
}

function getItemUri(item: CapabilityItem | undefined): string | undefined {
  if (!item) return undefined;
  const value = item.uri;
  return typeof value === "string" && value.trim() ? value : undefined;
}

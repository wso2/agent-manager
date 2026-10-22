/**
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

import React from "react";
import { Box, Button, ButtonGroup } from "@wso2/oxygen-ui";
import { X as RemoveIcon } from "@wso2/oxygen-ui-icons-react";

export interface FilterChipProps {
  name: string;
  value: string;
  /** Names the whole chip, for its title and the remove button; defaults to "<name> <value>". */
  label?: string;
  onOpen: () => void;
  onRemove: () => void;
}

/** The filter's name and value, which open the panel, and an × that removes it. */
export const FilterChip: React.FC<FilterChipProps> = ({
  name,
  value,
  label = `${name} ${value}`,
  onOpen,
  onRemove,
}) => (
  <ButtonGroup
    size="small"
    variant="outlined"
    color="inherit"
    sx={{ maxWidth: 320, "& .MuiButton-root": { borderColor: "divider" } }}
  >
    <Button title={label} onClick={onOpen} sx={{ textTransform: "none", minWidth: 0 }}>
      <Box component="span" sx={{ color: "text.secondary", whiteSpace: "nowrap" }}>
        {name}
      </Box>{" "}
      <Box
        component="span"
        sx={{
          ml: 0.75,
          fontWeight: 600,
          overflow: "hidden",
          textOverflow: "ellipsis",
          whiteSpace: "nowrap",
        }}
      >
        {value}
      </Box>
    </Button>
    <Button aria-label={`Remove ${label}`} onClick={onRemove} sx={{ minWidth: 0, px: 0.75 }}>
      <RemoveIcon size={14} />
    </Button>
  </ButtonGroup>
);

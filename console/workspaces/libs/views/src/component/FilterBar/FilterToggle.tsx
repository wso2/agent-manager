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
import { ToggleButton, ToggleButtonGroup } from "@wso2/oxygen-ui";

export interface FilterToggleOption {
  value: string;
  label: string;
}

export interface FilterToggleProps {
  /** The group's accessible name. */
  label: string;
  /** The selected option; undefined selects All. */
  value?: string;
  options: FilterToggleOption[];
  onChange: (value?: string) => void;
}

const ALL = "all";

/** All or one option; applies straight away. */
export const FilterToggle: React.FC<FilterToggleProps> = ({ label, value, options, onChange }) => (
  <ToggleButtonGroup
    exclusive
    size="small"
    value={value ?? ALL}
    aria-label={label}
    onChange={(_, next: string | null) => {
      // null means the pressed button was clicked again.
      if (next === null) return;
      onChange(next === ALL ? undefined : next);
    }}
    sx={{ "& .MuiToggleButton-root": { px: 1.75, textTransform: "none" } }}
  >
    <ToggleButton value={ALL}>All</ToggleButton>
    {options.map((option) => (
      <ToggleButton key={option.value} value={option.value}>
        {option.label}
      </ToggleButton>
    ))}
  </ToggleButtonGroup>
);

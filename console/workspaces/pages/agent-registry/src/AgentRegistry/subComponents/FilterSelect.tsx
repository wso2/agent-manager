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

import { MenuItem, Select } from "@wso2/oxygen-ui";
import { ALL, type FilterOption } from "../agentRegistryFilters";

export interface FilterSelectProps {
  /** Shown when nothing is chosen, e.g. "All projects". */
  allLabel: string;
  /** Names the dropdown for screen readers, e.g. "Filter by project". */
  ariaLabel: string;
  value: string;
  options: FilterOption[];
  onChange: (value: string) => void;
}

/** One dropdown of the registry's filter bar. */
export function FilterSelect({ allLabel, ariaLabel, value, options, onChange }: FilterSelectProps) {
  return (
    <Select
      size="small"
      variant="outlined"
      value={value}
      onChange={(event) => onChange(event.target.value as string)}
      inputProps={{ "aria-label": ariaLabel }}
      disabled={options.length === 0}
      sx={{ minWidth: 160, maxWidth: 240 }}
    >
      <MenuItem value={ALL}>{allLabel}</MenuItem>
      {options.map((option) => (
        <MenuItem key={option.value} value={option.value}>
          {option.label}
        </MenuItem>
      ))}
    </Select>
  );
}

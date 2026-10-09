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

import React, { useState } from "react";
import { IconButton, InputAdornment, TextField } from "@wso2/oxygen-ui";
import { Search } from "@wso2/oxygen-ui-icons-react";

export interface TraceIdSearchProps {
  onSearch: (traceId: string) => void;
}

/** Opens one trace by ID on Enter or the button, trimmed and lowercased as the CLI does. */
export const TraceIdSearch: React.FC<TraceIdSearchProps> = ({ onSearch }) => {
  const [draft, setDraft] = useState("");
  /** Opens the trace unless the draft is blank. */
  const submit = () => {
    const traceId = draft.trim().toLowerCase();
    if (traceId) onSearch(traceId);
  };
  return (
    <TextField
      size="small"
      placeholder="Go to trace ID"
      value={draft}
      onChange={(e) => setDraft(e.target.value)}
      onKeyDown={(e) => {
        if (e.key === "Enter") submit();
      }}
      slotProps={{
        htmlInput: { "aria-label": "Go to trace ID" },
        input: {
          endAdornment: (
            <InputAdornment position="end">
              <IconButton size="small" onClick={submit} aria-label="Go to trace">
                <Search size={16} />
              </IconButton>
            </InputAdornment>
          ),
        },
      }}
      // At the row's right end; on its own line once the row wraps.
      sx={{ width: 300, maxWidth: "100%", ml: "auto" }}
    />
  );
};

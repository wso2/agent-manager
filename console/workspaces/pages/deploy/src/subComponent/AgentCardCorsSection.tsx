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

import type { AgentCardCorsConfig } from "@agent-management-platform/types";
import {
  Autocomplete,
  Box,
  Checkbox,
  Chip,
  Collapse,
  Form,
  FormControl,
  FormControlLabel,
  FormHelperText,
  FormLabel,
  Radio,
  RadioGroup,
  Switch,
  TextField,
} from "@wso2/oxygen-ui";

const CARD_CORS_METHODS = ["GET", "OPTIONS"];
const DEFAULT_CARD_CORS_HEADERS = ["Content-Type"];

export interface AgentCardCorsDraft {
  mode: "inherit" | "custom";
  enabled: boolean;
  allowAll: boolean;
  origins: string[];
  headers: string[];
  allowCredentials: boolean;
}

export function cardDraftFromConfig(cfg?: AgentCardCorsConfig): AgentCardCorsDraft {
  const origins = cfg?.allowOrigin ?? ["*"];
  return {
    mode: !cfg || cfg.inherit ? "inherit" : "custom",
    enabled: cfg?.enabled ?? true,
    allowAll: origins.length === 1 && origins[0] === "*",
    origins,
    headers: cfg?.allowHeaders ?? DEFAULT_CARD_CORS_HEADERS,
    allowCredentials: cfg?.allowCredentials ?? false,
  };
}

// A "*" typed into the origins list means the same as "Allow all origins", and a
// wildcard origin cannot be combined with credentials.
function hasWildcardOrigin(d: AgentCardCorsDraft): boolean {
  return d.allowAll || d.origins.includes("*");
}

export function cardDraftToPayload(d: AgentCardCorsDraft): AgentCardCorsConfig {
  if (d.mode === "inherit") return { inherit: true };
  const wildcard = hasWildcardOrigin(d);
  return {
    inherit: false,
    enabled: d.enabled,
    allowOrigin: wildcard ? ["*"] : d.origins,
    allowHeaders: d.headers,
    allowCredentials: wildcard ? false : d.allowCredentials,
  };
}

export function isCardDraftInvalid(d: AgentCardCorsDraft): boolean {
  return d.mode === "custom" && d.enabled && !d.allowAll && d.origins.length === 0;
}

interface ChipListFieldProps {
  label: string;
  placeholder: string;
  value: string[];
  onChange: (value: string[]) => void;
  disabled?: boolean;
}

const ChipListField = ({ label, placeholder, value, onChange, disabled }: ChipListFieldProps) => (
  <FormControl fullWidth>
    <FormLabel>{label}</FormLabel>
    <Autocomplete
      multiple
      freeSolo
      options={[]}
      value={value}
      disabled={disabled}
      onChange={(_, v) => onChange(v as string[])}
      renderTags={(vals, getTagProps) =>
        vals.map((opt, i) => (
          <Chip label={opt as string} size="small" {...getTagProps({ index: i })} key={opt as string} />
        ))
      }
      renderInput={(params) => <TextField {...params} size="small" placeholder={placeholder} />}
    />
  </FormControl>
);

interface AgentCardCorsSectionProps {
  draft: AgentCardCorsDraft;
  onChange: (draft: AgentCardCorsDraft) => void;
  disabled?: boolean;
}

// The public Agent Card route runs its own policies, outside the agent's CORS, so a browser
// client can only fetch the card cross-origin if this section allows it.
export const AgentCardCorsSection = ({ draft, onChange, disabled }: AgentCardCorsSectionProps) => {
  const update = (patch: Partial<AgentCardCorsDraft>) => onChange({ ...draft, ...patch });
  const originsMissing = isCardDraftInvalid(draft);

  return (
    <Form.Section>
      <Form.Header>Agent Card CORS</Form.Header>
      <Form.Subheader>
        Control which origins may fetch this agent&apos;s public Agent Card from a browser.
      </Form.Subheader>
      <Form.Stack spacing={1}>
        <RadioGroup
          value={draft.mode}
          onChange={(_, value) => update({ mode: value as AgentCardCorsDraft["mode"] })}
        >
          <FormControlLabel value="inherit" control={<Radio disabled={disabled} />} label="Inherit from agent CORS" />
          <FormControlLabel value="custom" control={<Radio disabled={disabled} />} label="Custom" />
        </RadioGroup>
        <Collapse in={draft.mode === "custom"}>
          <Form.Stack spacing={2} sx={{ mt: 1 }}>
            <FormControlLabel
              control={
                <Switch
                  checked={draft.enabled}
                  onChange={(_, enabled) => update({ enabled })}
                  disabled={disabled}
                />
              }
              label="Enable Agent Card CORS"
            />
            <Collapse in={draft.enabled}>
              <Form.Stack spacing={2}>
                <Box display="flex" gap={2} alignItems="center">
                  <FormControlLabel
                    control={
                      <Checkbox
                        checked={draft.allowAll}
                        disabled={disabled}
                        onChange={(_, allowAll) =>
                          update(allowAll
                            ? { allowAll, origins: ["*"], allowCredentials: false }
                            : { allowAll, origins: draft.origins.filter((o) => o !== "*") })
                        }
                      />
                    }
                    label="Allow all origins"
                  />
                  <FormControlLabel
                    control={
                      <Checkbox
                        checked={draft.allowCredentials}
                        disabled={disabled || hasWildcardOrigin(draft)}
                        onChange={(_, allowCredentials) => update({ allowCredentials })}
                      />
                    }
                    label="Allow credentials"
                  />
                </Box>
                {!draft.allowAll && (
                  <>
                    <ChipListField
                      label="Allowed origins"
                      placeholder="Add origin and press Enter"
                      value={draft.origins}
                      onChange={(origins) => update({ origins })}
                      disabled={disabled}
                    />
                    {originsMissing && (
                      <FormHelperText error>Add at least one origin.</FormHelperText>
                    )}
                  </>
                )}
                <ChipListField
                  label="Allowed headers"
                  placeholder="Add header and press Enter"
                  value={draft.headers}
                  onChange={(headers) => update({ headers })}
                  disabled={disabled}
                />
                <FormControl fullWidth>
                  <FormLabel>Allowed methods</FormLabel>
                  <TextField size="small" value={CARD_CORS_METHODS.join(", ")} disabled />
                </FormControl>
              </Form.Stack>
            </Collapse>
          </Form.Stack>
        </Collapse>
      </Form.Stack>
    </Form.Section>
  );
};

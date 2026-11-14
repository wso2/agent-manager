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
 * KIND, either express or implied.  See the License for the
 * specific language governing permissions and limitations
 * under the License.
 */

import { Grid, IconButton, InputAdornment, Stack, Tooltip, Typography } from "@wso2/oxygen-ui";
import { Copy } from "@wso2/oxygen-ui-icons-react";
import type { ThunderInstanceResponse } from "@agent-management-platform/types";
import { TextInput } from "@agent-management-platform/views";
import { InfoCard } from "@agent-management-platform/shared-component";

function EndpointField({
  label,
  value,
  onCopy,
}: {
  label: string;
  value: string;
  onCopy: (v: string, l: string) => void;
}) {
  return (
    <TextInput
      label={label}
      value={value}
      size="small"
      fullWidth
      slotProps={{
        input: {
          readOnly: true,
          sx: { fontFamily: "monospace", fontSize: "0.8125rem" },
          endAdornment: (
            <InputAdornment position="end">
              <Tooltip title={`Copy ${label}`}>
                <IconButton
                  size="small"
                  aria-label={`Copy ${label}`}
                  onClick={() => onCopy(value, label)}
                >
                  <Copy size={14} />
                </IconButton>
              </Tooltip>
            </InputAdornment>
          ),
        },
      }}
    />
  );
}

export type ThunderInstanceOverviewTabProps = {
  instance: ThunderInstanceResponse;
  onCopy: (value: string, label: string) => void;
};

export function ThunderInstanceOverviewTab({ instance, onCopy }: ThunderInstanceOverviewTabProps) {
  return (
    <Stack spacing={3}>
      {/* Infrastructure */}
      <Stack spacing={1.5}>
        <Typography variant="subtitle2" fontWeight={600}>
          Infrastructure
        </Typography>
        <Grid container spacing={2}>
          <Grid size={{ xs: 12, sm: 6 }}>
            <InfoCard label="Kubernetes Namespace" value={instance.namespace} monospace />
          </Grid>
          <Grid size={{ xs: 12, sm: 6 }}>
            <InfoCard label="Environment" value={instance.envName} monospace />
          </Grid>
        </Grid>
      </Stack>

      {/* OAuth2 Endpoints */}
      <Stack spacing={1.5}>
        <Typography variant="subtitle2" fontWeight={600}>
          OAuth2 Endpoints
        </Typography>
        <Stack spacing={2}>
          <EndpointField label="Token Endpoint" value={instance.tokenUrl} onCopy={onCopy} />
          <EndpointField label="JWKS Endpoint" value={instance.jwksUrl} onCopy={onCopy} />
          <EndpointField label="Issuer URL" value={instance.issuerUrl} onCopy={onCopy} />
        </Stack>
      </Stack>
    </Stack>
  );
}

export default ThunderInstanceOverviewTab;

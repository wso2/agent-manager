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

import { useState } from "react";
import {
  Alert,
  Button,
  Checkbox,
  Dialog,
  DialogActions,
  DialogContent,
  DialogTitle,
  FormControlLabel,
  Radio,
  RadioGroup,
  Stack,
  Typography,
} from "@wso2/oxygen-ui";
import { Radar } from "@wso2/oxygen-ui-icons-react";
import {
  isShadowScanActive,
  useShadowConnections,
  useShadowSummary,
  useStartShadowScan,
} from "@agent-management-platform/api-client";
import { providerLabel, SOURCE_HOSTS } from "../shadowUi";

/**
 * The page's primary action: scan every platform, or the platforms chosen
 * in the dialog. One platform can also be scanned from its row in Platforms.
 */
export function RunScanButton({ orgName }: { orgName: string | undefined }) {
  const [open, setOpen] = useState(false);
  const [scope, setScope] = useState<"all" | "selected">("all");
  const [selected, setSelected] = useState<string[]>([]);
  const summary = useShadowSummary(orgName);
  const connections = useShadowConnections(orgName);
  const start = useStartShadowScan(orgName);
  const active = isShadowScanActive(summary.data?.latestScan?.status);
  const all = connections.data?.connections ?? [];
  // Source repository hosts are not listed for choosing: their scan is
  // skipped until source code analysis is added.
  const choosable = all.filter((c) => !SOURCE_HOSTS.includes(c.provider));
  const none = all.length === 0;
  const canStart = scope === "all" || selected.length > 0;

  const toggle = (id: string) =>
    setSelected((cur) => (cur.includes(id) ? cur.filter((x) => x !== id) : [...cur, id]));
  const close = () => {
    start.reset();
    setOpen(false);
    setScope("all");
    setSelected([]);
  };

  return (
    <>
      <Button
        variant="contained"
        startIcon={<Radar size={16} />}
        disabled={active || none || start.isPending}
        onClick={() => setOpen(true)}
      >
        {active ? "Scan running…" : "Run scan"}
      </Button>
      <Dialog open={open} onClose={close} maxWidth="xs" fullWidth>
        <DialogTitle>Run a scan</DialogTitle>
        <DialogContent>
          <Stack spacing={1.5} sx={{ pt: 1 }}>
            <Typography variant="body2" color="text.secondary">
              Lists the agents of each platform with read-only calls, in the background.
            </Typography>
            <RadioGroup
              value={scope}
              onChange={(e) => setScope(e.target.value as "all" | "selected")}
            >
              <FormControlLabel
                value="all"
                control={<Radio />}
                label={`All platforms (${all.length})`}
              />
              <FormControlLabel value="selected" control={<Radio />} label="Choose platforms" />
            </RadioGroup>
            {scope === "selected" && (
              <Stack sx={{ pl: 4 }}>
                {choosable.map((c) => (
                  <FormControlLabel
                    key={c.id}
                    control={
                      <Checkbox checked={selected.includes(c.id)} onChange={() => toggle(c.id)} />
                    }
                    label={`${c.name} · ${providerLabel(c.provider)}`}
                  />
                ))}
              </Stack>
            )}
            {start.error ? (
              <Alert severity="error">
                {(start.error as { message?: string })?.message ??
                  "The scan could not be started. Try again."}
              </Alert>
            ) : null}
          </Stack>
        </DialogContent>
        <DialogActions sx={{ justifyContent: "flex-start", px: 3, pb: 2 }}>
          <Button
            variant="contained"
            disabled={start.isPending || !canStart}
            onClick={() =>
              start.mutate(scope === "all" ? {} : { connectionIds: selected }, {
                onSuccess: close,
              })
            }
          >
            Start scan
          </Button>
          <Button variant="outlined" onClick={close}>
            Cancel
          </Button>
        </DialogActions>
      </Dialog>
    </>
  );
}

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
  Box,
  Button,
  Dialog,
  DialogActions,
  DialogContent,
  DialogTitle,
  Stack,
  Typography,
} from "@wso2/oxygen-ui";
import { CheckCircle, XCircle } from "@wso2/oxygen-ui-icons-react";
import type { CapabilityStatus } from "../capabilities";

function CapabilityRow({ status }: { status: CapabilityStatus }) {
  const { capability, available, missing } = status;
  return (
    <Stack direction="row" spacing={1.5} alignItems="flex-start">
      <Box
        sx={{ mt: "2px", color: available ? "success.main" : "text.disabled", display: "flex" }}
        aria-hidden
      >
        {available ? <CheckCircle size={18} /> : <XCircle size={18} />}
      </Box>
      <Box sx={{ minWidth: 0 }}>
        <Typography variant="body2" fontWeight={500}>
          {capability.title}
        </Typography>
        <Typography variant="caption" color="text.secondary" component="p">
          {capability.description}
        </Typography>
        {!available && (
          <Typography variant="caption" color="text.primary" component="p" sx={{ mt: 0.5 }}>
            Needs: {missing.join(", ")}
          </Typography>
        )}
      </Box>
    </Stack>
  );
}

/**
 * "What does Agent Manager list?" (for a source repository host, "What does
 * the scan look for?"): how many capabilities the form's credentials and
 * options unlock, and a dialog that lists each one with what it still needs.
 */
export function AnalyzeSection({
  providerName,
  statuses,
  sourceHost = false,
}: {
  providerName: string;
  statuses: CapabilityStatus[];
  /** GitHub, Azure DevOps: what the scan command looks for in the repositories. */
  sourceHost?: boolean;
}) {
  const [open, setOpen] = useState(false);
  if (statuses.length === 0) return null;
  const available = statuses.filter((s) => s.available);
  const unavailable = statuses.filter((s) => !s.available);
  const heading = sourceHost ? "What does the scan look for?" : "What does Agent Manager list?";
  return (
    <Box sx={{ border: 1, borderColor: "divider", borderRadius: 1, p: 2 }}>
      <Stack
        direction={{ xs: "column", sm: "row" }}
        spacing={1.5}
        alignItems={{ sm: "center" }}
        justifyContent="space-between"
      >
        <Box>
          <Typography variant="subtitle2">{heading}</Typography>
          <Typography variant="body2" color="text.secondary">
            {available.length} of {statuses.length}{" "}
            {sourceHost ? "with these options." : "available with these credentials and options."}
          </Typography>
        </Box>
        <Button variant="outlined" size="small" onClick={() => setOpen(true)}>
          {sourceHost ? "See what it looks for" : "See what it lists"}
        </Button>
      </Stack>
      <Dialog open={open} onClose={() => setOpen(false)} maxWidth="sm" fullWidth>
        <DialogTitle>
          {sourceHost ? "What the scan looks for" : "What Agent Manager lists"}
        </DialogTitle>
        <DialogContent>
          <Stack spacing={2.5}>
            {sourceHost ? (
              <Typography variant="body2" color="text.secondary">
                {providerName}, with the options in this form. The scan runs on your machine: the
                code is read there and deleted, and only confirmed agents are sent to Agent Manager
                (name, repository, file, line, framework, matched symbol and commit).
              </Typography>
            ) : (
              <Typography variant="body2" color="text.secondary">
                {providerName}, with the credentials and options in this form. Every read is
                read-only; only the platform&apos;s agent listings are called, and instructions,
                prompts and secrets are never stored.
              </Typography>
            )}
            <Box>
              <Typography variant="overline" color="text.secondary">
                Available ({available.length})
              </Typography>
              <Stack spacing={1.5} sx={{ mt: 1 }}>
                {available.length === 0 && (
                  <Typography variant="body2" color="text.secondary">
                    Nothing yet. Add a credential to the form.
                  </Typography>
                )}
                {available.map((s) => (
                  <CapabilityRow key={s.capability.key} status={s} />
                ))}
              </Stack>
            </Box>
            {unavailable.length > 0 && (
              <Box>
                <Typography variant="overline" color="text.secondary">
                  Not available ({unavailable.length})
                </Typography>
                <Stack spacing={1.5} sx={{ mt: 1 }}>
                  {unavailable.map((s) => (
                    <CapabilityRow key={s.capability.key} status={s} />
                  ))}
                </Stack>
              </Box>
            )}
          </Stack>
        </DialogContent>
        <DialogActions sx={{ justifyContent: "flex-start", px: 3, pb: 2 }}>
          <Button variant="outlined" onClick={() => setOpen(false)}>
            Close
          </Button>
        </DialogActions>
      </Dialog>
    </Box>
  );
}

export default AnalyzeSection;

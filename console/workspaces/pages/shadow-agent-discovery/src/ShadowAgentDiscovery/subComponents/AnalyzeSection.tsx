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
import { CheckCircle, CircleSlash, ListChecks, XCircle } from "@wso2/oxygen-ui-icons-react";
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

/** One thing a platform's scan lists: its title and what it covers. */
function ListedRow({ title, description }: { title: string; description: string }) {
  return (
    <Stack direction="row" spacing={1.5} alignItems="flex-start">
      <Box sx={{ mt: "2px", color: "primary.main", display: "flex" }} aria-hidden>
        <ListChecks size={18} />
      </Box>
      <Box sx={{ minWidth: 0 }}>
        <Typography variant="body2" fontWeight={500}>
          {title}
        </Typography>
        <Typography variant="caption" color="text.secondary" component="p">
          {description}
        </Typography>
      </Box>
    </Stack>
  );
}

/**
 * "What does Agent Manager list?": a short description of what a scan of the
 * platform lists, and a dialog with each listing and the API it calls, ending
 * with what is never read or listed. For a source repository
 * host ("What does the scan look for?"), how many capabilities the form's
 * options unlock, each with what it still needs.
 */
export function AnalyzeSection({
  providerName,
  statuses,
  notListed = [],
  sourceHost = false,
}: {
  providerName: string;
  statuses: CapabilityStatus[];
  /** What a scan of the platform never reads or lists. */
  notListed?: string[];
  /** GitHub: what the scan command looks for in the repositories. */
  sourceHost?: boolean;
}) {
  const [open, setOpen] = useState(false);
  if (statuses.length === 0) return null;
  if (!sourceHost) {
    const titles = statuses.map((s) => s.capability.title);
    return (
      <Box sx={{ border: 1, borderColor: "divider", borderRadius: 1, p: 2 }}>
        <Stack
          direction={{ xs: "column", sm: "row" }}
          spacing={1.5}
          alignItems={{ sm: "center" }}
          justifyContent="space-between"
        >
          <Box>
            <Typography variant="subtitle2">What does Agent Manager list?</Typography>
            <Typography variant="body2" color="text.secondary">
              {titles.join(", ")}, with read-only calls.
            </Typography>
          </Box>
          <Button variant="outlined" size="small" onClick={() => setOpen(true)}>
            See what it lists
          </Button>
        </Stack>
        <Dialog open={open} onClose={() => setOpen(false)} maxWidth="sm" fullWidth>
          <DialogTitle>What Agent Manager lists</DialogTitle>
          <DialogContent>
            <Stack spacing={2.5}>
              <Typography variant="body2" color="text.secondary">
                A scan makes only these read-only calls to {providerName}.
              </Typography>
              <Stack spacing={1.75}>
                {statuses.map((s) => (
                  <ListedRow
                    key={s.capability.key}
                    title={s.capability.title}
                    description={s.capability.description}
                  />
                ))}
              </Stack>
              {notListed.length > 0 && (
                <Box sx={{ borderTop: 1, borderColor: "divider", pt: 2 }}>
                  <Typography variant="subtitle2">Not read or listed</Typography>
                  <Stack component="ul" spacing={0.75} sx={{ m: 0, mt: 1, p: 0, listStyle: "none" }}>
                    {notListed.map((item) => (
                      <Stack
                        component="li"
                        key={item}
                        direction="row"
                        spacing={1.5}
                        alignItems="flex-start"
                      >
                        <Box sx={{ mt: "2px", color: "text.disabled", display: "flex" }} aria-hidden>
                          <CircleSlash size={16} />
                        </Box>
                        <Typography variant="body2" color="text.secondary">
                          {item}
                        </Typography>
                      </Stack>
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
  const available = statuses.filter((s) => s.available);
  const unavailable = statuses.filter((s) => !s.available);
  return (
    <Box sx={{ border: 1, borderColor: "divider", borderRadius: 1, p: 2 }}>
      <Stack
        direction={{ xs: "column", sm: "row" }}
        spacing={1.5}
        alignItems={{ sm: "center" }}
        justifyContent="space-between"
      >
        <Box>
          <Typography variant="subtitle2">What does the scan look for?</Typography>
          <Typography variant="body2" color="text.secondary">
            {available.length} of {statuses.length} with these options.
          </Typography>
        </Box>
        <Button variant="outlined" size="small" onClick={() => setOpen(true)}>
          See what it looks for
        </Button>
      </Stack>
      <Dialog open={open} onClose={() => setOpen(false)} maxWidth="sm" fullWidth>
        <DialogTitle>What the scan looks for</DialogTitle>
        <DialogContent>
          <Stack spacing={2.5}>
            <Typography variant="body2" color="text.secondary">
              {providerName}, with the options in this form. The scan runs on your machine: the
              code is read there and deleted, and only confirmed agents are sent to Agent Manager
              (name, repository, file, line, framework, matched symbol and commit).
            </Typography>
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

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
  Divider,
  Stack,
  Typography,
} from "@wso2/oxygen-ui";
import { BookOpen } from "@wso2/oxygen-ui-icons-react";
import type { ShadowProviderSpec } from "@agent-management-platform/types";
import { DocLinks, Section } from "./PermissionsPanel";

/**
 * "How it works" for a source repository host (GitHub): a
 * button next to the platform name that opens the whole flow in a dialog, so
 * the form itself holds only the fields.
 */
export function SourceScanGuide({ spec }: { spec: ShadowProviderSpec }) {
  const [open, setOpen] = useState(false);
  return (
    <>
      <Button
        size="small"
        variant="outlined"
        startIcon={<BookOpen size={16} />}
        onClick={() => setOpen(true)}
      >
        How it works
      </Button>
      <Dialog open={open} onClose={() => setOpen(false)} maxWidth="sm" fullWidth scroll="paper">
        <DialogTitle>How {spec.displayName} scanning works</DialogTitle>
        <DialogContent dividers>
          <Stack spacing={2.5} divider={<Divider flexItem />}>
            <Box>
              <Typography variant="subtitle2">The steps</Typography>
              <Box component="ol" sx={{ pl: 2.5, my: 0.5 }}>
                <Typography component="li" variant="body2" color="text.secondary">
                  Save the organization and repositories here. No token is entered or stored.
                </Typography>
                <Typography component="li" variant="body2" color="text.secondary">
                  Choose AI-enhanced detection and Upload results directly next to the form and
                  save: the command is shown there. Later, get a new one from Generate command on
                  the Platforms tab.
                </Typography>
                <Typography component="li" variant="body2" color="text.secondary">
                  Run the command in a terminal. It asks for a read-only token when private
                  repositories are included, and for an LLM API key when AI-enhanced detection is
                  on. It scans on your machine and saves the agents it confirms in a results file
                  in the folder you ran it from.
                </Typography>
                <Typography component="li" variant="body2" color="text.secondary">
                  Upload the results file from Upload results on the Scans tab, whenever you are
                  ready. With Upload results directly on, the command offers to upload it for you
                  when the scan ends, with a token valid for one hour. Until results arrive, the
                  scan shows as Waiting for results.
                </Typography>
              </Box>
            </Box>
            {spec.permissions.map((p) => (
              <Section key={p.title} p={p} />
            ))}
            <Box>
              <Typography variant="subtitle2">What is never stored</Typography>
              <Typography variant="body2" color="text.secondary" sx={{ mt: 0.5 }}>
                Your Git token, your LLM key and your code. The code is read on your machine and
                deleted; only confirmed agents are saved in the results file and sent to Agent
                Manager: name, repository, file, line, framework, matched symbol and commit.
              </Typography>
            </Box>
            {spec.docs.length > 0 && (
              <Box>
                <Typography variant="subtitle2">Official documentation</Typography>
                <DocLinks spec={spec} />
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
    </>
  );
}

export default SourceScanGuide;

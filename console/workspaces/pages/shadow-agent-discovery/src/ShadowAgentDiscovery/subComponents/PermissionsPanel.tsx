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

import { useEffect, useMemo, useRef, useState } from "react";
import {
  Box,
  Button,
  Card,
  CardContent,
  Dialog,
  DialogActions,
  DialogContent,
  DialogTitle,
  Divider,
  IconButton,
  Stack,
  Tooltip,
  Typography,
} from "@wso2/oxygen-ui";
import { Copy, ExternalLink } from "@wso2/oxygen-ui-icons-react";
import type { ShadowPermissionSpec, ShadowProviderSpec } from "@agent-management-platform/types";

/** A policy document, pretty-printed, with a copy button. */
export function CopyBlock({ text }: { text: string }) {
  const pretty = useMemo(() => {
    try {
      return JSON.stringify(JSON.parse(text), null, 2);
    } catch {
      return text;
    }
  }, [text]);
  return (
    <Box
      sx={{
        position: "relative",
        border: 1,
        borderColor: "divider",
        borderRadius: 1,
        bgcolor: "action.hover",
      }}
    >
      <Box
        component="pre"
        sx={{ m: 0, p: 1.5, pr: 5, overflowX: "auto", fontSize: 12, maxHeight: 240 }}
      >
        {pretty}
      </Box>
      <Tooltip title="Copy">
        <IconButton
          size="small"
          aria-label="Copy policy"
          sx={{ position: "absolute", top: 4, right: 4 }}
          onClick={() => void navigator.clipboard?.writeText(pretty)}
        >
          <Copy size={16} />
        </IconButton>
      </Tooltip>
    </Box>
  );
}

function BulletList({ items }: { items: string[] }) {
  return (
    <Box component="ul" sx={{ pl: 2.5, my: 0.5 }}>
      {items.map((it) => (
        <Typography key={it} component="li" variant="body2" color="text.secondary" sx={{ mb: 0.5 }}>
          {it}
        </Typography>
      ))}
    </Box>
  );
}

function Section({ p }: { p: ShadowPermissionSpec }) {
  return (
    <Box>
      <Typography variant="subtitle2">{p.title}</Typography>
      {p.summary && (
        <Typography variant="body2" sx={{ mt: 0.5 }}>
          {p.summary}
        </Typography>
      )}
      {p.reads && p.reads.length > 0 && (
        <Box sx={{ mt: 1 }}>
          <Typography variant="overline" color="text.secondary">
            What Agent Manager reads
          </Typography>
          <BulletList items={p.reads} />
        </Box>
      )}
      {p.items.length > 0 && (
        <Box sx={{ mt: 1 }}>
          <Typography variant="overline" color="text.secondary">
            How to set it up
          </Typography>
          <BulletList items={p.items} />
        </Box>
      )}
      {p.policy && (
        <Box sx={{ mt: 1 }}>
          <Typography variant="overline" color="text.secondary">
            Least-privilege policy
          </Typography>
          <CopyBlock text={p.policy} />
        </Box>
      )}
      {p.note && (
        <Typography variant="caption" color="text.secondary" component="p" sx={{ mt: 1 }}>
          {p.note}
        </Typography>
      )}
    </Box>
  );
}

function linkLabel(url: string): string {
  try {
    const u = new URL(url);
    return `${u.hostname}${u.pathname === "/" ? "" : u.pathname}`;
  } catch {
    return url;
  }
}

/** Every section, what is never stored, and the official links. */
function Guide({ spec }: { spec: ShadowProviderSpec }) {
  return (
    <Stack spacing={2.5} divider={<Divider flexItem />}>
      {spec.permissions.map((p) => (
        <Section key={p.title} p={p} />
      ))}
      <Box>
        <Typography variant="subtitle2">What is never stored</Typography>
        <Typography variant="body2" color="text.secondary" sx={{ mt: 0.5 }}>
          Every call is read-only. Prompts, completions, tool inputs and outputs and agent
          instructions are never stored or logged; the credential is stored encrypted and is never
          shown again.
        </Typography>
      </Box>
      {spec.docs.length > 0 && (
        <Box>
          <Typography variant="subtitle2">Official documentation</Typography>
          <Stack spacing={0.5} sx={{ mt: 0.5 }}>
            {spec.docs.map((d) => (
              <Typography key={d} variant="body2">
                <a href={d} target="_blank" rel="noreferrer noopener">
                  {linkLabel(d)}
                </a>{" "}
                <ExternalLink size={12} />
              </Typography>
            ))}
          </Stack>
        </Box>
      )}
    </Stack>
  );
}

/**
 * What Agent Manager does with each credential and source of the provider,
 * shown next to the form. When it is taller than the screen allows, it is cut
 * and "Show more" opens all of it in a dialog.
 */
export function PermissionsPanel({ spec }: { spec: ShadowProviderSpec }) {
  const [open, setOpen] = useState(false);
  const [overflowing, setOverflowing] = useState(false);
  const boxRef = useRef<HTMLDivElement | null>(null);
  useEffect(() => {
    const el = boxRef.current;
    if (!el) return undefined;
    const measure = () => setOverflowing(el.scrollHeight > el.clientHeight + 1);
    measure();
    const ro = typeof ResizeObserver !== "undefined" ? new ResizeObserver(measure) : undefined;
    ro?.observe(el);
    window.addEventListener("resize", measure);
    return () => {
      ro?.disconnect();
      window.removeEventListener("resize", measure);
    };
  }, [spec]);
  const title = `How ${spec.displayName} is used`;
  return (
    <Card variant="outlined">
      <CardContent>
        <Typography variant="subtitle1" gutterBottom>
          {title}
        </Typography>
        <Box
          ref={boxRef}
          sx={{
            maxHeight: "calc(100vh - 260px)",
            minHeight: 240,
            overflow: "hidden",
            position: "relative",
          }}
        >
          <Guide spec={spec} />
          {overflowing && (
            <Box
              aria-hidden
              sx={{
                position: "absolute",
                left: 0,
                right: 0,
                bottom: 0,
                height: 64,
                background: (t) =>
                  `linear-gradient(to bottom, transparent, ${t.palette.background.paper})`,
              }}
            />
          )}
        </Box>
        {overflowing && (
          <Button variant="outlined" size="small" sx={{ mt: 1.5 }} onClick={() => setOpen(true)}>
            Show more
          </Button>
        )}
      </CardContent>
      <Dialog open={open} onClose={() => setOpen(false)} maxWidth="md" fullWidth scroll="paper">
        <DialogTitle>{title}</DialogTitle>
        <DialogContent dividers>
          <Guide spec={spec} />
        </DialogContent>
        <DialogActions sx={{ justifyContent: "flex-start", px: 3, pb: 2 }}>
          <Button variant="outlined" onClick={() => setOpen(false)}>
            Close
          </Button>
        </DialogActions>
      </Dialog>
    </Card>
  );
}

export default PermissionsPanel;

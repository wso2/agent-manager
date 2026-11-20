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

import { useId } from "react";
import {
  Box,
  Card,
  CardActionArea,
  CardContent,
  Chip,
  Grid,
  Stack,
  Typography,
} from "@wso2/oxygen-ui";
import { KeyRound, Laptop } from "@wso2/oxygen-ui-icons-react";
import type { ShadowProviderSpec } from "@agent-management-platform/types";
import { ProviderLogoTile } from "../providerLogos";
import { AGENT_PLATFORMS, CUSTOM_PLATFORM, DISCOVERS, selectCardSx } from "../shadowUi";

/** The AI platforms in display order, then any other platform the server offers. */
function byDisplayOrder(a: ShadowProviderSpec, b: ShadowProviderSpec): number {
  const rank = (p: ShadowProviderSpec) => {
    const i = AGENT_PLATFORMS.indexOf(p.provider);
    return i < 0 ? AGENT_PLATFORMS.length : i;
  };
  return rank(a) - rank(b);
}

function PlatformCard({
  spec,
  onChoose,
}: {
  spec: ShadowProviderSpec;
  onChoose: (spec: ShadowProviderSpec) => void;
}) {
  const descriptionId = useId();
  const source = !!spec.sourceHost;
  return (
    <Card
      variant="outlined"
      sx={{
        height: "100%",
        transition: (t) => t.transitions.create(["border-color", "box-shadow"]),
        "&:hover": { borderColor: "primary.main", boxShadow: 2 },
        "@media (prefers-reduced-motion: reduce)": { transition: "none" },
      }}
    >
      {/*
        A div with button semantics (ButtonBase handles Enter and Space), so the
        card can hold block content and the tag chip.
      */}
      <CardActionArea
        component="div"
        onClick={() => onChoose(spec)}
        aria-label={`Connect ${spec.displayName}`}
        aria-describedby={descriptionId}
        sx={{
          ...selectCardSx,
          "&.Mui-focusVisible": {
            outline: 2,
            outlineStyle: "solid",
            outlineColor: "primary.main",
            outlineOffset: -2,
          },
        }}
      >
        <CardContent
          sx={{ display: "flex", flexDirection: "column", gap: 1.5, flexGrow: 1, width: "100%" }}
        >
          <Stack direction="row" spacing={1.5} alignItems="center">
            <ProviderLogoTile provider={spec.provider} size={44} />
            <Typography variant="subtitle1" component="span">
              {spec.displayName}
            </Typography>
          </Stack>
          <Typography id={descriptionId} variant="body2" color="text.secondary">
            {DISCOVERS[spec.provider] ?? spec.description}
          </Typography>
          <Box sx={{ mt: "auto" }}>
            <Chip
              size="small"
              variant="outlined"
              icon={source ? <Laptop size={14} /> : <KeyRound size={14} />}
              label={source ? "Runs on your machine" : "Read-only key"}
            />
          </Box>
        </CardContent>
      </CardActionArea>
    </Card>
  );
}

/**
 * The first step of Add a platform: AI platforms, source code hosts and the
 * Bring your own inventory as selectable cards, each kind in its own group.
 */
export function PlatformPicker({
  providers,
  onChoose,
}: {
  providers: ShadowProviderSpec[];
  onChoose: (spec: ShadowProviderSpec) => void;
}) {
  const offered = [...providers].sort(byDisplayOrder);
  const groups = [
    {
      title: "AI platforms",
      description: "Lists the agents on the platform through its API, with a read-only key.",
      list: offered.filter((p) => !p.sourceHost && p.provider !== CUSTOM_PLATFORM),
    },
    {
      title: "Source code repositories",
      description:
        "Finds agents in your code with a command you run. No token is stored in Agent Manager.",
      list: offered.filter((p) => p.sourceHost),
    },
    {
      title: "Bring your own inventory",
      description:
        "Lists the agents your own inventory API returns, built to the Agent Manager contract.",
      list: offered.filter((p) => p.provider === CUSTOM_PLATFORM),
    },
  ].filter((g) => g.list.length > 0);

  return (
    <Stack spacing={4}>
      <Typography variant="body1">Choose where to discover agents.</Typography>
      {groups.map((g) => (
        <Stack key={g.title} spacing={1.5} component="section" aria-label={g.title}>
          <Box>
            <Typography variant="h6" component="h2">
              {g.title}
            </Typography>
            <Typography variant="body2" color="text.secondary">
              {g.description}
            </Typography>
          </Box>
          <Grid container spacing={2}>
            {g.list.map((p) => (
              <Grid key={p.provider} size={{ xs: 12, sm: 6, lg: 4 }}>
                <PlatformCard spec={p} onChoose={onChoose} />
              </Grid>
            ))}
          </Grid>
        </Stack>
      ))}
    </Stack>
  );
}

export default PlatformPicker;

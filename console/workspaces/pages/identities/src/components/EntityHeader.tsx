/**
 * Copyright (c) 2026, WSO2 LLC. (https://www.wso2.com).
 *
 * WSO2 LLC. licenses this file to you under the Apache License,
 * Version 2.0 (the "License"); you may not use this file except
 * in compliance with the License. You may obtain a copy of the License at
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

import React, { type ReactNode } from "react";
import { Avatar, Box, Stack, Typography } from "@wso2/oxygen-ui";

interface EntityHeaderProps {
  /** Fallback letter shown in the avatar when `name` is empty. */
  fallback: string;
  /** Primary title — entity name. */
  name: string;
  /** Optional secondary line — email, description, etc. */
  subtitle?: string;
  /** Entity ID shown as a monospace caption. */
  id: string;
  /** Optional badge rendered inline after the title (e.g. a "Read-only" Chip). */
  badge?: ReactNode;
}

const AVATAR_SX  = { width: 48, height: 48, fontSize: 20 } as const;
const BOX_SX     = { flex: 1, minWidth: 0 } as const;
const CAPTION_SX = { fontFamily: "monospace" } as const;

export const EntityHeader: React.FC<EntityHeaderProps> = ({
  fallback,
  name,
  subtitle,
  id,
  badge,
}) => (
  <Stack direction="row" alignItems="center" spacing={2}>
    <Avatar sx={AVATAR_SX}>
      {name.charAt(0).toUpperCase() || fallback}
    </Avatar>
    <Box sx={BOX_SX}>
      {badge ? (
        <Stack direction="row" alignItems="center" spacing={1}>
          <Typography variant="h6" noWrap>{name}</Typography>
          {badge}
        </Stack>
      ) : (
        <Typography variant="h6" noWrap>{name}</Typography>
      )}
      {subtitle && (
        <Typography variant="body2" color="text.secondary" noWrap>
          {subtitle}
        </Typography>
      )}
      <Typography variant="caption" color="text.disabled" sx={CAPTION_SX}>
        {id}
      </Typography>
    </Box>
  </Stack>
);

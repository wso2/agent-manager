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

import { Box, Button, Card, CardContent, Typography } from "@wso2/oxygen-ui";
import { Trash } from "@wso2/oxygen-ui-icons-react";

export interface DangerZoneCardProps {
  title: string;
  description: string;
  buttonLabel: string;
  pendingLabel: string;
  isPending: boolean;
  onClick: () => void;
}

/** Destructive-action card used by the "Unpublish Kind" and "Delete Version" sections. */
export function DangerZoneCard(
  { title, description, buttonLabel, pendingLabel, isPending, onClick }: DangerZoneCardProps,
) {
  return (
    <Card variant="outlined" sx={{ borderColor: "error.main" }}>
      <CardContent
        sx={{
          display: "flex",
          alignItems: "center",
          justifyContent: "space-between",
          gap: 2,
          "&:last-child": { pb: 2 },
        }}
      >
        <Box>
          <Typography variant="body2" fontWeight={500}>
            {title}
          </Typography>
          <Typography variant="body2" color="text.secondary">
            {description}
          </Typography>
        </Box>
        <Button
          variant="outlined"
          color="error"
          startIcon={<Trash />}
          disabled={isPending}
          onClick={onClick}
          sx={{ flexShrink: 0 }}
        >
          {isPending ? pendingLabel : buttonLabel}
        </Button>
      </CardContent>
    </Card>
  );
}

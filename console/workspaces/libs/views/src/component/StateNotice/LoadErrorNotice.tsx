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

import React from "react";
import { Button, Stack } from "@wso2/oxygen-ui";
import { AlertTriangle } from "@wso2/oxygen-ui-icons-react";
import { Link } from "react-router-dom";
import { StateNotice } from "./StateNotice";

interface LoadErrorNoticeProps {
  /** What failed to load, e.g. "project". */
  entityLabel: string;
  error?: unknown;
  onRetry?: () => void;
  backHref?: string;
  backLabel?: string;
}

/** Shown when a page's entity fails to load (deleted, no access, or a server error). */
export const LoadErrorNotice: React.FC<LoadErrorNoticeProps> = ({
  entityLabel,
  error,
  onRetry,
  backHref,
  backLabel = "Go back",
}) => (
  <StateNotice
    icon={AlertTriangle}
    tone="error"
    title={`Couldn't load this ${entityLabel}`}
    description={
      error instanceof Error && error.message
        ? error.message
        : `The ${entityLabel} may have been deleted, or you may not have access to it.`
    }
    actions={
      <Stack direction="row" spacing={1}>
        {onRetry && (
          <Button variant="contained" onClick={onRetry}>
            Try again
          </Button>
        )}
        {backHref && (
          <Button variant="outlined" component={Link} to={backHref}>
            {backLabel}
          </Button>
        )}
      </Stack>
    }
  />
);

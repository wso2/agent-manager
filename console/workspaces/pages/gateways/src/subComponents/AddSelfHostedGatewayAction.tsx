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

import React, { useState } from "react";
import { Button } from "@wso2/oxygen-ui";
import { Plus } from "@wso2/oxygen-ui-icons-react";
import { useListEnvironments } from "@agent-management-platform/api-client";
import type { GatewayResponse } from "@agent-management-platform/types";
import { AddSelfHostedGatewayDrawer } from "./AddSelfHostedGatewayDrawer";

const DEFAULT_DISPLAY_NAME = "AI Self-Hosted";
const DEFAULT_DESCRIPTION = "Default self-hosted gateway";

interface AddSelfHostedGatewayActionProps {
  orgId: string;
  /** Environment name to pre-fill; defaults to the organization's first. */
  presetEnvironment?: string;
  onCreated?: (gateway: GatewayResponse) => void;
  disabled?: boolean;
}

/**
 * "Add self-hosted gateway" button plus its drawer, for deploy flows. Callers
 * render it only when `globalConfig.selfHostedGatewaysEnabled === true`.
 */
export const AddSelfHostedGatewayAction: React.FC<
  AddSelfHostedGatewayActionProps
> = ({ orgId, presetEnvironment, onCreated, disabled }) => {
  const [open, setOpen] = useState(false);
  const { data: environments } = useListEnvironments({ orgName: orgId });

  return (
    <>
      <Button
        size="small"
        variant="text"
        startIcon={<Plus size={16} />}
        disabled={disabled}
        onClick={() => setOpen(true)}
      >
        Add self-hosted gateway
      </Button>
      <AddSelfHostedGatewayDrawer
        open={open}
        onClose={() => setOpen(false)}
        orgId={orgId}
        presetEnvironment={presetEnvironment ?? environments?.[0]?.name}
        presetDisplayName={DEFAULT_DISPLAY_NAME}
        presetDescription={DEFAULT_DESCRIPTION}
        onCreated={onCreated}
      />
    </>
  );
};

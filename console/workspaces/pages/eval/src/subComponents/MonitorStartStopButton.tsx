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

import { useCallback, type MouseEvent } from "react";
import { CircularProgress, IconButton, Tooltip } from "@wso2/oxygen-ui";
import { Calendar, Pause, Play } from "@wso2/oxygen-ui-icons-react";
import { useStartMonitor, useStopMonitor } from "@agent-management-platform/api-client";
import type { MonitorResponse } from "@agent-management-platform/types";

interface MonitorStartStopButtonProps {
    monitorName: string;
    monitorType: MonitorResponse["type"];
    monitorStatus?: MonitorResponse["status"] | string;
    orgId?: string;
}

export function MonitorStartStopButton({ monitorName, monitorType, monitorStatus, orgId }:
    MonitorStartStopButtonProps) {
    const { mutate: startMonitor, isPending: isStarting } = useStartMonitor();
    const { mutate: stopMonitor, isPending: isStopping } = useStopMonitor();

    const isPastMonitor = monitorType === "past";
    const isActive = monitorStatus === "Active";
    const isDisabled = isPastMonitor || isStarting || isStopping || !orgId;

    const tooltipTitle = isActive
        ? "Stop Monitor"
        : isPastMonitor
            ? "Past monitors cannot be started"
            : "Start Monitor";

    const handleClick = useCallback((event: MouseEvent<HTMLButtonElement>) => {
        event.stopPropagation();
        if (isDisabled || !orgId) {
            return;
        }

        if (isActive) {
            stopMonitor({
                monitorName,
                orgName: orgId,
            });
        } else {
            startMonitor({
                monitorName,
                orgName: orgId,
            });
        }
    }, [isActive, isDisabled, monitorName, orgId, startMonitor, stopMonitor]);

    if (isPastMonitor){
        return (
            <Tooltip title="Past monitors cannot be started">
                <span>
                    <IconButton disabled>
                        <Calendar size={16} />
                    </IconButton>
                </span>
            </Tooltip>
        );
    }
    if (isStarting || isStopping) {
        return (
            <Tooltip title={tooltipTitle}>
                <IconButton disabled={isDisabled} onClick={handleClick}>
                    <CircularProgress size={16} />
                </IconButton>
        </Tooltip>
        );
    }
    return (
        <Tooltip title={tooltipTitle}>
                <IconButton disabled={isDisabled} onClick={handleClick}>
                    {isActive ? <Pause size={16} /> : <Play size={16} />}
                </IconButton>
        </Tooltip>
    );
}

export default MonitorStartStopButton;

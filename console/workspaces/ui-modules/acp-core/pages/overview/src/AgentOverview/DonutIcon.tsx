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
import { useTheme } from "@wso2/oxygen-ui";

export type DonutColor = "success" | "warning" | "error" | "primary";

/** Maps a 0-100 score/percent to its donut color band, `null` scores rendering as neutral. */
export const getDonutColorForPercent = (percent: number | null): DonutColor => {
    if (percent === null) return "primary";
    if (percent >= 70) return "success";
    if (percent >= 40) return "warning";
    return "error";
};

interface DonutIconProps {
    percent: number;
    color: DonutColor;
    size?: number;
}

export const DonutIcon: React.FC<DonutIconProps> = ({ percent, color, size = 30 }) => {
    const theme = useTheme();
    const cx = size / 2;
    const r = size * 0.43;
    const sw = size * 0.117;
    const circumference = 2 * Math.PI * r;
    const safePercent = Number.isFinite(percent) ? Math.min(Math.max(percent, 0), 100) : 0;
    const offset = circumference * (1 - safePercent / 100);
    return (
        <svg width={size} height={size} viewBox={`0 0 ${size} ${size}`} style={{ display: "block" }}>
            <circle cx={cx} cy={cx} r={r} fill="none"
                stroke={theme.vars?.palette?.action?.selected} strokeWidth={sw} />
            <circle cx={cx} cy={cx} r={r} fill="none"
                stroke={theme.vars?.palette?.[color]?.main} strokeWidth={sw}
                strokeDasharray={circumference}
                strokeDashoffset={offset}
                strokeLinecap="round"
                transform={`rotate(-90 ${cx} ${cx})`}
            />
        </svg>
    );
};

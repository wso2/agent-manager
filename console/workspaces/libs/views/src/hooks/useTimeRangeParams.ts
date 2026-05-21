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

import { useCallback, useMemo } from "react";

type SetSearchParams = (next: URLSearchParams) => void;

export const useTimeRangeParams = (
    searchParams: URLSearchParams,
    setSearchParams: SetSearchParams,
) => {
    const [customStartTime, customEndTime, hasCustomRange] = useMemo((): [
        string | undefined,
        string | undefined,
        boolean,
    ] => {
        const startRaw = searchParams.get("startTime") || undefined;
        const endRaw = searchParams.get("endTime") || undefined;
        if (!startRaw || !endRaw) return [undefined, undefined, false];
        const startMs = Date.parse(startRaw);
        const endMs = Date.parse(endRaw);
        if (isNaN(startMs) || isNaN(endMs) || startMs > endMs) return [undefined, undefined, false];
        return [startRaw, endRaw, true];
    }, [searchParams]);

    const handleCustomRangeApply = useCallback(
        (startISO: string, endISO: string) => {
            const next = new URLSearchParams(searchParams);
            next.set("startTime", startISO);
            next.set("endTime", endISO);
            next.delete("timeRange");
            setSearchParams(next);
        },
        [searchParams, setSearchParams],
    );

    const handleCustomRangeClear = useCallback(() => {
        const next = new URLSearchParams(searchParams);
        next.delete("startTime");
        next.delete("endTime");
        setSearchParams(next);
    }, [searchParams, setSearchParams]);

    return { customStartTime, customEndTime, hasCustomRange, handleCustomRangeApply, handleCustomRangeClear };
};

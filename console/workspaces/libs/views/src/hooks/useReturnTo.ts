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

import { useLocation, type Location } from "react-router-dom";

export interface ReturnToState {
    from?: string;
}

/** Router state for links into a create/edit page so it can return to the current page. */
export const returnToState = (location: Location): ReturnToState => ({
    from: `${location.pathname}${location.search}`,
});

/**
 * Where a create/edit page's Back/Cancel should go: the page that opened it
 * (`location.state.from`), else `fallback` on a direct visit.
 */
export const useReturnTo = (fallback: string): string => {
    const location = useLocation();
    const from = (location.state as ReturnToState | null)?.from;
    // Only same-app absolute paths; ignore anything that could leave the console.
    return typeof from === "string" && from.startsWith("/") && !from.startsWith("//")
        ? from
        : fallback;
};

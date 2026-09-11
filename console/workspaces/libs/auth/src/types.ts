/**
 * Copyright (c) 2025, WSO2 LLC. (https://www.wso2.com).
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

import type { ReactNode } from "react";

interface AuthProviderProps {
    children: ReactNode;
}

export type { AuthProviderProps };

export type UserInfo = {
    allowedScopes?: string;
    /** Space-separated OAuth2 scopes from the decoded access token. */
    scope?: string;
    displayName?: string;
    familyName?: string;
    email?: string;
    givenName?: string;
    jti?: string;
    orgHandle?: string;
    orgId?: string;
    orgName?: string;
    sessionState?: string;
    sub?: string;
    username?: string;
}

/**
 * Attributes nested under a ThunderID `/users/me` response's `attributes` object.
 * Keys are the raw claim names ThunderID returns (snake_case), not the
 * camelCase `UserInfo` contract the rest of the app consumes.
 */
export type ThunderIDUserAttributes = {
    email?: string;
    family_name?: string;
    given_name?: string;
    username?: string;
    [key: string]: unknown;
}

/**
 * Shape of the profile object ThunderID's `useUser()` hook returns
 * (both `profile` and `flattenedProfile` - the SDK's "flattening" is
 * currently a no-op, so the raw `/users/me` response shape leaks through
 * with profile fields nested under `attributes`).
 */
export type ThunderIDUserProfile = {
    id?: string;
    ouId?: string;
    type?: string;
    attributes?: ThunderIDUserAttributes;
    isReadOnly?: boolean;
}

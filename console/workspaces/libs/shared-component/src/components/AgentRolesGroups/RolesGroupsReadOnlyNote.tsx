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

import { Alert } from "@wso2/oxygen-ui";

interface RolesGroupsReadOnlyNoteProps {
  /** Name of the project's first environment, where identity is managed. */
  homeEnvironment?: string;
}

/**
 * Explains why roles and groups cannot be edited in an environment that
 * receives its identity by promotion: they are managed in the first
 * environment and carried over when the identity is promoted.
 */
export const RolesGroupsReadOnlyNote: React.FC<RolesGroupsReadOnlyNoteProps> = ({
  homeEnvironment,
}) => {
  const where = homeEnvironment ? `the ${homeEnvironment} environment` : "the first environment";
  return (
    <Alert severity="info" sx={{ mb: 2 }}>
      Roles and groups are managed in {where} and carried to this environment when the identity is
      promoted, so they are read only here.
    </Alert>
  );
};

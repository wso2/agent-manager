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

import { Button } from "@wso2/oxygen-ui";
import { CornerLeftUp } from "@wso2/oxygen-ui-icons-react";
import { Link } from "react-router-dom";
import { PageLayout, StateNotice } from "@agent-management-platform/views";
import {
  entryRoutePath,
  type NavEntry,
  type NavLevel,
  type NavParams,
} from "../../../Layouts/OxygenLayout/navigation/navModel";

interface UpLevelNoticeProps {
  entry: NavEntry;
  from: NavLevel;
  to: NavLevel;
  params: NavParams;
}

/** Shown when a menu item that lives at a higher level is opened deeper down. */
export function UpLevelNotice({ entry, from, to, params }: UpLevelNoticeProps) {
  const target = to === "org" ? "organization" : to;
  const href = entryRoutePath(entry, to, params);
  return (
    <PageLayout title={entry.label} avatar={null}>
      <StateNotice
        icon={CornerLeftUp}
        title={`${entry.label} is managed at the ${target} level`}
        description={`You're viewing ${from === "agent" ? "an agent" : "a project"}. Go back to the ${target} to open ${entry.label}.`}
        actions={
          href && (
            <Button variant="contained" component={Link} to={href}>
              Go to {target}
            </Button>
          )
        }
      />
    </PageLayout>
  );
}

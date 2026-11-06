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

import { AuthProvider } from "@agent-management-platform/auth";
import { ClientProvider } from "@agent-management-platform/api-client";
import { OxygenUIThemeProvider, ChoreoLabsTheme } from "@wso2/oxygen-ui";
import { ConfirmationDialogProvider } from "@agent-management-platform/shared-component";
import {
  SnackBarProvider,
  ExternalModuleProvider,
  type ExternalModule,
} from "@agent-management-platform/views";
import type { FeatureFlags } from "@agent-management-platform/feature-management";
import type { UseFeatureAccess } from "../../types/featureAccess";
import { RuntimeConfigProvider } from "./RuntimeConfigProvider";
import { FeatureAccessBridge } from "./FeatureAccessBridge";

export const GlobalProviders = ({
  children,
  externalPageModules = [],
  featureFlags,
  useFeatureAccess,
}: {
  children: React.ReactNode;
  externalPageModules?: ExternalModule[];
  featureFlags?: Partial<FeatureFlags>;
  useFeatureAccess?: UseFeatureAccess;
}) => {
  return (
    <OxygenUIThemeProvider theme={ChoreoLabsTheme}>
      <SnackBarProvider>
        <AuthProvider>
          <ClientProvider>
            <RuntimeConfigProvider>
              <FeatureAccessBridge featureFlags={featureFlags} useAccess={useFeatureAccess}>
                <ConfirmationDialogProvider>
                  <ExternalModuleProvider externalPageModules={externalPageModules}>
                    {children}
                  </ExternalModuleProvider>
                </ConfirmationDialogProvider>
              </FeatureAccessBridge>
            </RuntimeConfigProvider>
          </ClientProvider>
        </AuthProvider>
      </SnackBarProvider>
    </OxygenUIThemeProvider>
  );
};

/*
 * Copyright (c) 2026, WSO2 LLC. (https://www.wso2.com).
 *
 * WSO2 LLC. licenses this file to you under the Apache License,
 * Version 2.0 (the "License"); you may not use this file except
 * in compliance with the License.
 * You may obtain a copy of the License at
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

import { useMemo } from "react";
import SwaggerUI from "swagger-ui-react";
import "swagger-ui-react/swagger-ui.css";
import "./SwaggerSpecViewer.css";

const SwaggerUIComponent =
  SwaggerUI as unknown as React.ComponentType<Record<string, unknown>>;

type SwaggerSelectorWrapper = () => () => unknown;

export type SwaggerSpecViewerProps = {
  spec: Record<string, unknown>;
  className?: string;
  docExpansion?: "list" | "full" | "none";
  defaultModelsExpandDepth?: number;
  displayRequestDuration?: boolean;
  hideInfoSection?: boolean;
  hideServers?: boolean;
  hideAuthorizeButton?: boolean;
  hideTagHeaders?: boolean;
  hideOperationHeader?: boolean;
};

export default function SwaggerSpecViewer({
  spec,
  className,
  docExpansion = "list",
  defaultModelsExpandDepth = -1,
  displayRequestDuration = true,
  hideInfoSection = false,
  hideServers = false,
  hideAuthorizeButton = false,
  hideTagHeaders = false,
  hideOperationHeader = false,
}: SwaggerSpecViewerProps) {
  const plugin = useMemo(() => {
    const wrapSelectors: Record<string, SwaggerSelectorWrapper> = {};
    const wrapComponents: Record<string, () => () => null> = {};

    if (hideServers) {
      wrapSelectors.servers = () => () => [];
    }

    if (hideAuthorizeButton) {
      wrapComponents.authorizeBtn = () => () => null;
      wrapSelectors.securityDefinitions = () => () => null;
      wrapSelectors.schemes = () => () => [];
    }

    const hasWrapSelectors = Object.keys(wrapSelectors).length > 0;
    const hasWrapComponents =
      hideInfoSection || Object.keys(wrapComponents).length > 0;

    if (!hasWrapSelectors && !hasWrapComponents) {
      return undefined;
    }

    const swaggerPlugin: Record<string, unknown> = {};

    if (hasWrapSelectors) {
      swaggerPlugin.statePlugins = {
        spec: {
          wrapSelectors,
        },
      };
    }

    if (hasWrapComponents) {
      swaggerPlugin.wrapComponents = {
        ...(hideInfoSection ? { info: () => () => null } : {}),
        ...wrapComponents,
      };
    }

    return swaggerPlugin;
  }, [hideAuthorizeButton, hideInfoSection, hideServers]);

  const plugins = plugin ? [plugin] : undefined;
  const containerClassName = [
    "swagger-spec-viewer",
    hideInfoSection ? "hide-info-section" : "",
    hideServers ? "hide-servers" : "",
    hideAuthorizeButton ? "hide-authorize" : "",
    hideTagHeaders ? "hide-tag-headers" : "",
    hideOperationHeader ? "hide-operation-header" : "",
    className ?? "",
  ]
    .filter(Boolean)
    .join(" ");

  return (
    <div className={containerClassName}>
      <SwaggerUIComponent
        spec={spec}
        docExpansion={docExpansion}
        defaultModelsExpandDepth={defaultModelsExpandDepth}
        displayRequestDuration={displayRequestDuration}
        supportedSubmitMethods={[]}
        plugins={plugins}
      />
    </div>
  );
}

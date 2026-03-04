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

import yaml from "js-yaml";
import { PolicyDefinition, ParameterSchema } from "./types";

interface RawPolicyYaml {
  name: string;
  version: string;
  description: string;
  parameters?: ParameterSchema;
  systemParameters?: ParameterSchema;
}

export function parsePolicyYaml(yamlContent: string): PolicyDefinition {
  const parsed = yaml.load(yamlContent) as RawPolicyYaml;

  if (!parsed.name) {
    throw new Error("Policy definition must have a name");
  }

  if (!parsed.version) {
    throw new Error(
      "Policy definition must have a version. eg: 1.0.0",
    );
  }

  const parameters: ParameterSchema = parsed.parameters || {
    type: "object",
    properties: {},
  };

  return {
    name: parsed.name,
    version: parsed.version,
    description: parsed.description || "",
    parameters,
    systemParameters: parsed.systemParameters,
  };
}

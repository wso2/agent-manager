/**
 * Copyright (c) 2026, WSO2 LLC. (https://www.wso2.com).
 *
 * WSO2 LLC. licenses this file to you under the Apache License,
 * Version 2.0 (the "License"); you may not use this file except
 * in compliance with the License. You may obtain a copy of the License at
 *
 * http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing,
 * software distributed under the License is distributed on an
 * "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
 * KIND, either express or implied. See the License for the
 * specific language governing permissions and limitations
 * under the License.
 */

import {
  asObject,
  asString,
  isObject,
  objectsIn,
  stringsIn,
  type JsonObject,
} from "./a2aCardReaders";
import { validateEndpointUrl } from "./url";

export interface ParsedA2AInterface {
  url: string;
  protocolBinding: string;
}

export interface ParsedA2ASkill {
  id: string;
  name: string;
  description: string;
  tags: string[];
  examples: string[];
  inputModes: string[];
  outputModes: string[];
}

export interface A2ACardCapabilities {
  streaming: boolean;
  pushNotifications: boolean;
  extendedAgentCard: boolean;
}

export interface ParsedA2ACard {
  name: string;
  description: string;
  version: string;
  providerOrganization: string;
  providerUrl: string;
  interfaces: ParsedA2AInterface[];
  skills: ParsedA2ASkill[];
  capabilities: A2ACardCapabilities;
  securitySchemes: string[];
}

function emptyCard(): ParsedA2ACard {
  return {
    name: "",
    description: "",
    version: "",
    providerOrganization: "",
    providerUrl: "",
    interfaces: [],
    skills: [],
    capabilities: {
      streaming: false,
      pushNotifications: false,
      extendedAgentCard: false,
    },
    securitySchemes: [],
  };
}

export const EMPTY_A2A_CARD: ParsedA2ACard = emptyCard();

function safeHttpUrl(value: unknown): string {
  return typeof value === "string" && validateEndpointUrl(value) === null
    ? value
    : "";
}

function parseInterface(item: JsonObject): ParsedA2AInterface {
  return {
    url: asString(item.url),
    protocolBinding: asString(item.protocolBinding),
  };
}

function parseSkill(item: JsonObject): ParsedA2ASkill {
  return {
    id: asString(item.id),
    name: asString(item.name),
    description: asString(item.description),
    tags: stringsIn(item.tags),
    examples: stringsIn(item.examples),
    inputModes: stringsIn(item.inputModes),
    outputModes: stringsIn(item.outputModes),
  };
}

function parseCapabilities(value: unknown): A2ACardCapabilities {
  const capabilities = asObject(value);
  return {
    streaming: capabilities.streaming === true,
    pushNotifications: capabilities.pushNotifications === true,
    extendedAgentCard: capabilities.extendedAgentCard === true,
  };
}

/** Reads an untrusted A2A agent card into a fully populated record; never throws. */
export function parseA2ACard(card: unknown): ParsedA2ACard {
  if (!isObject(card)) return emptyCard();
  const provider = asObject(card.provider);
  return {
    name: asString(card.name),
    description: asString(card.description),
    version: asString(card.version),
    providerOrganization: asString(provider.organization),
    providerUrl: safeHttpUrl(provider.url),
    interfaces: objectsIn(card.supportedInterfaces)
      .map(parseInterface)
      .filter((reachable) => reachable.url !== ""),
    skills: objectsIn(card.skills).map(parseSkill),
    capabilities: parseCapabilities(card.capabilities),
    securitySchemes: Object.keys(asObject(card.securitySchemes)),
  };
}

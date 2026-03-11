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

import { z } from "zod";

const VERSION_PATTERN = /^v\d+\.\d+$/;

const isValidUrlOrHostname = (v: string): boolean => {
  if (!v || typeof v !== "string") return false;
  const s = v.trim();
  if (!s) return false;
  // Wildcard subdomain (e.g. *.example.com) - URL constructor rejects these
  if (s.startsWith("*.")) {
    try {
      new URL(`https://${s.slice(2)}`);
      return true;
    } catch {
      return false;
    }
  }
  try {
    new URL(s.includes("://") ? s : `https://${s}`);
    return true;
  } catch {
    return false;
  }
};

export const addLLMProviderSchema = z.object({
  templateId: z
    .string()
    .trim()
    .min(1, "Select a provider template"),
  displayName: z
    .string()
    .trim()
    .min(1, "Display name is required")
    .min(2, "Display name must be at least 2 characters")
    .max(120, "Display name must be at most 120 characters"),
  version: z
    .string()
    .trim()
    .min(1, "Version is required")
    .regex(VERSION_PATTERN, "Version must match v<major>.<minor> (e.g., v1.0)"),
  description: z
    .string()
    .trim()
    .max(512, "Description cannot exceed 512 characters")
    .optional()
    .or(z.literal("")),
  context: z
    .string()
    .trim()
    .refine(
      (v) => !v || /^\/([a-zA-Z0-9_\-\/]*[^\/])?$/.test(v),
      "Context must start with / and have no trailing slash (e.g., /my-provider)"
    )
    .optional()
    .or(z.literal("")),
  upstreamUrl: z
    .string()
    .trim()
    .refine(
      (v) => !v || z.string().url().safeParse(v).success,
      "Enter a valid URL"
    ),
  apiKey: z
    .string()
    .trim()
    .optional()
    .or(z.literal("")),
  gatewayIds: z.array(z.string()).optional(),
});

export type AddLLMProviderFormValues = z.infer<typeof addLLMProviderSchema>;

const GATEWAY_NAME_PATTERN = /^[a-z0-9-]+$/;

export const addGatewaySchema = z.object({
  displayName: z
    .string()
    .trim()
    .min(3, "Display name is required, minimum 3 characters")
    .max(128, "Display name must be at most 128 characters"),
  name: z
    .string()
    .trim()
    .min(1, "Name is required")
    .max(64, "Name must be at most 64 characters")
    .regex(
      GATEWAY_NAME_PATTERN,
      "Name must be lowercase alphanumeric with hyphens only (e.g. ai-gateway-prod)"
    ),
  vhost: z
    .string()
    .trim()
    .min(3, "Virtual host is required, minimum 3 characters")
    .max(253, "Virtual host must be at most 253 characters")
    .refine(isValidUrlOrHostname, {
      message: "Enter a valid URL or hostname (e.g., api.example.com)",
    }),
  isCritical: z.boolean(),
  environmentIds: z
    .array(z.string())
    .min(1, "Select at least one environment"),
});

export type AddGatewayFormValues = z.infer<typeof addGatewaySchema>;

export const editGatewaySchema = z.object({
  displayName: z
    .string()
    .trim()
    .min(3, "Display name is required")
    .max(128, "Display name must be at most 128 characters"),
  isCritical: z.boolean(),
});

export type EditGatewayFormValues = z.infer<typeof editGatewaySchema>;

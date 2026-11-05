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
import { deriveResourceName } from "@agent-management-platform/shared-component";
import { isGatewayBaseUrl } from "../subComponents/selfHostedSetup";

// Exported so the drawers cap their input at exactly what these schemas accept.
export const GATEWAY_DISPLAY_NAME_MAX_LENGTH = 128;
const GATEWAY_DISPLAY_NAME_MIN_LENGTH = 3;

export const GATEWAY_NAME_MAX_LENGTH = 64;
const GATEWAY_NAME_MIN_LENGTH = 3;

export const deriveGatewayName = (displayName: string) =>
  deriveResourceName(displayName, GATEWAY_NAME_MAX_LENGTH);

const displayName = (minMessage: string) =>
  z
    .string()
    .trim()
    .min(GATEWAY_DISPLAY_NAME_MIN_LENGTH, minMessage)
    .max(GATEWAY_DISPLAY_NAME_MAX_LENGTH, `Display name must be at most ${GATEWAY_DISPLAY_NAME_MAX_LENGTH} characters`);

export const editGatewaySchema = z.object({
  displayName: displayName("Display name is required"),
  isCritical: z.boolean(),
});

export type EditGatewayFormValues = z.infer<typeof editGatewaySchema>;

export const addSelfHostedGatewaySchema = z.object({
  displayName: displayName(
    `Display name must be at least ${GATEWAY_DISPLAY_NAME_MIN_LENGTH} characters`,
  ),
  name: z
    .string()
    .trim()
    .min(GATEWAY_NAME_MIN_LENGTH, `Name must be at least ${GATEWAY_NAME_MIN_LENGTH} characters`)
    .max(GATEWAY_NAME_MAX_LENGTH, `Name must be at most ${GATEWAY_NAME_MAX_LENGTH} characters`)
    .regex(
      /^[a-z0-9]([a-z0-9-]*[a-z0-9])?$/,
      "Use lowercase letters, numbers and hyphens, not starting or ending with a hyphen",
    ),
  environment: z.string().min(1, "Select an environment"),
  publicUrl: z
    .string()
    .refine(
      isGatewayBaseUrl,
      "Enter the gateway's base URL, such as https://gateway.example.com (no path, query or fragment)",
    ),
  description: z.string(),
});

export type AddSelfHostedGatewayFormValues = z.infer<typeof addSelfHostedGatewaySchema>;

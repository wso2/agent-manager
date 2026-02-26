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

import { z } from "zod";
import type { MonitorEvaluator, MonitorLLMProviderConfig, MonitorType } from "@agent-management-platform/types";

const evaluatorSchema: z.ZodType<MonitorEvaluator> = z.object({
    identifier: z
        .string()
        .trim()
        .min(1, "Evaluator identifier is required"),
    displayName: z
        .string()
        .trim()
        .min(1, "Evaluator display name is required"),
    config: z.record(z.string(), z.unknown()).optional(),
});

const monitorLLMProviderConfigSchema: z.ZodType<MonitorLLMProviderConfig> = z.object({
    providerName: z.string().trim().min(1, "Provider name is required"),
    envVar: z.string().trim().min(1, "Environment variable is required"),
    value: z.string().trim().min(1, "Credential value is required"),
});

export const createMonitorSchema = z
    .object({
        displayName: z
            .string()
            .trim()
            .min(1, "Monitor title is required")
            .min(3, "Monitor title must be at least 3 characters")
            .max(120, "Monitor title must be at most 120 characters"),
        name: z
            .string()
            .trim()
            .min(1, "Monitor identifier is required")
            .regex(/^[a-z0-9-]+$/, "Use lowercase letters, numbers, and hyphens only")
            .min(3, "Identifier must be at least 3 characters")
            .max(60, "Identifier must be at most 60 characters"),
        description: z
            .string()
            .trim()
            .max(512, "Description cannot exceed 512 characters")
            .optional(),
        type: z.enum(["past", "future"]) as z.ZodType<MonitorType>,
        traceStart: z.date().nullable().optional(),
        traceEnd: z.date().nullable().optional(),
        intervalMinutes: z
            .union([z.number(), z.string()])
            .transform((val) => {
                if (val === "" || val === null || val === undefined) {
                    return undefined;
                }
                return typeof val === "string" ? Number(val) : val;
            })
            .refine((value) => value === undefined || (Number.isInteger(value) && value >= 5), {
                message: "Interval must be greater than 5 minutes",
            })
            .optional(),
        samplingRate: z
            .union([z.number(), z.string()])
            .transform((val) => {
                if (val === "" || val === null || val === undefined) {
                    return undefined;
                }
                return typeof val === "string" ? Number(val) : val;
            })
            .refine((value) => value === undefined ||
                (Number.isInteger(value) && value >= 0 && value <= 100), {
                message: "Sampling rate must be between 1 and 100",
            })
            .optional(),
        evaluators: z
            .array(evaluatorSchema)
            .min(1, "Select at least one evaluator"),
        llmProviderConfigs: z.array(monitorLLMProviderConfigSchema).optional(),
    })
    .superRefine((value, ctx) => {
        if (value.type !== "past") {
            return;
        }

        const now = new Date();
        if (!value.traceStart) {
            ctx.addIssue({
                code: z.ZodIssueCode.custom,
                path: ["traceStart"],
                message: "Start time is required for past traces",
            });
        }

        if (!value.traceEnd) {
            ctx.addIssue({
                code: z.ZodIssueCode.custom,
                path: ["traceEnd"],
                message: "End time is required for past traces",
            });
        }

        if (value.traceEnd && value.traceEnd > now) {
            ctx.addIssue({
                code: z.ZodIssueCode.custom,
                path: ["traceEnd"],
                message: "End time cannot be in the future",
            });
        }

        if (value.traceStart && value.traceEnd && value.traceStart >= value.traceEnd) {
            ctx.addIssue({
                code: z.ZodIssueCode.custom,
                path: ["traceStart"],
                message: "Start time must be earlier than the end time",
            });
            ctx.addIssue({
                code: z.ZodIssueCode.custom,
                path: ["traceEnd"],
                message: "End time must be after the start time",
            });
        }
    });

export type CreateMonitorFormValues = z.infer<typeof createMonitorSchema>;

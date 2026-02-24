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

import { useState, useCallback } from "react";
import { useFormValidation } from "@agent-management-platform/views";

/**
 * Wraps `useFormValidation` and tracks the last submitted validation errors,
 * so they can be surfaced in an alert near the submit button.
 *
 * @param schema - Zod schema for the form values.
 * @returns All fields from `useFormValidation`, plus `lastSubmittedValidationErrors`
 *          and `guardSubmit` (call instead of `validateForm` in your submit handler).
 */
// eslint-disable-next-line @typescript-eslint/no-explicit-any
export function useValidatedForm<T extends object>(schema: any) {
    const { errors, validateForm, setFieldError, validateField } =
        useFormValidation<T>(schema);

    const [lastSubmittedValidationErrors, setLastSubmittedValidationErrors] =
        useState<Record<string, string | undefined>>({});

    /**
     * Call this inside your submit handler instead of `validateForm`.
     * Returns `true` if the form is valid (and clears the error snapshot),
     * or `false` if invalid (and captures the current errors for display).
     */
    const guardSubmit = useCallback(
        (formData: T): boolean => {
            if (!validateForm(formData)) {
                setLastSubmittedValidationErrors(
                    { ...errors } as Record<string, string | undefined>);
                return false;
            }
            setLastSubmittedValidationErrors({});
            return true;
        },
        [validateForm, errors]
    );

    return {
        errors,
        setFieldError,
        validateField,
        lastSubmittedValidationErrors,
        guardSubmit,
    };
}

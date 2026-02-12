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

import { useState, useCallback } from 'react';
import { z } from 'zod';

// eslint-disable-next-line @typescript-eslint/no-explicit-any
export function useFormValidation<T extends Record<string, any>>(
  schema: z.ZodSchema<T>
) {
  const [errors, setErrors] = useState<Partial<Record<keyof T, string>>>({});

  const validateField = useCallback(
    (field: keyof T, value: unknown, fullData?: T) => {
      // If fullData is provided, validate using the full schema to catch refinements
      if (fullData) {
        const result = schema.safeParse(fullData);
        if (!result.success) {
          const fieldError = result.error.issues.find(
            issue => issue.path[0] === field
          );
          return fieldError?.message;
        }
        return undefined;
      }
      
      // Otherwise, validate just the field
      // eslint-disable-next-line @typescript-eslint/no-explicit-any
      const fieldSchema = (schema as any).shape[field];
      if (!fieldSchema) return undefined;
      
      const result = fieldSchema.safeParse(value);
      return !result.success ? result.error.issues[0]?.message : undefined;
    },
    [schema]
  );

  const validateForm = useCallback((data: T) => {
    const result = schema.safeParse(data);
    
    if (!result.success) {
      const fieldErrors: Partial<Record<keyof T, string>> = {};
      result.error.issues.forEach((issue) => {
        if (issue.path[0]) {
          fieldErrors[issue.path[0] as keyof T] = issue.message;
        }
      });
      setErrors(fieldErrors);
      return false;
    }
    
    setErrors({});
    return true;
  }, [schema]);

  const clearErrors = useCallback(() => {
    setErrors({});
  }, []);

  const clearFieldError = useCallback((field: keyof T) => {
    setErrors(prev => {
      const next = { ...prev };
      delete next[field];
      return next;
    });
  }, []);

  const setFieldError = useCallback((field: keyof T, error: string | undefined) => {
    setErrors(prev => {
      if (error === undefined) {
        const next = { ...prev };
        delete next[field];
        return next;
      }
      return { ...prev, [field]: error };
    });
  }, []);

  return {
    errors,
    setErrors,
    validateField,
    validateForm,
    clearErrors,
    clearFieldError,
    setFieldError,
  };
}

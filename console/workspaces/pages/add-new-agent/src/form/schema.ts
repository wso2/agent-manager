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

import * as yup from 'yup';
import type { InputInterfaceType } from '@agent-management-platform/types';

export type InterfaceType = InputInterfaceType;

export interface AddAgentFormValues {
  deploymentType?: 'new' | 'existing';
  name: string;
  displayName: string;
  description?: string;
  repositoryUrl?: string;
  branch?: string;
  appPath?: string;
  runCommand?: string;
  language?: string;
  languageVersion?: string;
  interfaceType: InterfaceType;
  port?: number;
  basePath?: string;
  openApiFileName?: string;
  openApiContent?: string;
  env: Array<{ key?: string; value?: string }>;
}

export const addAgentSchema = yup.object({
  deploymentType: yup.mixed<'new' | 'existing'>().oneOf(['new', 'existing']),
  displayName: yup
    .string()
    .trim()
    .required('Name is required')
    .min(3, 'Name must be at least 3 characters')
    .max(100, 'Name must be at most 100 characters'),
  name: yup
    .string()
    .trim()
    .required('Name is required')
    .matches(/^[a-z0-9-]+$/, 'Name must be lowercase letters, numbers, and hyphens only (no spaces)')
    .min(3, 'Name must be at least 3 characters')
    .max(50, 'Name must be at most 50 characters'),
  language: yup.string().trim().when('deploymentType', {
    is: 'new',
    then: (schema) => schema.required('Language is required'),
    otherwise: (schema) => schema.notRequired(),
  }),
  repositoryUrl: yup
    .string()
    .trim()
    .url('Must be a valid URL')
    .when('deploymentType', {
      is: 'new',
      then: (schema) => schema.required('Repository URL is required'),
      otherwise: (schema) => schema.notRequired(),
    }),
  description: yup.string().trim(),
  branch: yup.string().trim().when('deploymentType', {
    is: 'new',
    then: (schema) => schema.required('Branch is required'),
    otherwise: (schema) => schema.notRequired(),
  }),
  appPath: yup.string().trim(),
  runCommand: yup.string().trim().when('deploymentType', {
    is: 'new',
    then: (schema) => schema.required('Start Command is required'),
    otherwise: (schema) => schema.notRequired(),
  }),
  languageVersion: yup.string().trim(),
  interfaceType: yup.mixed<InterfaceType>().oneOf(['DEFAULT', 'CUSTOM']).required(),
  port: yup
    .number()
    .transform((v, o) => (o === '' || o === null ? undefined : v))
    .when('interfaceType', {
      is: 'CUSTOM',
      then: (schema) => schema
        .typeError('Port must be a number')
        .required('Port is required')
        .min(1, 'Port must be at least 1')
        .max(65535, 'Port must be at most 65535'),
      otherwise: (schema) => schema.notRequired(),
    }),
  basePath: yup.string().trim().when('interfaceType', {
    is: 'CUSTOM',
    then: (schema) => schema.required('Base path is required'),
    otherwise: (schema) => schema.notRequired(),
  }),
  openApiFileName: yup.string().when('interfaceType', {
    is: 'CUSTOM',
    then: (schema) => schema.required('OpenAPI spec file is required'),
    otherwise: (schema) => schema.notRequired(),
  }),
  openApiContent: yup.string().when('interfaceType', {
    is: 'CUSTOM',
    then: (schema) => schema.required('OpenAPI spec content is required'),
    otherwise: (schema) => schema.notRequired(),
  }),
  env: yup
    .array()
    .of(
      yup.object({
        key: yup.string().trim(),
        value: yup.string().trim(),
      })
    )
    .test('complete-pairs', 'Both key and value must be filled or both empty', (list) => {
      if (!list) return true;
      return list.every((e) => (e.key && e.value) || (!e.key && !e.value));
    })
    .test('unique-keys', 'Environment variable keys must be unique', (list) => {
      if (!list) return true;
      const keys = list.map((e) => e.key).filter(Boolean);
      return new Set(keys).size === keys.length;
    })
    .required(),
});



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

export interface AddProjectFormValues {
  name: string;
  displayName: string;
  description?: string;
  deploymentPipeline: string;
}

export const addProjectSchema = yup.object({
  displayName: yup
    .string()
    .trim()
    .required('Display name is required')
    .min(3, 'Display name must be at least 3 characters')
    .max(100, 'Display name must be at most 100 characters'),
  name: yup
    .string()
    .trim()
    .required('Name is required')
    .matches(/^[a-z0-9-]+$/, 'Name must be lowercase letters, numbers, and hyphens only (no spaces)')
    .min(3, 'Name must be at least 3 characters')
    .max(50, 'Name must be at most 50 characters'),
  description: yup.string().trim(),
  deploymentPipeline: yup
    .string()
    .trim()
    .required('Deployment pipeline is required'),
});


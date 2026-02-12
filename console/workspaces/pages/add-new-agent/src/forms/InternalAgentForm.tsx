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

import { Form, TextField } from "@wso2/oxygen-ui";
import { useEffect, useMemo, useCallback } from "react";
import { useParams } from "react-router-dom";
import { debounce } from "lodash";
import { useGenerateResourceName } from "@agent-management-platform/api-client";
import { InputInterface } from "../components/InputInterface";
import { EnvironmentVariable } from "../components/EnvironmentVariable";
import type { CreateAgentFormValues } from "../form/schema";

interface InternalAgentFormProps {
  formData: CreateAgentFormValues;
  setFormData: React.Dispatch<React.SetStateAction<CreateAgentFormValues>>;
  errors: Record<string, string | undefined>;
  setFieldError: (
    field: keyof CreateAgentFormValues,
    error: string | undefined
  ) => void;
  validateField: (
    field: keyof CreateAgentFormValues,
    value: unknown,
    fullData?: CreateAgentFormValues
  ) => string | undefined;
}

export const InternalAgentForm = ({
  formData,
  setFormData,
  errors,
  setFieldError,
  validateField,
}: InternalAgentFormProps) => {
  const { projectId } = useParams<{ orgId: string; projectId: string }>();
  
  const { mutate: generateName } = useGenerateResourceName({
    orgName: useParams<{ orgId: string }>().orgId,
  });

  const handleFieldChange = useCallback(
    (field: keyof CreateAgentFormValues, value: unknown) => {
      setFormData(prevData => {
        const newData = { ...prevData, [field]: value };
        const error = validateField(field, value, newData);
        setFieldError(field, error);
        return newData;
      });
    },
    [setFormData, validateField, setFieldError]
  );
  
  // Create debounced function for name generation
  const debouncedGenerateName = useMemo(
    () =>
      debounce((name: string) => {
        generateName({
          displayName: name,
          resourceType: 'agent',
          projectName: projectId,
        }, {
          onSuccess: (data: { name: string }) => {
            handleFieldChange("name", data.name);
          },
          onError: (error: unknown) => {
            // eslint-disable-next-line no-console
            console.error('Failed to generate name:', error);
          }
        });
      }, 500),
    [generateName, handleFieldChange, projectId]
  );

  // Cleanup debounce on unmount
  useEffect(() => {
    return () => {
      debouncedGenerateName.cancel();
    };
  }, [debouncedGenerateName]);

  // Auto-generate name from display name using API with debounce
  useEffect(() => {
    if (formData.displayName) {
      debouncedGenerateName(formData.displayName);
    } else if (!formData.displayName) {
      debouncedGenerateName.cancel();
      handleFieldChange("name", "");
    }
  }, [formData.displayName, handleFieldChange, debouncedGenerateName]);

  return (
    <Form.Stack spacing={3}>
      <Form.Section>
        <Form.Subheader>Agent Details</Form.Subheader>
        <Form.Stack spacing={2}>
          <Form.ElementWrapper label="Name" name="displayName">
            <TextField
              id="displayName"
              placeholder="e.g., Customer Support Agent"
              value={formData.displayName}
              onChange={(e) => handleFieldChange('displayName', e.target.value)}
              error={!!errors.displayName}
              helperText={
                errors.displayName ||
                "A name for your agent"
              }
              fullWidth
            />
          </Form.ElementWrapper>
          <Form.ElementWrapper label="Description (optional)" name="description">
            <TextField
              id="description"
              placeholder="Short description of what this agent does"
              multiline
              minRows={2}
              maxRows={6}
              value={formData.description || ''}
              onChange={(e) => handleFieldChange('description', e.target.value)}
              error={!!errors.description}
              helperText={errors.description}
              fullWidth
            />
          </Form.ElementWrapper>
        </Form.Stack>
      </Form.Section>

      <Form.Section>
        <Form.Subheader>Repository Details</Form.Subheader>
        <Form.Stack spacing={2}>
          <Form.ElementWrapper label="GitHub Repository" name="repositoryUrl">
            <TextField
              id="repositoryUrl"
              placeholder="https://github.com/username/repo"
              value={formData.repositoryUrl}
              onChange={(e) => handleFieldChange('repositoryUrl', e.target.value)}
              error={!!errors.repositoryUrl}
              helperText={errors.repositoryUrl}
              fullWidth
            />
          </Form.ElementWrapper>
          <Form.Stack direction="row" spacing={2}>
            <Form.ElementWrapper label="Branch" name="branch">
              <TextField
                id="branch"
                placeholder="main"
                value={formData.branch}
                onChange={(e) => handleFieldChange('branch', e.target.value)}
                error={!!errors.branch}
                helperText={errors.branch}
                fullWidth
              />
            </Form.ElementWrapper>
            <Form.ElementWrapper label="Project Path" name="appPath">
              <TextField
                id="appPath"
                placeholder="my-agent"
                value={formData.appPath}
                onChange={(e) => handleFieldChange('appPath', e.target.value)}
                error={!!errors.appPath}
                helperText={errors.appPath}
                fullWidth
              />
            </Form.ElementWrapper>
          </Form.Stack>
        </Form.Stack>
      </Form.Section>

      <Form.Section>
        <Form.Subheader>Build Details</Form.Subheader>
        <Form.Stack spacing={2}>
          <Form.Stack direction="row" spacing={2}>
            <Form.ElementWrapper label="Language" name="language">
              <TextField
                id="language"
                placeholder="python"
                disabled
                value={formData.language}
                onChange={(e) => handleFieldChange('language', e.target.value)}
                error={!!errors.language}
                helperText={
                  errors.language ||
                  "e.g., python, nodejs, go"
                }
                fullWidth
              />
            </Form.ElementWrapper>
            <Form.ElementWrapper label="Language Version" name="languageVersion">
              <TextField
                id="languageVersion"
                placeholder="3.11"
                value={formData.languageVersion || ''}
                onChange={(e) => handleFieldChange('languageVersion', e.target.value)}
                error={!!errors.languageVersion}
                helperText={
                  errors.languageVersion ||
                  "e.g., 3.11, 20, 1.21"
                }
                fullWidth
              />
            </Form.ElementWrapper>
          </Form.Stack>
          <Form.ElementWrapper label="Start Command" name="runCommand">
            <TextField
              id="runCommand"
              placeholder="python main.py"
              value={formData.runCommand}
              onChange={(e) => handleFieldChange('runCommand', e.target.value)}
              error={!!errors.runCommand}
              helperText={
                errors.runCommand ||
                "Dependencies auto-install from package.json, requirements.txt, or pyproject.toml"
              }
              fullWidth
            />
          </Form.ElementWrapper>
        </Form.Stack>
      </Form.Section>

      <InputInterface
        formData={formData}
        setFormData={setFormData}
        errors={errors}
        setFieldError={setFieldError}
        validateField={validateField}
      />
      <EnvironmentVariable
        formData={formData}
        setFormData={setFormData}
      />
    </Form.Stack>
  );
};

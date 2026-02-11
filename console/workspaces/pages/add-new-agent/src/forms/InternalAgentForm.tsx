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
import { useFormContext, useWatch, Controller } from "react-hook-form";
import { useEffect, useMemo } from "react";
import { useParams } from "react-router-dom";
import { debounce } from "lodash";
import { useGenerateResourceName } from "@agent-management-platform/api-client";
import { InputInterface } from "../components/InputInterface";
import { EnvironmentVariable } from "../components/EnvironmentVariable";

export const InternalAgentForm = () => {
  const {
    control,
    formState: { errors },
    setValue,
  } = useFormContext();
  const { orgId, projectId } = useParams<{ orgId: string; projectId: string }>();
  const displayName = useWatch({ control, name: "displayName" });
  
  const { mutate: generateName } = useGenerateResourceName({
    orgName: orgId,
  });

  // Create debounced function for name generation
  const debouncedGenerateName = useMemo(
    () =>
      debounce((name: string) => {
        generateName({
          displayName: name,
          resourceType: 'agent',
          projectName: projectId,
        }, {
          onSuccess: (data) => {
            setValue("name", data.name, {
              shouldValidate: true,
              shouldDirty: true,
              shouldTouch: true,
            });
          },
          onError: (error) => {
            // eslint-disable-next-line no-console
            console.error('Failed to generate name:', error);
          }
        });
      }, 500),
    [generateName, setValue, projectId, orgId]
  );

  // Cleanup debounce on unmount
  useEffect(() => {
    return () => {
      debouncedGenerateName.cancel();
    };
  }, [debouncedGenerateName]);

  // Auto-generate name from display name using API with debounce
  useEffect(() => {
    if (displayName) {
      debouncedGenerateName(displayName);
    } else if (!displayName) {
      debouncedGenerateName.cancel();
      setValue("name", "", {
        shouldValidate: true,
        shouldDirty: true,
        shouldTouch: true,
      });
    }
  }, [displayName, setValue, debouncedGenerateName]);

  return (
    <Form.Stack spacing={3}>
      <Form.Section>
        <Form.Subheader>Agent Details</Form.Subheader>
        <Form.Stack spacing={2}>
          <Controller
            name="displayName"
            control={control}
            render={({ field }) => (
              <Form.ElementWrapper label="Name" name="displayName">
                <TextField
                  {...field}
                  id="displayName"
                  placeholder="e.g., Customer Support Agent"
                  error={!!errors.displayName}
                  helperText={
                    (errors.displayName?.message as string) ||
                    "A name for your agent"
                  }
                  fullWidth
                />
              </Form.ElementWrapper>
            )}
          />
          <Controller
            name="description"
            control={control}
            render={({ field }) => (
              <Form.ElementWrapper label="Description (optional)" name="description">
                <TextField
                  {...field}
                  id="description"
                  placeholder="Short description of what this agent does"
                  multiline
                  minRows={2}
                  maxRows={6}
                  error={!!errors.description}
                  helperText={errors.description?.message as string}
                  fullWidth
                />
              </Form.ElementWrapper>
            )}
          />
        </Form.Stack>
      </Form.Section>

      <Form.Section>
        <Form.Subheader>Repository Details</Form.Subheader>
        <Form.Stack spacing={2}>
          <Controller
            name="repositoryUrl"
            control={control}
            render={({ field }) => (
              <Form.ElementWrapper label="GitHub Repository" name="repositoryUrl">
                <TextField
                  {...field}
                  id="repositoryUrl"
                  placeholder="https://github.com/username/repo"
                  error={!!errors.repositoryUrl}
                  helperText={errors.repositoryUrl?.message as string}
                  fullWidth
                />
              </Form.ElementWrapper>
            )}
          />
          <Form.Stack direction="row" spacing={2}>
            <Controller
              name="branch"
              control={control}
              render={({ field }) => (
                <Form.ElementWrapper label="Branch" name="branch">
                  <TextField
                    {...field}
                    id="branch"
                    placeholder="main"
                    error={!!errors.branch}
                    helperText={errors.branch?.message as string}
                    fullWidth
                  />
                </Form.ElementWrapper>
              )}
            />
            <Controller
              name="appPath"
              control={control}
              render={({ field }) => (
                <Form.ElementWrapper label="Project Path" name="appPath">
                  <TextField
                    {...field}
                    id="appPath"
                    placeholder="my-agent"
                    error={!!errors.appPath}
                    helperText={errors.appPath?.message as string}
                    fullWidth
                  />
                </Form.ElementWrapper>
              )}
            />
          </Form.Stack>
        </Form.Stack>
      </Form.Section>

      <Form.Section>
        <Form.Subheader>Build Details</Form.Subheader>
        <Form.Stack spacing={2}>
          <Form.Stack direction="row" spacing={2}>
            <Controller
              name="language"
              control={control}
              render={({ field }) => (
                <Form.ElementWrapper label="Language" name="language">
                  <TextField
                    {...field}
                    id="language"
                    placeholder="python"
                    disabled
                    error={!!errors.language}
                    helperText={
                      (errors.language?.message as string) ||
                      "e.g., python, nodejs, go"
                    }
                    fullWidth
                  />
                </Form.ElementWrapper>
              )}
            />
            <Controller
              name="languageVersion"
              control={control}
              render={({ field }) => (
                <Form.ElementWrapper label="Language Version" name="languageVersion">
                  <TextField
                    {...field}
                    id="languageVersion"
                    placeholder="3.11"
                    error={!!errors.languageVersion}
                    helperText={
                      (errors.languageVersion?.message as string) ||
                      "e.g., 3.11, 20, 1.21"
                    }
                    fullWidth
                  />
                </Form.ElementWrapper>
              )}
            />
          </Form.Stack>
          <Controller
            name="runCommand"
            control={control}
            render={({ field }) => (
              <Form.ElementWrapper label="Start Command" name="runCommand">
                <TextField
                  {...field}
                  id="runCommand"
                  placeholder="python main.py"
                  error={!!errors.runCommand}
                  helperText={
                    (errors.runCommand?.message as string) ||
                    "Dependencies auto-install from package.json, requirements.txt, or pyproject.toml"
                  }
                  fullWidth
                />
              </Form.ElementWrapper>
            )}
          />
        </Form.Stack>
      </Form.Section>

      <InputInterface />
      <EnvironmentVariable />
    </Form.Stack>
  );
};

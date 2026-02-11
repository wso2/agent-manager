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

import { Box, Form, Select, MenuItem, TextField } from "@wso2/oxygen-ui";
import { useFormContext, Controller, useWatch } from "react-hook-form";
import { useEffect, useMemo } from "react";
import { useParams } from "react-router-dom";
import { debounce } from "lodash";
import { useGenerateResourceName } from "@agent-management-platform/api-client";
import { AddProjectFormValues } from "../form/schema";

export const ProjectForm = () => {
  const {
    control,
    formState: { errors },
    setValue,
  } = useFormContext<AddProjectFormValues>();
  const { orgId } = useParams<{ orgId: string }>();
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
          resourceType: 'project',
        }, {
          onSuccess: (data) => {
            setValue("name", data.name, {
              shouldValidate: true,
              shouldDirty: true,
              shouldTouch: false,
            });
          },
          onError: (error) => {
            // eslint-disable-next-line no-console
            console.error('Failed to generate name:', error);
          }
        });
      }, 500), // 500ms delay
    [generateName, setValue]
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
      // Clear the name field if display name is empty
      debouncedGenerateName.cancel();
      setValue("name", "", {
        shouldValidate: true,
        shouldDirty: true,
        shouldTouch: false,
      });
    }
  }, [displayName, setValue, debouncedGenerateName]);

  return (
    <Form.Stack spacing={3}>
      <Form.Section>
        <Form.Subheader>Project Details</Form.Subheader>
        <Form.Stack spacing={2}>
          <Controller
            name="displayName"
            control={control}
            render={({ field }) => (
              <Form.ElementWrapper label="Name" name="displayName">
                <TextField
                  {...field}
                  id="displayName"
                  placeholder="e.g., Customer Support Platform"
                  error={!!errors.displayName}
                  helperText={errors.displayName?.message as string}
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
                  placeholder="Short description of this project"
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

          <Box display="none">
            <Controller
              name="deploymentPipeline"
              control={control}
              defaultValue="default"
              render={({ field }) => (
                <Form.ElementWrapper label="Deployment Pipeline" name="deploymentPipeline">
                  <Select
                    {...field}
                    id="deploymentPipeline"
                    error={!!errors.deploymentPipeline}
                    fullWidth
                  >
                    <MenuItem value="default">default</MenuItem>
                  </Select>
                </Form.ElementWrapper>
              )}
            />
          </Box>
        </Form.Stack>
      </Form.Section>
    </Form.Stack>
  );
};

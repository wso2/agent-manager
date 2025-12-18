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

import { Box, Card, CardContent, Typography, FormControl, Select, MenuItem, FormHelperText } from "@wso2/oxygen-ui";
import { useFormContext, Controller } from "react-hook-form";
import { useEffect, useMemo } from "react";
import { useParams } from "react-router-dom";
import { debounce } from "lodash";
import { TextInput } from "@agent-management-platform/views";
import { useGenerateResourceName } from "@agent-management-platform/api-client";
import { AddProjectFormValues } from "../form/schema";

export const ProjectForm = () => {
  const {
    register,
    control,
    formState: { errors },
    watch,
    setValue,
  } = useFormContext<AddProjectFormValues>();
  const { orgId } = useParams<{ orgId: string }>();
  const displayName = watch("displayName");
  
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
    [generateName, setValue, orgId]
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
    <Box display="flex" flexDirection="column" gap={2} flexGrow={1}>
      <Card variant="outlined">
        <CardContent sx={{ gap: 1, display: "flex", flexDirection: "column" }}>
          <Box display="flex" flexDirection="column" gap={1}>
            <Typography variant="h5">Project Details</Typography>
          </Box>
          <Box display="flex" flexDirection="column" gap={1}>
            <TextInput
              placeholder="e.g., Customer Support Platform"
              label="Name"
              size="small"
              fullWidth
              error={!!errors.displayName}
              helperText={
                (errors.displayName?.message as string)
              }
              {...register("displayName")}
            />
            <TextInput
              placeholder="Short description of this project"
              label="Description (optional)"
              fullWidth
              size="small"
              multiline
              minRows={2}
              maxRows={6}
              error={!!errors.description}
              helperText={errors.description?.message as string}
              {...register("description")}
            />
            <Box display="none" flexDirection="column" gap={0.5}>
              <Typography variant="body2" component="label">
                Deployment Pipeline
              </Typography>
              <Controller
                name="deploymentPipeline"
                control={control}
                defaultValue="default"
                render={({ field }) => (
                  <FormControl fullWidth size="small" error={!!errors.deploymentPipeline}>
                    <Select
                      {...field}
                    >
                      <MenuItem value="default">default</MenuItem>
                    </Select>
                    {errors.deploymentPipeline && (
                      <FormHelperText>{errors.deploymentPipeline.message as string}</FormHelperText>
                    )}
                    {!errors.deploymentPipeline && (
                      <FormHelperText>Name of the deployment pipeline to use</FormHelperText>
                    )}
                  </FormControl>
                )}
              />
            </Box>
          </Box>
        </CardContent>
      </Card>
    </Box>
  );
};

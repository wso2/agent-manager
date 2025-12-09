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
import { useEffect, useRef } from "react";
import { TextInput } from "@agent-management-platform/views";
import { AddProjectFormValues } from "../form/schema";

// Generate a random 6-character string
const generateRandomString = (length: number = 6): string => {
  const chars = "abcdefghijklmnopqrstuvwxyz0123456789";
  let result = "";
  for (let i = 0; i < length; i++) {
    result += chars.charAt(Math.floor(Math.random() * chars.length));
  }
  return result;
};

// Convert display name to URL-friendly format
const sanitizeNameForUrl = (displayName: string): string => {
  return displayName
    .toLowerCase()
    .trim()
    .replace(/[^a-z0-9\s-]/g, "") // Remove special characters
    .replace(/\s+/g, "-") // Replace spaces with hyphens
    .replace(/-+/g, "-") // Replace multiple hyphens with single hyphen
    .replace(/^-|-$/g, ""); // Remove leading/trailing hyphens
};

export const ProjectForm = () => {
  const {
    register,
    control,
    formState: { errors },
    watch,
    setValue,
  } = useFormContext<AddProjectFormValues>();
  const isNameManuallyEdited = useRef(false);
  const randomSuffix = useRef<string>("");
  const displayName = watch("displayName");

  // Generate random suffix once
  if (!randomSuffix.current) {
    randomSuffix.current = generateRandomString(6);
  }

  // Auto-generate name from display name
  useEffect(() => {
    if (displayName && !isNameManuallyEdited.current) {
      const sanitizedName = sanitizeNameForUrl(displayName);
      if (sanitizedName) {
        const generatedName = `${sanitizedName.substring(0, 10)}-${randomSuffix.current}`;
        setValue("name", generatedName, {
          shouldValidate: true,
          shouldDirty: true,
          shouldTouch: false,
        });
      }
    } else if (!displayName && !isNameManuallyEdited.current) {
      // Clear the name field if display name is empty
      setValue("name", "", {
        shouldValidate: true,
        shouldDirty: true,
        shouldTouch: false,
      });
    }
  }, [displayName, setValue]);

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
            <Box display="flex" flexDirection="column" gap={0.5}>
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

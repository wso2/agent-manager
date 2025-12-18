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

import { Box, Card, CardContent, Typography } from "@wso2/oxygen-ui";
import { useFormContext } from "react-hook-form";
import { useEffect, useMemo } from "react";
import { useParams } from "react-router-dom";
import { debounce } from "lodash";
import { TextInput } from "@agent-management-platform/views";
import { useGenerateResourceName } from "@agent-management-platform/api-client";

export const ConnectAgentForm = () => {
  const {
    register,
    formState: { errors },
    watch,
    setValue,
  } = useFormContext();
  const { orgId, projectId } = useParams<{ orgId: string; projectId: string }>();
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
          resourceType: 'agent',
          projectName: projectId,
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
    // eslint-disable-next-line react-hooks/exhaustive-deps
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
            <Typography variant="h5">Agent Details</Typography>
          </Box>
          <Box display="flex" flexDirection="column" gap={1}>
            <TextInput
              placeholder="e.g., Customer Support"
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
              placeholder="Short description of what this agent does"
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
          </Box>
        </CardContent>
      </Card>
    </Box>
  );
};

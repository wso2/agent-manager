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

import { CircularProgress, Form, Stack, TextField, Typography } from "@wso2/oxygen-ui";
import { useCallback, useEffect, useMemo } from "react";
import { useParams } from "react-router-dom";
import { debounce } from "lodash";
import { useGenerateResourceName } from "@agent-management-platform/api-client";
import { LabelsEditor, MarkdownEditor } from "@agent-management-platform/shared-component";
import type { CreateAgentFormValues } from "../form/schema";
import { INPUT_LIMITS } from "@agent-management-platform/types";

interface CatalogAgentFormProps {
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

export const CatalogAgentForm = ({
  formData,
  setFormData,
  errors,
  setFieldError,
  validateField,
}: CatalogAgentFormProps) => {
  const { orgId, projectId } = useParams<{ orgId: string; projectId: string }>();

  const { mutate: generateName, isPending: isGeneratingName } = useGenerateResourceName({
    orgName: orgId,
  });

  const handleFieldChange = useCallback(
    (field: keyof CreateAgentFormValues, value: unknown) => {
      setFormData((prevData) => {
        const newData = { ...prevData, [field]: value };
        const error = validateField(field, value, newData);
        setFieldError(field, error);
        return newData;
      });
    },
    [setFormData, validateField, setFieldError]
  );

  const debouncedGenerateName = useMemo(
    () =>
      debounce((name: string) => {
        if (name.length < 3) {
          handleFieldChange("name", "");
          return;
        }

        generateName(
          {
            displayName: name,
            resourceType: "agent",
            projectName: projectId,
          },
          {
            onSuccess: (data: { name: string }) => {
              handleFieldChange("name", data.name);
            },
            onError: (error: unknown) => {
              // eslint-disable-next-line no-console
              console.error("Failed to generate name:", error);
            },
          }
        );
      }, 500),
    [generateName, handleFieldChange, projectId]
  );

  useEffect(() => {
    return () => {
      debouncedGenerateName.cancel();
    };
  }, [debouncedGenerateName]);

  useEffect(() => {
    if (formData.displayName && formData.displayName.length >= 3) {
      debouncedGenerateName(formData.displayName);
    } else {
      debouncedGenerateName.cancel();
      handleFieldChange("name", "");
    }
  }, [formData.displayName, debouncedGenerateName, handleFieldChange]);

  return (
    <Form.Section>
      <Form.Subheader>Agent Details</Form.Subheader>
      <Form.Stack spacing={2}>
        <Form.ElementWrapper label="Name" name="displayName">
          <TextField
            slotProps={{ htmlInput: { maxLength: INPUT_LIMITS.NAME } }}
            id="displayName"
            placeholder="e.g., Customer Support Agent"
            value={formData.displayName}
            onChange={(e) => handleFieldChange("displayName", e.target.value)}
            error={!!errors.displayName}
            helperText={
              isGeneratingName ? (
                <Stack direction="row" alignItems="center" gap={1}>
                  <CircularProgress size={12} />
                  <Typography variant="caption">Validating name...</Typography>
                </Stack>
              ) : (
                errors.displayName || "A name for your agent"
              )
            }
            fullWidth
          />
        </Form.ElementWrapper>
        <Form.ElementWrapper label="Description (optional)" name="description">
          <MarkdownEditor
            id="description"
            placeholder="Short description of what this agent does. Markdown is supported."
            value={formData.description || ""}
            onChange={(value) => handleFieldChange("description", value)}
            error={!!errors.description}
            helperText={errors.description}
          />
        </Form.ElementWrapper>
        <Form.ElementWrapper label="Labels (optional)" name="labels">
          <LabelsEditor
            hideTitle
            description="Attach key/value labels to organize and filter agents."
            value={formData.labels ?? {}}
            onChange={(labels) => handleFieldChange("labels", labels)}
          />
        </Form.ElementWrapper>
      </Form.Stack>
    </Form.Section>
  );
};

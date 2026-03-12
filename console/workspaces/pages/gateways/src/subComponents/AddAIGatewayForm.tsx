/**
 * Copyright (c) 2026, WSO2 LLC. (https://www.wso2.com).
 *
 * WSO2 LLC. licenses this file to you under the Apache License,
 * Version 2.0 (the "License"); you may not use this file except
 * in compliance with the License. You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing,
 * software distributed under the License is distributed on an
 * "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
 * KIND, either express or implied. See the License for the
 * specific language governing permissions and limitations
 * under the License.
 */

import React, { useCallback, useEffect, useRef } from "react";
import {
  Form,
  MenuItem,
  Select,
  Stack,
  Switch,
  TextField,
  Typography,
  useTheme,
} from "@wso2/oxygen-ui";
import { Check } from "@wso2/oxygen-ui-icons-react";
import { useListEnvironments } from "@agent-management-platform/api-client";
import type { AddGatewayFormValues } from "../form/schema";
import { useParams } from "react-router-dom";

function toSlug(value: string): string {
  return value
    .toLowerCase()
    .trim()
    .replace(/\s+/g, "-")
    .replace(/[^a-z0-9-]/g, "")
    .replace(/^-+|-+$/g, "");
}

interface AddAIGatewayFormProps {
  formData: AddGatewayFormValues;
  setFormData: React.Dispatch<React.SetStateAction<AddGatewayFormValues>>;
  errors: Partial<Record<keyof AddGatewayFormValues, string>>;
  setFieldError: (
    field: keyof AddGatewayFormValues,
    error: string | undefined
  ) => void;
  validateField: (
    field: keyof AddGatewayFormValues,
    value: unknown,
    fullData?: AddGatewayFormValues
  ) => string | undefined;
}

export const AddAIGatewayForm: React.FC<AddAIGatewayFormProps> = ({
  formData,
  setFormData,
  errors,
  setFieldError,
  validateField,
}) => {
  const { orgId } = useParams<{ orgId: string }>();
  const theme = useTheme();

  const { data: environments = [] } = useListEnvironments({
    orgName: orgId,
  });

  const hasInitializedEnvironments = useRef(false);
  useEffect(() => {
    if (environments.length > 0 && !hasInitializedEnvironments.current) {
      hasInitializedEnvironments.current = true;
      const firstEnvId = environments[0].id;
      if (firstEnvId) {
        setFormData((prev) => ({
          ...prev,
          environmentIds: [firstEnvId],
        }));
      }
    }
  }, [environments, setFormData]);

  const handleFieldChange = useCallback(
    (field: keyof AddGatewayFormValues, value: unknown) => {
      const newData = { ...formData, [field]: value };
      setFormData(newData);
      const error = validateField(field, value, newData);
      setFieldError(field, error);
    },
    [formData, setFormData, validateField, setFieldError]
  );

  useEffect(() => {
    if (formData.displayName) {
      const slug = toSlug(formData.displayName);
      if (slug !== formData.name) {
        setFormData((prev: AddGatewayFormValues) => ({ ...prev, name: slug }));
        const error = validateField("name", slug, { ...formData, name: slug });
        setFieldError("name", error);
      }
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [formData.displayName, setFieldError]);

  const isNameValid =
    formData.name &&
    /^[a-z0-9-]+$/.test(formData.name) &&
    formData.name.length <= 64;

  return (
    <Form.Stack spacing={3}>
      <Form.Section>
        <Form.Subheader>Gateway Details</Form.Subheader>
        <Form.Stack spacing={2}>
          <Form.ElementWrapper label="Name" name="displayName">
            <TextField
              id="displayName"
              placeholder="e.g., Production AI Gateway"
              value={formData.displayName}
              onChange={(e: React.ChangeEvent<HTMLInputElement>) =>
                handleFieldChange("displayName", e.target.value)
              }
              error={!!errors.displayName}
              helperText={
                errors.displayName
              }
              fullWidth
              slotProps={{
                input: {
                  endAdornment:
                    !!formData.displayName && isNameValid ? (
                      <Check
                        size={20}
                        color={theme.vars?.palette.success.main}
                      />
                    ) : null,
                },
              }}
            />
          </Form.ElementWrapper>
          <Form.ElementWrapper label="Virtual Host" name="vhost">
            <TextField
              id="vhost"
              placeholder="e.g., api.production.example.com"
              value={formData.vhost}
              onChange={(e: React.ChangeEvent<HTMLInputElement>) =>
                handleFieldChange("vhost", e.target.value)
              }
              error={!!errors.vhost}
              helperText={errors.vhost || "FQDN or IP for the gateway"}
              fullWidth
            />
          </Form.ElementWrapper>
          <Form.ElementWrapper label="Critical production gateway" name="isCritical">
            <Stack direction="row" alignItems="center" spacing={1}>
              <Switch
                checked={formData.isCritical}
                onChange={(_, checked) => handleFieldChange("isCritical", checked)}
              />
              <Typography variant="caption" color="text.secondary">
                Mark as critical for production deployments
              </Typography>
            </Stack>
          </Form.ElementWrapper>
          <Form.ElementWrapper
            label="Environments"
            name="environmentIds"
          >
            <Select
              multiple
              value={formData.environmentIds ?? []}
              onChange={(e: { target: { value: unknown } }) => {
                const v = e.target.value;
                handleFieldChange(
                  "environmentIds",
                  Array.isArray(v) ? v : [v]
                );
              }}
              renderValue={(selected) =>
                (selected as string[])
                  .map((id) => {
                    const env = environments.find((e) => e.id === id);
                    return env?.displayName || env?.name || id;
                  })
                  .join(", ")
              }
              fullWidth
              displayEmpty
            >
              {environments
                .filter((env) => env.id)
                .map((env) => (
                  <MenuItem key={env.id} value={env.id!}>
                    {env.displayName || env.name}
                  </MenuItem>
                ))}
            </Select>
          </Form.ElementWrapper>
        </Form.Stack>
      </Form.Section>
    </Form.Stack>
  );
};

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

import React, { useCallback, useMemo, useState } from "react";
import { Alert, Form } from "@wso2/oxygen-ui";
import { PageLayout, useFormValidation } from "@agent-management-platform/views";
import { generatePath, useNavigate, useParams } from "react-router-dom";
import {
  absoluteRouteMap,
  type CreateGatewayRequest,
} from "@agent-management-platform/types";
import { useCreateGateway } from "@agent-management-platform/api-client";
import {
  addGatewaySchema,
  type AddGatewayFormValues,
} from "./form/schema";
import { AddAIGatewayForm } from "./subComponents/AddAIGatewayForm";
import { CreateButtons } from "./subComponents/CreateButtons";

function toSlug(value: string): string {
  return value
    .toLowerCase()
    .trim()
    .replace(/\s+/g, "-")
    .replace(/[^a-z0-9-]/g, "")
    .replace(/^-+|-+$/g, "");
}

export const AddAIGatewayOrganization: React.FC = () => {
  const { orgId } = useParams<{ orgId: string }>();
  const navigate = useNavigate();

  const [formData, setFormData] = useState<AddGatewayFormValues>({
    name: "",
    displayName: "",
    vhost: "",
    isCritical: false,
    environmentIds: [],
  });

  const { errors, validateForm, setFieldError, validateField } =
    useFormValidation<AddGatewayFormValues>(addGatewaySchema);

  const {
    mutate: createGateway,
    isPending,
    error: createError,
  } = useCreateGateway();

  const backHref = useMemo(
    () =>
      orgId
        ? generatePath(
            absoluteRouteMap.children.org.children.gateways.path,
            { orgId }
          )
        : "#",
    [orgId]
  );

  const handleCancel = useCallback(() => {
    navigate(backHref);
  }, [navigate, backHref]);

  const [lastSubmittedValidationErrors, setLastSubmittedValidationErrors] =
    useState<typeof errors>({});

  const handleSubmit = useCallback(() => {
    const dataToValidate = {
      ...formData,
      name: formData.name || toSlug(formData.displayName),
    };
    const result = addGatewaySchema.safeParse(dataToValidate);
    if (!result.success) {
      const fieldErrors: Partial<Record<keyof AddGatewayFormValues, string>> = {};
      result.error.issues.forEach((issue) => {
        if (issue.path[0]) {
          fieldErrors[issue.path[0] as keyof AddGatewayFormValues] = issue.message;
        }
      });
      setLastSubmittedValidationErrors(fieldErrors);
      validateForm(dataToValidate); // syncs errors to form fields
      return;
    }
    setLastSubmittedValidationErrors({});

    const payload: CreateGatewayRequest = {
      name: dataToValidate.name.trim(),
      displayName: formData.displayName.trim(),
      gatewayType: "AI",
      vhost: formData.vhost.trim(),
      isCritical: formData.isCritical,
      environmentIds: formData.environmentIds,
    };

    createGateway(
      {
        params: { orgName: orgId ?? "" },
        body: payload,
      },
      {
        onSuccess: (data) => {
          const viewPath = generatePath(
            absoluteRouteMap.children.org.children.gateways.children.view.path,
            { orgId: orgId ?? "", gatewayId: data.uuid }
          );
          navigate(viewPath);
        },
        onError: (e: unknown) => {
          // eslint-disable-next-line no-console
          console.error("Failed to create AI gateway:", e);
        },
      }
    );
  }, [formData, validateForm, createGateway, navigate, orgId]);

  return (
    <PageLayout
      title="Add AI Gateway"
      backHref={backHref}
      backLabel="Back to AI Gateways"
      disableIcon
    >
      <Form.Stack spacing={2}>
        <AddAIGatewayForm
          formData={formData}
          setFormData={setFormData}
          errors={errors}
          setFieldError={
            setFieldError as (
              field: keyof AddGatewayFormValues,
              error: string | undefined
            ) => void
          }
          validateField={
            validateField as (
              field: keyof AddGatewayFormValues,
              value: unknown,
              fullData?: AddGatewayFormValues
            ) => string | undefined
          }
        />

        {!!createError && (
          <Alert severity="error" sx={{ mt: 2 }}>
            {createError instanceof Error
              ? createError.message
              : "Failed to create AI gateway"}
          </Alert>
        )}

        <CreateButtons
          lastSubmittedValidationErrors={lastSubmittedValidationErrors}
          isPending={isPending}
          onCancel={handleCancel}
          onSubmit={handleSubmit}
        />
      </Form.Stack>
    </PageLayout>
  );
};

export default AddAIGatewayOrganization;

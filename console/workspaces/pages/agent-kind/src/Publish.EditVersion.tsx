/**
 * Copyright (c) 2026, WSO2 LLC. (https://www.wso2.com).
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

import React, { useCallback, useEffect, useState } from "react";
import { generatePath, useNavigate, useParams } from "react-router-dom";
import {
  Button,
  Form,
  Skeleton,
  Stack,
} from "@wso2/oxygen-ui";
import { X as CloseIcon } from "@wso2/oxygen-ui-icons-react";
import { PageLayout, TextInput } from "@agent-management-platform/views";
import { absoluteRouteMap } from "@agent-management-platform/types";
import { useGetAgentKind, useUpdateAgentKind } from "@agent-management-platform/api-client";
import { useConfirmationDialog } from "@agent-management-platform/shared-component";

export const PublishEditVersion: React.FC = () => {
  const navigate = useNavigate();
  const { orgId, projectId, agentId, versionId } = useParams<{
    orgId: string;
    projectId: string;
    agentId: string;
    versionId: string;
  }>();

  const backHref = generatePath(
    absoluteRouteMap.children.org.children.projects.children.agents
      .children.publish.children.versionDetails.path,
    { orgId: orgId ?? "", projectId: projectId ?? "", agentId: agentId ?? "", versionId: versionId ?? "" },
  );

  const { data: kind, isLoading } = useGetAgentKind({ orgName: orgId!, kindName: agentId! });
  const { mutateAsync: updateKind, isPending: isSaving } = useUpdateAgentKind();
  const { addConfirmation } = useConfirmationDialog();

  const [displayName, setDisplayName] = useState("");
  const [description, setDescription] = useState("");

  useEffect(() => {
    if (kind) {
      setDisplayName(kind.displayName);
      setDescription(kind.description ?? "");
    }
  }, [kind]);

  const initialDisplayName = kind?.displayName ?? "";
  const initialDescription = kind?.description ?? "";
  const isDirty = displayName !== initialDisplayName || description !== initialDescription;

  const handleSave = useCallback(async () => {
    await updateKind({
      params: { orgName: orgId!, kindName: agentId! },
      body: { displayName: displayName.trim(), description: description.trim() || undefined },
    });
    navigate(backHref);
  }, [orgId, agentId, displayName, description, updateKind, navigate, backHref]);

  const handleCancel = useCallback(() => {
    if (isDirty) {
      addConfirmation({
        title: "Discard Changes?",
        description: "You have unsaved changes. Are you sure you want to leave without saving?",
        confirmButtonText: "Discard",
        confirmButtonColor: "error",
        onConfirm: () => navigate(backHref),
      });
    } else {
      navigate(backHref);
    }
  }, [isDirty, addConfirmation, navigate, backHref]);

  return (
    <PageLayout
      title="Edit Agent Kind"
      description="Update the display name and description for this Agent Kind."
      disableIcon
      backHref={backHref}
      backLabel="Back to Version"
      actions={
        <Stack direction="row" spacing={1}>
          <Button
            variant="outlined"
            startIcon={<CloseIcon size={16} />}
            onClick={handleCancel}
            disabled={isSaving}
          >
            Cancel
          </Button>
          <Button
            variant="contained"
            color="primary"
            onClick={handleSave}
            disabled={isSaving || isLoading || !displayName.trim()}
          >
            {isSaving ? "Saving..." : "Save Changes"}
          </Button>
        </Stack>
      }
    >
      {isLoading ? (
        <Stack spacing={2} sx={{ maxWidth: 600 }}>
          <Skeleton variant="rounded" height={40} />
          <Skeleton variant="rounded" height={80} />
        </Stack>
      ) : (
        <Form.Stack spacing={3}>
          <Form.Section>
            <Form.Subheader>Kind Details</Form.Subheader>
            <Form.Stack spacing={2}>
              <Form.ElementWrapper label="Display Name" name="displayName">
                <TextInput
                  id="displayName"
                  value={displayName}
                  onChange={(e) => setDisplayName(e.target.value)}
                  fullWidth
                  size="small"
                />
              </Form.ElementWrapper>
              <Form.ElementWrapper label="Description" name="description">
                <TextInput
                  id="description"
                  value={description}
                  onChange={(e) => setDescription(e.target.value)}
                  fullWidth
                  size="small"
                  multiline
                  rows={3}
                />
              </Form.ElementWrapper>
            </Form.Stack>
          </Form.Section>
        </Form.Stack>
      )}
    </PageLayout>
  );
};

export default PublishEditVersion;


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

import { useCallback, useState } from "react";
import { Button, Skeleton, Stack } from "@wso2/oxygen-ui";
import { SearchX } from "@wso2/oxygen-ui-icons-react";
import { Navigate, useLocation, useNavigate, useParams } from "react-router-dom";
import { PageLayout, StateNotice, useReturnTo } from "@agent-management-platform/views";
import { useUnsavedChangesGuard } from "@agent-management-platform/shared-component";
import {
  useCreateEventDestination,
  useEventDestination,
  useUpdateEventDestination,
} from "@agent-management-platform/api-client";
import type { EventDestinationResponse } from "@agent-management-platform/types";
import { DestinationForm } from "./DestinationForm";
import { SecretReveal } from "./eventsUi";
import type { DestinationPageState, SubscriptionEditorState } from "./subscriptionDraft";
import type { EventsAccess } from "./useEventsAccess";

export interface DestinationEditorProps {
  orgName?: string;
  access: EventsAccess;
  /** The organization's Events page, to return to on a direct visit. */
  listPath: string;
}

/**
 * Add or edit an organization destination. Opened by "Add & use" in the
 * subscription editor, it returns there with the new destination selected.
 */
export function DestinationEditor({ orgName, access, listPath }: DestinationEditorProps) {
  const navigate = useNavigate();
  const location = useLocation();
  const { destinationId } = useParams<{ destinationId: string }>();
  const pageState = location.state as DestinationPageState | null;
  const draft = pageState?.subscriptionDraft;
  const returnTo = useReturnTo(`${listPath}?tab=destinations`);

  const existing = useEventDestination(orgName, destinationId);
  const { mutate: create, isPending: creating } = useCreateEventDestination(orgName);
  const { mutate: update, isPending: updating } = useUpdateEventDestination(orgName);
  const [dirty, setDirty] = useState(false);
  const { allowNavigation } = useUnsavedChangesGuard(dirty);
  // A new webhook's signing secret, shown once before leaving the page.
  const [created, setCreated] = useState<EventDestinationResponse | null>(null);

  // The subscription editor's draft goes back with every way out of the
  // page, with the new destination selected once it exists.
  const editorState = useCallback(
    (saved?: EventDestinationResponse): SubscriptionEditorState | undefined =>
      draft
        ? {
            from: draft.from,
            subscriptionDraft: saved ? { ...draft, destinationId: saved.id } : draft,
          }
        : undefined,
    [draft],
  );
  const leave = useCallback(
    (saved?: EventDestinationResponse) =>
      allowNavigation(() => navigate(returnTo, { state: editorState(saved) })),
    [allowNavigation, editorState, navigate, returnTo],
  );

  if (access.resolved && !access.canManageDestinations) {
    return <Navigate to={`${listPath}?tab=destinations`} replace />;
  }

  const editing = !!destinationId;
  const title = editing
    ? `Edit ${existing.data?.name ?? "destination"}`
    : "Add destination";
  const layout = (children: React.ReactNode) => (
    <PageLayout
      title={title}
      description={
        draft
          ? "The new destination is selected for the subscription you were editing."
          : "Where events leave the platform: a notification service or a webhook."
      }
      disableIcon
      backHref={returnTo}
      backState={editorState(created ?? undefined)}
      backLabel="Back"
    >
      {children}
    </PageLayout>
  );

  if (editing && existing.isLoading) {
    return layout(
      <Stack spacing={2}>
        <Skeleton variant="rounded" height={120} />
        <Skeleton variant="rounded" height={240} />
      </Stack>,
    );
  }
  if (editing && !existing.data) {
    return layout(
      <StateNotice
        icon={SearchX}
        tone="error"
        title={existing.error ? "Couldn't load the destination" : "Destination not found"}
        description={
          existing.error
            ? "Reload the page to try again."
            : "It may have been deleted. Choose another from the destinations list."
        }
        actions={
          <Button variant="outlined" onClick={() => navigate(`${listPath}?tab=destinations`)}>
            Back to destinations
          </Button>
        }
      />,
    );
  }

  if (created?.secret) {
    return layout(
      <Stack spacing={2}>
        <SecretReveal name={created.name} secret={created.secret} />
        <Stack direction="row" justifyContent="flex-start">
          <Button variant="contained" onClick={() => leave(created)}>
            Done
          </Button>
        </Stack>
      </Stack>,
    );
  }

  return layout(
    <DestinationForm
      orgName={orgName}
      destination={existing.data}
      defaultAvailability={pageState?.defaultAvailability}
      saving={creating || updating}
      submitLabel={editing ? "Save" : draft ? "Add & use" : "Add destination"}
      onDirtyChange={setDirty}
      onCancel={() => leave()}
      onSubmit={(body) => {
        if (existing.data) {
          update(
            { id: existing.data.id, body },
            {
              onSuccess: () => {
                setDirty(false);
                leave();
              },
            },
          );
          return;
        }
        create(body, {
          onSuccess: (saved) => {
            setDirty(false);
            if (saved.secret) {
              setCreated(saved);
            } else {
              leave(saved);
            }
          },
        });
      }}
    />,
  );
}

export default DestinationEditor;

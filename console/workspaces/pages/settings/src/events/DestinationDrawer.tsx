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

import { useEffect, useState } from "react";
import { Alert, Button, IconButton, Stack, Typography } from "@wso2/oxygen-ui";
import { Copy, Send } from "@wso2/oxygen-ui-icons-react";
import { DrawerContent, DrawerHeader, DrawerWrapper } from "@agent-management-platform/views";
import {
  useCreateEventDestination,
  useUpdateEventDestination,
} from "@agent-management-platform/api-client";
import type {
  DestinationAvailability,
  EventDestinationResponse,
} from "@agent-management-platform/types";
import { DestinationForm } from "./DestinationForm";

/** A signing secret to show once, with copy, after it is generated. */
export function SecretReveal({
  name,
  secret,
  onClose,
}: {
  name: string;
  secret: string;
  onClose?: () => void;
}) {
  return (
    <Alert
      severity="success"
      onClose={onClose}
      action={
        <IconButton
          size="small"
          aria-label="Copy signing secret"
          onClick={() => navigator.clipboard.writeText(secret)}
        >
          <Copy size={16} />
        </IconButton>
      }
    >
      <Typography variant="body2" fontWeight={600}>
        Signing secret for {name}. Copy it now; it is not shown again.
      </Typography>
      <Typography variant="body2" sx={{ fontFamily: "monospace", wordBreak: "break-all" }}>
        {secret}
      </Typography>
      <Typography variant="caption" color="text.secondary">
        Requests carry Standard Webhooks headers: verify webhook-signature (HMAC-SHA256 of
        &quot;id.timestamp.body&quot;) with this secret.
      </Typography>
    </Alert>
  );
}

export interface DestinationDrawerProps {
  open: boolean;
  orgName?: string;
  /** The destination to edit; undefined to add one. */
  destination?: EventDestinationResponse;
  /** Availability a new destination starts with. Keep the object stable. */
  defaultAvailability?: { availability: DestinationAvailability; projects: string[] };
  /** Label of the add button, e.g. "Add & use" when picking for a subscription. */
  createLabel?: string;
  onClose: () => void;
  /** Called with the destination once it is saved. */
  onSaved?: (destination: EventDestinationResponse) => void;
}

/** Add or edit a destination in a side drawer. */
export function DestinationDrawer({
  open,
  orgName,
  destination,
  defaultAvailability,
  createLabel = "Add destination",
  onClose,
  onSaved,
}: DestinationDrawerProps) {
  const { mutate: create, isPending: creating } = useCreateEventDestination(orgName);
  const { mutate: update, isPending: updating } = useUpdateEventDestination(orgName);
  const [created, setCreated] = useState<EventDestinationResponse | null>(null);

  useEffect(() => {
    if (!open) setCreated(null);
  }, [open]);

  const close = () => {
    setCreated(null);
    onClose();
  };

  return (
    <DrawerWrapper
      open={open}
      onClose={close}
      minWidth="min(720px, 100vw)"
      maxWidth="min(720px, 100vw)"
    >
      <DrawerHeader
        icon={<Send size={22} />}
        title={destination ? `Edit ${destination.name}` : "Add destination"}
        onClose={close}
      />
      <DrawerContent>
        {created?.secret ? (
          <Stack spacing={2}>
            <SecretReveal name={created.name} secret={created.secret} />
            <Stack direction="row" justifyContent="flex-end">
              <Button variant="contained" onClick={close}>
                Done
              </Button>
            </Stack>
          </Stack>
        ) : (
          <DestinationForm
            orgName={orgName}
            destination={destination}
            defaultAvailability={defaultAvailability}
            saving={creating || updating}
            submitLabel={destination ? "Save" : createLabel}
            onCancel={close}
            onSubmit={(body) => {
              if (destination) {
                update(
                  { id: destination.id, body },
                  {
                    onSuccess: (saved) => {
                      onSaved?.(saved);
                      close();
                    },
                  },
                );
                return;
              }
              create(body, {
                onSuccess: (saved) => {
                  onSaved?.(saved);
                  if (saved.secret) {
                    setCreated(saved); // show the signing secret once
                  } else {
                    close();
                  }
                },
              });
            }}
          />
        )}
      </DrawerContent>
    </DrawerWrapper>
  );
}

export default DestinationDrawer;

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

import React, { useCallback, useState } from "react";
import { Box, Button, Chip, Form, Stack, Typography } from "@wso2/oxygen-ui";
import { Plus } from "@wso2/oxygen-ui-icons-react";
import type { GuardrailDefinition } from "@agent-management-platform/api-client";
import type { ParameterValues } from "../PolicyParameterEditor/types";
import { GuardrailSelectorDrawer } from "../components/GuardrailSelectorDrawer";

export type GuardrailSelection = {
  name: string;
  version: string;
  displayName?: string;
  settings?: Record<string, unknown>;
};

interface GuardrailsSectionProps {
  guardrails: GuardrailSelection[];
  onAddGuardrail: (guardrail: GuardrailSelection) => void;
  onEditGuardrail: (guardrail: GuardrailSelection) => void;
  onRemoveGuardrail: (name: string, version: string) => void;
}

export const GuardrailsSection: React.FC<GuardrailsSectionProps> = ({
  guardrails,
  onAddGuardrail,
  onEditGuardrail,
  onRemoveGuardrail,
}) => {
  const [drawerOpen, setDrawerOpen] = useState(false);
  const [editingGuardrail, setEditingGuardrail] = useState<GuardrailSelection | null>(null);

  const handleAddGuardrail = useCallback(
    (guardrail: GuardrailDefinition, settings: ParameterValues) => {
      onAddGuardrail({
        name: guardrail.name,
        version: guardrail.version,
        displayName: guardrail.displayName,
        settings: settings as Record<string, unknown>,
      });
      setDrawerOpen(false);
      setEditingGuardrail(null);
    },
    [onAddGuardrail],
  );

  const handleEditGuardrail = useCallback(
    (guardrail: GuardrailDefinition, settings: ParameterValues) => {
      onEditGuardrail({
        name: guardrail.name,
        version: guardrail.version,
        displayName: guardrail.displayName,
        settings: settings as Record<string, unknown>,
      });
      setDrawerOpen(false);
      setEditingGuardrail(null);
    },
    [onEditGuardrail],
  );

  const handleChipClick = useCallback((g: GuardrailSelection) => {
    setEditingGuardrail(g);
    setDrawerOpen(true);
  }, []);

  const handleCloseDrawer = useCallback(() => {
    setDrawerOpen(false);
    setEditingGuardrail(null);
  }, []);

  return (
    <>
      <Form.Section>
        <Form.Header>Guardrails</Form.Header>
        <Stack spacing={3}>
          <Box>
            <Typography variant="body2" color="text.secondary">
              Add safety policies to enforce consistent protections.
            </Typography>
          </Box>

          <Stack direction="row" spacing={1} flexWrap="wrap" useFlexGap>
            {guardrails.map((g) => (
              <Chip
                key={`${g.name}@${g.version}`}
                label={`${g.displayName || g.name} (${g.version})`}
                color="default"
                variant="outlined"
                onClick={() => handleChipClick(g)}
                onDelete={() => onRemoveGuardrail(g.name, g.version)}
                sx={{ cursor: "pointer" }}
              />
            ))}
            <Button
              variant="outlined"
              size="small"
              startIcon={<Plus size={16} />}
              onClick={() => setDrawerOpen(true)}
            >
              Add Guardrail
            </Button>
          </Stack>
        </Stack>
      </Form.Section>

      <GuardrailSelectorDrawer
        open={drawerOpen}
        onClose={handleCloseDrawer}
        onSubmit={editingGuardrail ? handleEditGuardrail : handleAddGuardrail}
        disabledGuardrailKeys={
          editingGuardrail
            ? []
            : guardrails.map((g) => `${g.name}@${g.version}`)
        }
        existingSettings={
          editingGuardrail
            ? (editingGuardrail.settings as Record<string, unknown>)
            : undefined
        }
        editGuardrailKey={
          editingGuardrail
            ? `${editingGuardrail.name}@${editingGuardrail.version}`
            : undefined
        }
        title={editingGuardrail ? "Edit Guardrail" : "Add Guardrail"}
        subtitle={
          editingGuardrail
            ? "Update the guardrail configuration."
            : "Choose a guardrail to configure advanced options."
        }
        minWidth={800}
        maxWidth={800}
      />
    </>
  );
};

export default GuardrailsSection;

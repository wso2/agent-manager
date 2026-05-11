import React, { useState, useCallback, useMemo } from "react";
import { generatePath, useNavigate, useParams } from "react-router-dom";
import {
  Alert,
  Button,
  Form,
  Stack,
} from "@wso2/oxygen-ui";
import { X as CloseIcon } from "@wso2/oxygen-ui-icons-react";
import { PageLayout } from "@agent-management-platform/views";
import { absoluteRouteMap } from "@agent-management-platform/types";
import { DUMMY_CATALOG_LIST } from "./catalog.mock";
import { RuntimeConfigEditor, type RuntimeConfigRow } from "./RuntimeConfigEditor";
import { useConfirmationDialog } from "@agent-management-platform/shared-component";

const MOCK_ITEM = DUMMY_CATALOG_LIST[0];

type RuntimeConfigType = "string" | boolean | number;
type RuntimeConfigTypeOption = "string" | "number" | "boolean";

function runtimeTypeToOption(value: RuntimeConfigType): RuntimeConfigTypeOption {
  if (typeof value === "boolean") return "boolean";
  if (typeof value === "number") return "number";
  return "string";
}

function toRuntimeRows(runtimeConfig: Record<string, { isSecrete: boolean; type: RuntimeConfigType }> | null | undefined): RuntimeConfigRow[] {
  const rows = Object.entries(runtimeConfig ?? {}).map(([key, value]) => ({
    key,
    type: runtimeTypeToOption(value.type),
    isSecrete: Boolean(value.isSecrete),
  }));
  return rows.length > 0 ? rows : [{ key: "", type: "string", isSecrete: false }];
}

function optionToRuntimeType(option: RuntimeConfigTypeOption): RuntimeConfigType {
  if (option === "boolean") return false;
  if (option === "number") return 0;
  return "string";
}

export const PublishEditVersion: React.FC = () => {
  const navigate = useNavigate();
  const { orgId, projectId, agentId, versionId } = useParams<{
    orgId: string;
    projectId: string;
    agentId: string;
    versionId: string;
  }>();

  const backHref = generatePath(
    absoluteRouteMap.children.org.children.projects.children.agents.children.publish.children.versionDetails.path,
    { orgId: orgId ?? "", projectId: projectId ?? "", agentId: agentId ?? "", versionId: versionId ?? "" },
  );

  const version = versionId ? MOCK_ITEM.versions[versionId] : undefined;

  const initialRows = useMemo(() => toRuntimeRows(version?.runtimeConfig), [version]);
  const [values, setValues] = useState<RuntimeConfigRow[]>(() => toRuntimeRows(version?.runtimeConfig));
  const [isSaving, setIsSaving] = useState(false);

  const { addConfirmation } = useConfirmationDialog();

  const isDirty = useMemo(
    () => JSON.stringify(values) !== JSON.stringify(initialRows),
    [values, initialRows],
  );

  const handleSave = useCallback(() => {
    const payload = values.reduce<Record<string, { isSecrete: boolean; type: RuntimeConfigType }>>(
      (acc, row) => {
        const key = row.key.trim();
        if (!key) return acc;
        acc[key] = {
          isSecrete: row.isSecrete,
          type: optionToRuntimeType(row.type as RuntimeConfigTypeOption),
        };
        return acc;
      },
      {},
    );

    setIsSaving(true);
    // TODO: replace with real API call using payload
    void payload;
    setTimeout(() => {
      setIsSaving(false);
      navigate(backHref);
    }, 400);
  }, [values, navigate, backHref]);

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

  if (!version) {
    return (
      <PageLayout title="Edit Version" disableIcon backHref={backHref} backLabel="Back to Version">
        <Alert severity="error">Version "{versionId}" not found.</Alert>
      </PageLayout>
    );
  }

  return (
    <PageLayout
      title={`Edit v${versionId}`}
      description="Update runtime config for this version."
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
            disabled={isSaving}
          >
            {isSaving ? "Saving..." : "Save Changes"}
          </Button>
        </Stack>
      }
    >
      <Form.Stack spacing={3}>
        <Form.Section>
          <Form.Subheader>Runtime Configuration</Form.Subheader>
          <RuntimeConfigEditor rows={values} onChange={setValues} />
        </Form.Section>
      </Form.Stack>
    </PageLayout>
  );
};

export default PublishEditVersion;

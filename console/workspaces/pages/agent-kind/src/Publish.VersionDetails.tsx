import React, { useCallback, useMemo, useState } from "react";
import { generatePath, useLocation, useNavigate, useParams } from "react-router-dom";
import {
  Alert,
  Box,
  Button,
  Chip,
  Divider,
  Form,
  ListingTable,
  Stack,
  Typography,
} from "@wso2/oxygen-ui";
import { Edit } from "@wso2/oxygen-ui-icons-react";
import { DrawerWrapper, DrawerHeader, DrawerContent, PageLayout } from "@agent-management-platform/views";
import { absoluteRouteMap } from "@agent-management-platform/types";
import { SwaggerSpecViewer, useConfirmationDialog } from "@agent-management-platform/shared-component";
import { DUMMY_CATALOG_LIST, type CatalogItemVersion } from "./catalog.mock";
import { RuntimeConfigEditor, type RuntimeConfigRow } from "./RuntimeConfigEditor";

type RuntimeConfigType = "string" | boolean | number;

function runtimeTypeToOption(value: RuntimeConfigType): "string" | "number" | "boolean" {
  if (typeof value === "boolean") return "boolean";
  if (typeof value === "number") return "number";
  return "string";
}

function optionToRuntimeType(option: "string" | "number" | "boolean"): RuntimeConfigType {
  if (option === "boolean") return false;
  if (option === "number") return 0;
  return "string";
}

function toRuntimeRows(
  runtimeConfig: Record<string, { isSecrete: boolean; type: RuntimeConfigType }> | null | undefined,
): RuntimeConfigRow[] {
  const rows = Object.entries(runtimeConfig ?? {}).map(([key, value]) => ({
    key,
    type: runtimeTypeToOption(value.type),
    isSecrete: Boolean(value.isSecrete),
  }));
  return rows.length > 0 ? rows : [{ key: "", type: "string", isSecrete: false }];
}

const MOCK_ITEM = DUMMY_CATALOG_LIST[0];

export const PublishVersionDetails: React.FC = () => {
  const navigate = useNavigate();
  const location = useLocation();
  const { orgId, projectId, agentId, versionId } = useParams<{
    orgId: string;
    projectId: string;
    agentId: string;
    versionId: string;
  }>();

  const versionDetailsHref = generatePath(
    absoluteRouteMap.children.org.children.projects.children.agents.children.publish.children.versionDetails.path,
    { orgId: orgId ?? "", projectId: projectId ?? "", agentId: agentId ?? "", versionId: versionId ?? "" },
  );

  const editHref = versionDetailsHref + "/edit";

  const isEditOpen = location.pathname.endsWith("/edit");

  const backHref = generatePath(
    absoluteRouteMap.children.org.children.projects.children.agents.children.publish.path,
    { orgId: orgId ?? "", projectId: projectId ?? "", agentId: agentId ?? "" },
  );

  const version: CatalogItemVersion | undefined = versionId ? MOCK_ITEM.versions[versionId] : undefined;

  const openApiSpec = useMemo(() => {
    if (!version?.apiSpecs) return null;
    return version.apiSpecs as Record<string, unknown>;
  }, [version]);

  const runtimeConfigRows = useMemo(
    () => Object.entries(version?.runtimeConfig ?? {}),
    [version],
  );

  const formattedDate = version
    ? new Date(version.releaseDate).toLocaleDateString("en-US", {
      year: "numeric",
      month: "long",
      day: "numeric",
    })
    : undefined;

  // Edit drawer state
  const initialEditRows = useMemo(() => toRuntimeRows(version?.runtimeConfig), [version]);
  const [editValues, setEditValues] = useState<RuntimeConfigRow[]>(() => toRuntimeRows(version?.runtimeConfig));
  const [isSaving, setIsSaving] = useState(false);

  const { addConfirmation } = useConfirmationDialog();

  const isEditDirty = useMemo(
    () => JSON.stringify(editValues) !== JSON.stringify(initialEditRows),
    [editValues, initialEditRows],
  );

  const handleDrawerClose = useCallback(() => {
    if (isEditDirty) {
      addConfirmation({
        title: "Discard Changes?",
        description: "You have unsaved changes. Are you sure you want to close without saving?",
        confirmButtonText: "Discard",
        confirmButtonColor: "error",
        onConfirm: () => {
          setEditValues(initialEditRows);
          navigate(versionDetailsHref);
        },
      });
    } else {
      navigate(versionDetailsHref);
    }
  }, [isEditDirty, addConfirmation, initialEditRows, navigate, versionDetailsHref]);

  const handleSave = useCallback(() => {
    const payload = editValues.reduce<Record<string, { isSecrete: boolean; type: RuntimeConfigType }>>(
      (acc, row) => {
        const key = row.key.trim();
        if (!key) return acc;
        acc[key] = { isSecrete: row.isSecrete, type: optionToRuntimeType(row.type) };
        return acc;
      },
      {},
    );

    setIsSaving(true);
    void payload; // TODO: replace with real API call
    setTimeout(() => {
      setIsSaving(false);
      navigate(versionDetailsHref);
    }, 400);
  }, [editValues, navigate, versionDetailsHref]);

  return (
    <>
    <PageLayout
      title={`${MOCK_ITEM.title} v${versionId}`}
      description={MOCK_ITEM.description ?? "Version details"}
      disableIcon
      backHref={backHref}
      backLabel="Back to Publish"
      actions={
        <Button
          variant="outlined"
          startIcon={<Edit />}
          onClick={() => navigate(editHref)}
        >
          Edit
        </Button>
      }
    >
      <Stack spacing={3}>
        {/* Metadata */}
        <Stack direction="row" spacing={1} alignItems="center">
          <Chip label={`v${versionId}`} size="small" color="primary" variant="outlined" />
          {formattedDate && (
            <Typography variant="body2" color="text.secondary">
              Released on {formattedDate}
            </Typography>
          )}
        </Stack>

        <Divider />

        {/* Runtime Configuration */}
        <Stack spacing={1.5}>
          <Typography variant="subtitle1" fontWeight={600}>
            Runtime Configuration
          </Typography>
          {runtimeConfigRows.length > 0 ? (
            <ListingTable.Container>
              <ListingTable>
                <ListingTable.Head>
                  <ListingTable.Row>
                    <ListingTable.Cell width="40%">Key</ListingTable.Cell>
                    <ListingTable.Cell width="30%">Type</ListingTable.Cell>
                    <ListingTable.Cell width="30%">Secret</ListingTable.Cell>
                  </ListingTable.Row>
                </ListingTable.Head>
                <ListingTable.Body>
                  {runtimeConfigRows.map(([key, config]) => (
                    <ListingTable.Row key={key}>
                      <ListingTable.Cell>
                        <Typography variant="body2" fontWeight={500}>{key}</Typography>
                      </ListingTable.Cell>
                      <ListingTable.Cell>
                        <Typography variant="body2" color="text.secondary">
                          {typeof config.type === "boolean" ? "boolean" : typeof config.type === "number" ? "number" : "string"}
                        </Typography>
                      </ListingTable.Cell>
                      <ListingTable.Cell>
                        <Typography variant="body2" color="text.secondary">
                          {config.isSecrete ? "Yes" : "No"}
                        </Typography>
                      </ListingTable.Cell>
                    </ListingTable.Row>
                  ))}
                </ListingTable.Body>
              </ListingTable>
            </ListingTable.Container>
          ) : (
            <Alert severity="info">No runtime config keys available for this version.</Alert>
          )}
        </Stack>

        <Divider />

        {/* API Spec */}
        <Stack spacing={1.5}>
          <Typography variant="subtitle1" fontWeight={600}>
            API Specification
          </Typography>
          {openApiSpec ? (
            <SwaggerSpecViewer
              spec={openApiSpec}
              docExpansion="full"
              defaultModelsExpandDepth={2}
              hideInfoSection
              hideServers
              hideAuthorizeButton
            />
          ) : (
            <Alert severity="info">No API specification available for this version.</Alert>
          )}
        </Stack>
      </Stack>
    </PageLayout>

    {/* Edit Drawer */}
    <DrawerWrapper open={isEditOpen} onClose={handleDrawerClose} minWidth={520} maxWidth={700}>
      <DrawerHeader title={`Edit v${versionId} Runtime Configuration`} icon={<Edit size={24} />} onClose={handleDrawerClose} />
      <DrawerContent>
        <Form.Stack spacing={3}>
          <Form.Section>
            <Form.Subheader>Runtime Configuration</Form.Subheader>
            <RuntimeConfigEditor rows={editValues} onChange={setEditValues} />
          </Form.Section>

          <Box display="flex" justifyContent="flex-end" gap={1}>
            <Button variant="outlined" color="inherit" onClick={handleDrawerClose} disabled={isSaving}>
              Cancel
            </Button>
            <Button variant="contained" color="primary" onClick={handleSave} disabled={isSaving}>
              {isSaving ? "Saving..." : "Save Changes"}
            </Button>
          </Box>
        </Form.Stack>
      </DrawerContent>
    </DrawerWrapper>
  </>
  );
};

export default PublishVersionDetails;


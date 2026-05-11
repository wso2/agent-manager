import React, { useCallback, useMemo, useState } from "react";
import {
  Box,
  Button,
  Chip,
  CircularProgress,
  Form,
  ListingTable,
  MenuItem,
  Select,
  Typography,
} from "@wso2/oxygen-ui";
import { Plus } from "@wso2/oxygen-ui-icons-react";
import { generatePath, Link, useLocation, useNavigate, useParams } from "react-router-dom";
import { DrawerWrapper, DrawerHeader, DrawerContent, TextInput, PageLayout } from "@agent-management-platform/views";
import { absoluteRouteMap } from "@agent-management-platform/types";
import { useGetAgentBuilds } from "@agent-management-platform/api-client";
import { type BuildResponse } from "@agent-management-platform/types";
import { useConfirmationDialog } from "@agent-management-platform/shared-component";
import { DUMMY_CATALOG_LIST, getLatestVersion } from "./catalog.mock";
import { RuntimeConfigEditor, type RuntimeConfigRow } from "./RuntimeConfigEditor";

const MOCK_ITEM = DUMMY_CATALOG_LIST[0];

export const PublishedList: React.FC = () => {
  const navigate = useNavigate();
  const location = useLocation();
  const { orgId, projectId, agentId } = useParams<{
    orgId: string;
    projectId: string;
    agentId: string;
  }>();

  const listPath = generatePath(
    absoluteRouteMap.children.org.children.projects.children.agents.children.publish.path,
    { orgId: orgId ?? "", projectId: projectId ?? "", agentId: agentId ?? "" },
  );

  const createVersionPath = generatePath(
    absoluteRouteMap.children.org.children.projects.children.agents.children.publish.children.createNewVersion.path,
    { orgId: orgId ?? "", projectId: projectId ?? "", agentId: agentId ?? "" },
  );

  const isCreateOpen = location.pathname.endsWith("/create-new-version");

  // Create drawer state
  const [versionName, setVersionName] = useState("");
  const [selectedBuildName, setSelectedBuildName] = useState("");
  const [createRows, setCreateRows] = useState<RuntimeConfigRow[]>([{ key: "", type: "string", isSecrete: false }]);
  const [isCreating, setIsCreating] = useState(false);

  const { addConfirmation } = useConfirmationDialog();

  const isDirty = useMemo(
    () => versionName.trim() !== "" || selectedBuildName !== "" || createRows.some((r) => r.key.trim() !== ""),
    [versionName, selectedBuildName, createRows],
  );

  const resetCreateForm = useCallback(() => {
    setVersionName("");
    setSelectedBuildName("");
    setCreateRows([{ key: "", type: "string", isSecrete: false }]);
  }, []);

  const handleDrawerClose = useCallback(() => {
    if (isDirty) {
      addConfirmation({
        title: "Discard Changes?",
        description: "You have unsaved changes. Are you sure you want to close without saving?",
        confirmButtonText: "Discard",
        confirmButtonColor: "error",
        onConfirm: () => {
          resetCreateForm();
          navigate(listPath);
        },
      });
    } else {
      navigate(listPath);
    }
  }, [isDirty, addConfirmation, resetCreateForm, navigate, listPath]);

  const handleCreate = useCallback(() => {
    setIsCreating(true);
    // TODO: replace with real API call
    setTimeout(() => {
      setIsCreating(false);
      resetCreateForm();
      navigate(listPath);
    }, 400);
  }, [navigate, listPath, resetCreateForm]);

  const { data: buildsData, isLoading: isBuildsLoading } = useGetAgentBuilds({
    orgName: orgId,
    projName: projectId,
    agentName: agentId,
  });

  const succeededBuilds = useMemo(
    () => (buildsData?.builds ?? []).filter((b: BuildResponse) => b.status === "Succeeded"),
    [buildsData],
  );

  const versions = useMemo(
    () =>
      Object.entries(MOCK_ITEM.versions).sort(
        ([, a], [, b]) => new Date(b.releaseDate).getTime() - new Date(a.releaseDate).getTime(),
      ),
    [],
  );

  const latestVersionKey = useMemo(() => getLatestVersion(MOCK_ITEM)?.versionKey, []);

  const handleRowClick = (versionKey: string) => {
    navigate(
      generatePath(
        absoluteRouteMap.children.org.children.projects.children.agents.children.publish.children.versionDetails.path,
        { orgId: orgId ?? "", projectId: projectId ?? "", agentId: agentId ?? "", versionId: versionKey },
      ),
    );
  };

  return (
    <>
      <PageLayout
        title="Publish"
        description="Manage and publish versions of this agent kind to the catalog."
        disableIcon
        actions={
          <Button
            variant="contained"
            component={Link}
            to={createVersionPath}
            startIcon={<Plus />}
            color="primary"
          >
            Create Version
          </Button>
        }
      >
        <ListingTable.Container>
          <ListingTable>
            <ListingTable.Head>
              <ListingTable.Row>
                <ListingTable.Cell width="12%">Version</ListingTable.Cell>
                <ListingTable.Cell width="18%">Release Date</ListingTable.Cell>
                <ListingTable.Cell>Description</ListingTable.Cell>
                <ListingTable.Cell width="15%">Runtime Configuration</ListingTable.Cell>
              </ListingTable.Row>
            </ListingTable.Head>
            <ListingTable.Body>
              {versions.map(([versionKey, version]) => (
                <ListingTable.Row
                  key={versionKey}
                  hover
                  clickable
                  onClick={() => handleRowClick(versionKey)}
                >
                  <ListingTable.Cell>
                    <Typography variant="body2" fontWeight={600}>
                      v{versionKey}
                      {versionKey === latestVersionKey && (
                        <Chip
                          label="Latest"
                          size="small"
                          color="primary"
                          sx={{ ml: 1, height: 18, fontSize: "0.65rem" }}
                        />
                      )}
                    </Typography>
                  </ListingTable.Cell>
                  <ListingTable.Cell>
                    <Typography variant="body2" color="text.secondary">
                      {new Date(version.releaseDate).toLocaleDateString("en-US", {
                        year: "numeric",
                        month: "short",
                        day: "numeric",
                      })}
                    </Typography>
                  </ListingTable.Cell>
                  <ListingTable.Cell>
                    <Typography variant="body2">{MOCK_ITEM.description}</Typography>
                  </ListingTable.Cell>
                  <ListingTable.Cell>
                    <Typography variant="body2" color="text.secondary">
                      {Object.keys(version.runtimeConfig ?? {}).length} key{Object.keys(version.runtimeConfig ?? {}).length !== 1 ? "s" : ""}
                    </Typography>
                  </ListingTable.Cell>
                </ListingTable.Row>
              ))}
            </ListingTable.Body>
          </ListingTable>
        </ListingTable.Container>
      </PageLayout>

      {/* Create Version Drawer */}
      <DrawerWrapper open={isCreateOpen} onClose={handleDrawerClose} minWidth={520} maxWidth={700}>
        <DrawerHeader title="Create New Version" icon={<Plus size={24} />} onClose={handleDrawerClose} />
        <DrawerContent>
          <Form.Stack spacing={3}>
            <Form.Section>
              <Form.Subheader>Version Details</Form.Subheader>
              <Form.Stack spacing={2}>
                <Form.ElementWrapper label="Version Name" name="versionName">
                  <TextInput
                    id="versionName"
                    placeholder="e.g. 1.2.0"
                    value={versionName}
                    onChange={(e) => setVersionName(e.target.value)}
                    fullWidth
                    size="small"
                  />
                </Form.ElementWrapper>
                <Form.ElementWrapper label="Build" name="selectedBuildName">
                  <Select
                    id="selectedBuildName"
                    fullWidth
                    size="small"
                    displayEmpty
                    value={selectedBuildName}
                    onChange={(e) => setSelectedBuildName(e.target.value)}
                    disabled={isBuildsLoading}
                    renderValue={(value) => {
                      if (!value) return <Typography variant="body2" color="text.secondary">Select a build</Typography>;
                      const build = succeededBuilds.find((b: BuildResponse) => b.buildName === value);
                      return build ? build.buildName : value;
                    }}
                    endAdornment={
                      isBuildsLoading ? <CircularProgress size={16} sx={{ mr: 3 }} /> : undefined
                    }
                  >
                    {succeededBuilds.length === 0 && !isBuildsLoading && (
                      <MenuItem disabled value="">
                        <Typography variant="body2" color="text.secondary">No succeeded builds available</Typography>
                      </MenuItem>
                    )}
                    {succeededBuilds.map((build: BuildResponse) => (
                      <MenuItem key={build.buildName} value={build.buildName}>
                        <Box>
                          <Typography variant="body2" fontWeight={500}>{build.buildName}</Typography>
                          <Typography variant="caption" color="text.secondary">
                            {build.buildParameters.branch}
                            {build.buildParameters.commitId ? ` · ${build.buildParameters.commitId.slice(0, 7)}` : ""}
                            {" · "}{new Date(build.startedAt).toLocaleDateString("en-US", { year: "numeric", month: "short", day: "numeric" })}
                          </Typography>
                        </Box>
                      </MenuItem>
                    ))}
                  </Select>
                </Form.ElementWrapper>
              </Form.Stack>
            </Form.Section>

            <Form.Section>
              <Form.Subheader>Runtime Configuration</Form.Subheader>
              <RuntimeConfigEditor rows={createRows} onChange={setCreateRows} />
            </Form.Section>

            <Box display="flex" justifyContent="flex-end" gap={1}>
              <Button variant="outlined" color="inherit" onClick={handleDrawerClose} disabled={isCreating}>
                Cancel
              </Button>
              <Button
                variant="contained"
                color="primary"
                onClick={handleCreate}
                disabled={isCreating || !versionName.trim() || !selectedBuildName}
              >
                {isCreating ? "Creating..." : "Create Version"}
              </Button>
            </Box>
          </Form.Stack>
        </DrawerContent>
      </DrawerWrapper>
    </>
  );
};

export default PublishedList;

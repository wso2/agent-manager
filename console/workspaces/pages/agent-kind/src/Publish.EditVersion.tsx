import React, { useState, useCallback } from "react";
import { generatePath, useNavigate, useParams } from "react-router-dom";
import {
  Alert,
  Box,
  Button,
  Divider,
  IconButton,
  Stack,
  TextField,
  Typography,
} from "@wso2/oxygen-ui";
import { Plus, X as CloseIcon } from "@wso2/oxygen-ui-icons-react";
import { PageLayout } from "@agent-management-platform/views";
import { absoluteRouteMap } from "@agent-management-platform/types";
import { DUMMY_CATALOG_LIST } from "./catalog.mock";

const MOCK_ITEM = DUMMY_CATALOG_LIST[0];

interface EditFormValues {
  description: string;
  changes: string[];
  inputSchema: string;
  outputSchema: string;
}

function safeStringify(value: unknown): string {
  try {
    return JSON.stringify(value, null, 2);
  } catch {
    return "";
  }
}

function safeParse(value: string): Record<string, unknown> | null {
  try {
    const parsed = JSON.parse(value);
    if (typeof parsed === "object" && parsed !== null) return parsed as Record<string, unknown>;
    return null;
  } catch {
    return null;
  }
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

  const apiSpecs = version?.apiSpecs as { input?: unknown; output?: unknown } | null | undefined;

  const [values, setValues] = useState<EditFormValues>({
    description: version?.description ?? "",
    changes: version?.changes ? [...version.changes] : [""],
    inputSchema: safeStringify(apiSpecs?.input ?? {}),
    outputSchema: safeStringify(apiSpecs?.output ?? {}),
  });

  const [errors, setErrors] = useState<Partial<Record<keyof EditFormValues, string>>>({});
  const [isSaving, setIsSaving] = useState(false);

  const validate = useCallback((): boolean => {
    const next: Partial<Record<keyof EditFormValues, string>> = {};
    if (!values.description.trim()) {
      next.description = "Description is required";
    }
    if (values.inputSchema.trim() && !safeParse(values.inputSchema)) {
      next.inputSchema = "Invalid JSON";
    }
    if (values.outputSchema.trim() && !safeParse(values.outputSchema)) {
      next.outputSchema = "Invalid JSON";
    }
    setErrors(next);
    return Object.keys(next).length === 0;
  }, [values]);

  const handleSave = useCallback(() => {
    if (!validate()) return;
    setIsSaving(true);
    // TODO: replace with real API call
    setTimeout(() => {
      setIsSaving(false);
      navigate(backHref);
    }, 400);
  }, [validate, navigate, backHref]);

  const handleCancel = () => navigate(backHref);

  const updateChange = (index: number, value: string) => {
    setValues((prev) => {
      const next = [...prev.changes];
      next[index] = value;
      return { ...prev, changes: next };
    });
  };

  const addChange = () => {
    setValues((prev) => ({ ...prev, changes: [...prev.changes, ""] }));
  };

  const removeChange = (index: number) => {
    setValues((prev) => ({
      ...prev,
      changes: prev.changes.filter((_, i) => i !== index),
    }));
  };

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
      description="Update the description, changelog, and API spec for this version."
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
      <Stack spacing={4}>
        {/* Description */}
        <Stack spacing={1}>
          <Typography variant="subtitle1" fontWeight={600}>
            Description
          </Typography>
          <TextField
            placeholder="Describe this version"
            value={values.description}
            onChange={(e) => {
              setValues((prev) => ({ ...prev, description: e.target.value }));
              setErrors((prev) => ({ ...prev, description: undefined }));
            }}
            multiline
            minRows={3}
            fullWidth
            error={!!errors.description}
            helperText={errors.description}
          />
        </Stack>

        <Divider />

        {/* Changelog */}
        <Stack spacing={1.5}>
          <Typography variant="subtitle1" fontWeight={600}>
            Changelog
          </Typography>
          <Stack spacing={1}>
            {values.changes.map((change, i) => (
              <Stack key={i} direction="row" spacing={1} alignItems="center">
                <TextField
                  placeholder={`Change item ${i + 1}`}
                  value={change}
                  onChange={(e) => updateChange(i, e.target.value)}
                  fullWidth
                  size="small"
                />
                <IconButton
                  size="small"
                  onClick={() => removeChange(i)}
                  disabled={values.changes.length === 1}
                  aria-label="Remove change"
                >
                  <CloseIcon size={16} />
                </IconButton>
              </Stack>
            ))}
          </Stack>
          <Box>
            <Button
              size="small"
              variant="outlined"
              startIcon={<Plus />}
              onClick={addChange}
            >
              Add Change
            </Button>
          </Box>
        </Stack>

        <Divider />

        {/* API Spec */}
        <Stack spacing={2}>
          <Typography variant="subtitle1" fontWeight={600}>
            API Specification
          </Typography>

          <Stack spacing={1}>
            <Typography variant="body2" fontWeight={500}>
              Input Schema (JSON)
            </Typography>
            <TextField
              value={values.inputSchema}
              onChange={(e) => {
                setValues((prev) => ({ ...prev, inputSchema: e.target.value }));
                setErrors((prev) => ({ ...prev, inputSchema: undefined }));
              }}
              multiline
              minRows={6}
              fullWidth
              error={!!errors.inputSchema}
              helperText={errors.inputSchema ?? "OpenAPI-compatible JSON schema for the request body"}
              inputProps={{ style: { fontFamily: "monospace", fontSize: "0.8rem" } }}
            />
          </Stack>

          <Stack spacing={1}>
            <Typography variant="body2" fontWeight={500}>
              Output Schema (JSON)
            </Typography>
            <TextField
              value={values.outputSchema}
              onChange={(e) => {
                setValues((prev) => ({ ...prev, outputSchema: e.target.value }));
                setErrors((prev) => ({ ...prev, outputSchema: undefined }));
              }}
              multiline
              minRows={6}
              fullWidth
              error={!!errors.outputSchema}
              helperText={errors.outputSchema ?? "OpenAPI-compatible JSON schema for the response body"}
              inputProps={{ style: { fontFamily: "monospace", fontSize: "0.8rem" } }}
            />
          </Stack>
        </Stack>

      </Stack>
    </PageLayout>
  );
};

export default PublishEditVersion;

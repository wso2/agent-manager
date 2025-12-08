import {
  Paperclip as AttachFile,
  CheckCircle,
  Circle,
  Settings,
} from "@wso2/oxygen-ui-icons-react";
import {
  Alert,
  Box,
  Button,
  Card,
  CardContent,
  Collapse,
  Divider,
  Typography,
  useTheme,
} from "@wso2/oxygen-ui";
import { useCallback, useRef } from "react";
import { useFormContext, useWatch } from "react-hook-form";
import { TextInput } from "@agent-management-platform/views";

const inputInterfaces = [
  {
    label: "Chat Agent",
    description: "Interactive chat agent following the interface specification",
    default: true,
    value: "DEFAULT",
    icon: <CheckCircle />,
  },
  {
    label: "Agent API",
    description:
      "Agent exposed as an API, with a user-specified OpenAPI specification and port configuration.",
    default: false,
    value: "CUSTOM",
    icon: <Settings />,
  },
];

export const InputInterface = () => {
  const {
    setValue,
    control,
    register,
    formState: { errors },
  } = useFormContext();
  const interfaceType =
    useWatch({ control, name: "interfaceType" }) || "DEFAULT";
  const port = useWatch({ control, name: "port" }) as unknown as string;
  const openApiFileName = useWatch({
    control,
    name: "openApiFileName",
  }) as string;
  const fileInputRef = useRef<HTMLInputElement | null>(null);
  const theme = useTheme();
  const handleSelect = useCallback(
    (value: string) => {
      setValue("interfaceType", value, { shouldValidate: true });
      if (value === "DEFAULT") {
        setValue("openApiFileName", "", { shouldValidate: true });
        setValue("openApiContent", "", { shouldValidate: true });
        setValue("port", "" as unknown as number, { shouldValidate: true });
        setValue("basePath", "/", { shouldValidate: true });
      }
    },
    [setValue]
  );

  const handlePortChange = useCallback(
    (e: React.ChangeEvent<HTMLInputElement>) => {
      const next = e.target.value;
      if (/^\d*$/.test(next)) {
        setValue(
          "port",
          next === "" ? ("" as unknown as number) : Number(next),
          { shouldValidate: true }
        );
      }
    },
    [setValue]
  );

  const handleFilePick = useCallback(
    (e: React.ChangeEvent<HTMLInputElement>) => {
      const file = e.target.files?.[0];
      if (!file) return;

      // Validate file size (max 2MB)
      const MAX_FILE_SIZE = 2 * 1024 * 1024; // 2MB
      if (file.size > MAX_FILE_SIZE) {
        alert("File size exceeds 2MB. Please upload a smaller file.");
        e.target.value = ""; // Reset file input
        return;
      }

      // Validate file extension
      if (!file.name.match(/\.(yaml|yml)$/i)) {
        alert("Please upload a YAML file (.yaml or .yml)");
        e.target.value = "";
        return;
      }

      setValue("openApiFileName", file.name, { shouldValidate: true });
      const reader = new FileReader();
      reader.onload = () => {
        const text = typeof reader.result === "string" ? reader.result : "";
        setValue("openApiContent", text, { shouldValidate: true });
      };
      reader.onerror = () => {
        alert("Failed to read file. Please try again.");
        setValue("openApiFileName", "");
      };
      reader.readAsText(file);
    },
    [setValue]
  );

  return (
    <Card variant="outlined">
      <CardContent sx={{ gap: 1, display: "flex", flexDirection: "column" }}>
        <Typography variant="h5">Agent Type</Typography>

        <Typography variant="body2" color="text.secondary">
          How your agent receives requests
        </Typography>
        <Box display="flex" flexDirection="column" gap={1}>
          <Box display="flex" flexDirection="row" gap={1}>
            {inputInterfaces.map((inputInterface) => (
              <Card
                key={inputInterface.value}
                variant="outlined"
                onClick={() => handleSelect(inputInterface.value)}
                sx={{
                  maxWidth: 500,
                  cursor: "pointer",
                  flexGrow: 1,
                  transition: theme.transitions.create([
                    "background-color",
                    "border-color",
                  ]),
                  "&.MuiCard-root": {
                    backgroundColor:
                      interfaceType === inputInterface.value
                        ? "background.default"
                        : "action.paper",
                    borderColor:
                      interfaceType === inputInterface.value
                        ? "primary.main"
                        : "divider",
                    "&:hover": {
                      backgroundColor: "background.default",
                      borderColor: "primary.main",
                    },
                  },
                }}
              >
                <CardContent sx={{ height: "100%" }}>
                  <Box
                    display="flex"
                    flexDirection="row"
                    alignItems="center"
                    height="100%"
                    gap={1}
                  >
                    <Box
               
                    >
                      {interfaceType === inputInterface.value ? (
                        <CheckCircle size={16} />
                      ) : (
                        <Circle size={16} />
                      )}
                    </Box>
                    <Divider orientation="vertical" flexItem />
                    <Box>
                      <Typography variant="h6">
                        {inputInterface.label}
                      </Typography>
                      <Typography variant="caption">
                        {inputInterface.description}
                      </Typography>
                    </Box>
                  </Box>
                </CardContent>
              </Card>
            ))}
          </Box>
          <Collapse in={interfaceType === "DEFAULT"}>
            <Typography variant="body2" color="text.secondary">
              <Alert severity="info">
                /chat (string message, string session_id, context: JSON) →
                string reply Runs on port 8080.
              </Alert>
            </Typography>
          </Collapse>
          <Collapse in={interfaceType === "CUSTOM"}>
            <Box display="flex" flexDirection="column" gap={1}>
              <Box display="flex" flexDirection="row" gap={1}>
                <Box display="flex" flexDirection="column" flexGrow={1}>
                  <TextInput
                    label="OpenAPI Spec"
                    placeholder="openapi.yaml"
                    value={openApiFileName || ""}
                    fullWidth
                    size="small"
                    slotProps={{ input: { readOnly: true } }}
                    error={!!errors.openApiFileName || !!errors.openApiContent}
                    helperText={
                      (errors.openApiFileName?.message as string) ||
                      (errors.openApiContent?.message as string) ||
                      (openApiFileName
                        ? "File loaded in browser"
                        : "Upload your OpenAPI YAML file")
                    }
                  />
                  <Box pt={1}>
                    <Button
                      variant="outlined"
                      startIcon={<AttachFile size={16} />}
                      onClick={() => fileInputRef.current?.click()}
                    >
                      Choose File
                    </Button>
                  </Box>
                  <input
                    ref={fileInputRef}
                    type="file"
                    accept=".yaml,.yml,text/yaml,application/x-yaml,application/yaml"
                    style={{ display: "none" }}
                    onChange={handleFilePick}
                  />
                </Box>
                <Box>
                  <TextInput
                    label="Port"
                    placeholder="8080"
                    required
                    value={port}
                    onChange={handlePortChange}
                    size="small"
                    type="number"
                    error={!!errors.port}
                    helperText={
                      (errors.port?.message as string) ||
                      (port ? undefined : "Port is required")
                    }
                  />
                </Box>
              </Box>
              <Box>
                <TextInput
                  label="Base Path"
                  placeholder="/"
                  required
                  fullWidth
                  size="small"
                  error={!!errors.basePath}
                  helperText={
                    (errors.basePath?.message as string) ||
                    "API base path (e.g., / or /api/v1)"
                  }
                  {...register("basePath")}
                />
              </Box>
            </Box>
          </Collapse>
        </Box>
      </CardContent>
    </Card>
  );
};

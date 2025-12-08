import { Box, Card, CardContent, Typography } from "@wso2/oxygen-ui";
import { useFormContext } from "react-hook-form";
import { useEffect, useRef } from "react";
import { TextInput } from "@agent-management-platform/views";

// Generate a random 6-character string
const generateRandomString = (length: number = 6): string => {
  const chars = "abcdefghijklmnopqrstuvwxyz0123456789";
  let result = "";
  for (let i = 0; i < length; i++) {
    result += chars.charAt(Math.floor(Math.random() * chars.length));
  }
  return result;
};

// Convert display name to URL-friendly format
const sanitizeNameForUrl = (displayName: string): string => {
  return displayName
    .toLowerCase()
    .trim()
    .replace(/[^a-z0-9\s-]/g, "") // Remove special characters
    .replace(/\s+/g, "-") // Replace spaces with hyphens
    .replace(/-+/g, "-") // Replace multiple hyphens with single hyphen
    .replace(/^-|-$/g, ""); // Remove leading/trailing hyphens
};

export const SourceAndConfiguration = () => {
  const {
    register,
    formState: { errors },
    watch,
    setValue,
  } = useFormContext();
  const isNameManuallyEdited = useRef(false);
  const randomSuffix = useRef<string>("");
  const displayName = watch("displayName");

  // Generate random suffix once
  if (!randomSuffix.current) {
    randomSuffix.current = generateRandomString(6);
  }

  // Auto-generate name from display name
  useEffect(() => {
    if (displayName && !isNameManuallyEdited.current) {
      const sanitizedName = sanitizeNameForUrl(displayName);
      if (sanitizedName) {
        const generatedName = `${sanitizedName}-${randomSuffix.current}`;
        setValue("name", generatedName, {
          shouldValidate: true,
          shouldDirty: true,
          shouldTouch: false,
        });
      }
    } else if (!displayName && !isNameManuallyEdited.current) {
      // Clear the name field if display name is empty
      setValue("name", "", {
        shouldValidate: true,
        shouldDirty: true,
        shouldTouch: false,
      });
    }
  }, [displayName, setValue]);

  return (
    <>
      <Card variant="outlined">
        <CardContent sx={{ gap: 1, display: "flex", flexDirection: "column" }}>
          <Typography variant="h5">Agent Details</Typography>
          <Box display="flex" flexDirection="column" gap={1}>
            <TextInput
              placeholder="e.g., Customer Support Agent"
              label="Name"
              fullWidth
              size="small"
              error={!!errors.displayName}
              helperText={
                (errors.displayName?.message as string) ||
                "A name for your agent"
              }
              {...register("displayName")}
            />
            <TextInput
              placeholder="Short description of what this agent does"
              label="Description (optional)"
              fullWidth
              size="small"
              multiline
              minRows={2}
              maxRows={6}
              error={!!errors.description}
              helperText={errors.description?.message as string}
              {...register("description")}
            />
          </Box>
        </CardContent>
      </Card>
      <Card variant="outlined">
        <CardContent sx={{ gap: 1, display: "flex", flexDirection: "column" }}>
          <Typography variant="h5">Repository Details</Typography>
          <Box display="flex" flexDirection="column" gap={1}>
            <TextInput
              placeholder="https://github.com/username/repo"
              label="GitHub Repository"
              fullWidth
              size="small"
              error={!!errors.repositoryUrl}
              helperText={errors.repositoryUrl?.message as string}
              {...register("repositoryUrl")}
            />

            <Box display="flex" flexDirection="row" gap={1}>
              <TextInput
                placeholder="main"
                label="Branch"
                fullWidth
                size="small"
                error={!!errors.branch}
                helperText={errors.branch?.message as string}
                {...register("branch")}
              />
              <TextInput
                placeholder="my-agent"
                label="Project Path"
                fullWidth
                size="small"
                error={!!errors.appPath}
                helperText={errors.appPath?.message as string}
                {...register("appPath")}
              />
            </Box>
          </Box>
        </CardContent>
      </Card>
      <Card variant="outlined">
        <CardContent sx={{ gap: 1, display: "flex", flexDirection: "column" }}>
          <Typography variant="h5">Build Details</Typography>
          <Box display="flex" flexDirection="column" gap={1}>
            <Box display="flex" flexDirection="row" gap={1}>
              <TextInput
                placeholder="python"
                disabled
                label="Language"
                fullWidth
                size="small"
                error={!!errors.language}
                helperText={
                  (errors.language?.message as string) ||
                  "e.g., python, nodejs, go"
                }
                {...register("language")}
              />
              <TextInput
                placeholder="3.11"
                label="Language Version"
                fullWidth
                size="small"
                error={!!errors.languageVersion}
                helperText={
                  (errors.languageVersion?.message as string) ||
                  "e.g., 3.11, 20, 1.21"
                }
                {...register("languageVersion")}
              />
            </Box>

            <TextInput
              placeholder="python main.py"
              label="Start Command"
              fullWidth
              size="small"
              error={!!errors.runCommand}
              helperText={
                (errors.runCommand?.message as string) ||
                "Dependencies auto-install from package.json, requirements.txt, or pyproject.toml"
              }
              {...register("runCommand")}
            />
          </Box>
        </CardContent>
      </Card>
    </>
  );
};

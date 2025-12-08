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

export const ConnectAgentForm = () => {
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
        const generatedName = `${sanitizedName.substring(0, 10)}-${randomSuffix.current}`;
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
    <Box display="flex" flexDirection="column" gap={2} flexGrow={1}>
      <Card variant="outlined" sx={{ "& .MuiCardContent-root": { backgroundColor: "background.paper" } }}>
        <CardContent sx={{ gap: 1, display: "flex", flexDirection: "column" }}>
          <Box display="flex" flexDirection="column" gap={1}>
            <Typography variant="h5">Agent Details</Typography>
          </Box>
          <Box display="flex" flexDirection="column" gap={1}>
            <TextInput
              placeholder="e.g., Customer Support"
              label="Name"
              size="small"
              fullWidth
              error={!!errors.displayName}
              helperText={
                (errors.displayName?.message as string)
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
    </Box>
  );
};

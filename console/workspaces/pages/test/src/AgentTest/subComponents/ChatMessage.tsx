import { useGetAgent } from "@agent-management-platform/api-client";
import { Box, Card, CardContent, Typography } from "@wso2/oxygen-ui";
import { useParams } from "react-router-dom";

interface ChatMessageProps {
  id: string;
  role: "user" | "assistant";
  content: string;
}

export function ChatMessage({ role, content }: ChatMessageProps) {
  const { orgId, projectId, agentId } = useParams();
  const { data: agent } = useGetAgent({
    orgName: orgId ?? "default",
    projName: projectId ?? "default",
    agentName: agentId ?? "default",
  });

  return (
    <Box
      display="flex"
      justifyContent={role === "user" ? "flex-end" : "flex-start"}
      width="100%"
      sx={{ mb: 0.5 }}
    >
      <Box
        display="flex"
        gap={1.5}
        maxWidth="75%"
        flexDirection={role === "user" ? "row-reverse" : "row"}
        alignItems="flex-start"
      >
        <Card
          variant="outlined"
          sx={{
            borderBottomLeftRadius: role !== "user" ? 0 : 16,
            borderBottomRightRadius: role === "user" ? 0 : 16,
            "& .MuiCardContent-root": {
              minWidth: 300,
              backgroundColor:
                role === "user" ? "secondary.dark" : "background.paper",
            },
          }}
        >
          <CardContent>
            {role !== "user" && (
              <Typography variant="caption">{agent?.displayName}</Typography>
            )}
            {role === "user" && (
              <Typography variant="caption" color="primary.contrastText">
                You
              </Typography>
            )}
            <Typography
              variant="h6"
              color={role === "user" ? "primary.contrastText" : "text.primary"}
            >
              {content}
            </Typography>
          </CardContent>
        </Card>
      </Box>
    </Box>
  );
}

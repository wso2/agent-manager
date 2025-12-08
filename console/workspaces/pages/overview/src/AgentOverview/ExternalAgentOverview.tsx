import { Box, Typography, Button } from "@wso2/oxygen-ui";
import { Clock as AccessTime, Settings } from "@wso2/oxygen-ui-icons-react";
import { useState } from "react";
import { useParams } from "react-router-dom";
import dayjs from "dayjs";
import { useGetAgent } from "@agent-management-platform/api-client";
import { EnvironmentCard } from "@agent-management-platform/shared-component";
import { InstrumentationDrawer } from "./InstrumentationDrawer";

export const ExternalAgentOverview = () => {
  const { agentId, orgId, projectId } = useParams();
  const [isInstrumentationDrawerOpen, setIsInstrumentationDrawerOpen] =
    useState(false);

  const { data: agent } = useGetAgent({
    orgName: orgId ?? "default",
    projName: projectId ?? "default",
    agentName: agentId ?? "",
  });

  // Sample instrumentation config - these would come from props or API
  const instrumentationUrl = "http://localhost:21893";
  const apiKey = "00000000-0000-0000-0000-000000000000";

  return (
    <>
      <Box display="flex" flexDirection="column" pb={4} gap={1}>
        <Box
          sx={{
            maxWidth: "fit-content",
            gap: 1.5,
            display: "flex",
            flexDirection: "column",
            width: "50%",
          }}
        >
          <Box display="flex" flexDirection="row" gap={1} alignItems="center">
            <Typography variant="body2">Created</Typography>
            <AccessTime size={16} />
            <Typography variant="body2">
              {dayjs(agent?.createdAt).fromNow()}
            </Typography>
          </Box>
        </Box>
        <EnvironmentCard
          external
          orgId={orgId ?? "default"}
          projectId={projectId ?? "default"}
          agentId={agentId ?? "default"}
          actions={
            <Button
              variant="text"
              size="small"
              startIcon={<Settings size={16} />}
              onClick={() => setIsInstrumentationDrawerOpen(true)}
            >
              Setup Agent
            </Button>
          }
        />
      </Box>
      <InstrumentationDrawer
        open={isInstrumentationDrawerOpen}
        onClose={() => setIsInstrumentationDrawerOpen(false)}
        agentId={agentId ?? ""}
        instrumentationUrl={instrumentationUrl}
        apiKey={apiKey}
      />
    </>
  );
};

import { Box, Divider } from "@wso2/oxygen-ui";
import { GitBranch as Timeline } from "@wso2/oxygen-ui-icons-react";
import { Span } from "@agent-management-platform/types";
import { DrawerHeader, DrawerContent } from "@agent-management-platform/views";
import { BasicInfoSection } from "./spanDetails/BasicInfoSection";
import { TimingSection } from "./spanDetails/TimingSection";
import { StatusSection } from "./spanDetails/StatusSection";
import { AttributesSection } from "./spanDetails/AttributesSection";

interface SpanDetailsPanelProps {
  span: Span | null;
  onClose: () => void;
}

export function SpanDetailsPanel({ span, onClose }: SpanDetailsPanelProps) {
  if (!span) {
    return null;
  }

  return (
    <>
      <DrawerHeader
        icon={<Timeline size={24} />}
        title="Span Details"
        onClose={onClose}
      />
      <DrawerContent>
        <Box
          sx={{
            overflowY: "auto",
            gap: 1,
            overflowX: "visible",
            display: "flex",
            flexDirection: "column",
            height: "calc(100vh - 80px)",
          }}
        >
          <BasicInfoSection span={span} />
          <Divider />
          <TimingSection span={span} />
          <Divider />
          <StatusSection span={span} />
          {span.attributes && Object.keys(span.attributes).length > 0 && (
            <>
              <Divider />
              <AttributesSection attributes={span.attributes} />
            </>
          )}
        </Box>
      </DrawerContent>
    </>
  );
}

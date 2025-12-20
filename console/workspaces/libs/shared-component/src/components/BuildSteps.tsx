/**
 * Copyright (c) 2025, WSO2 LLC. (https://www.wso2.com).
 *
 * WSO2 LLC. licenses this file to you under the Apache License,
 * Version 2.0 (the "License"); you may not use this file except
 * in compliance with the License.
 * You may obtain a copy of the License at
 *
 * http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing,
 * software distributed under the License is distributed on an
 * "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
 * KIND, either express or implied.  See the License for the
 * specific language governing permissions and limitations
 * under the License.
 */

import {
  BuildDetailsResponse,
  BuildStep,
} from "@agent-management-platform/types";
import { alpha, CircularProgress, Stack, Typography, useTheme } from "@wso2/oxygen-ui";
import { Check, Clock, X } from "@wso2/oxygen-ui-icons-react";
import dayjs from "dayjs";
import relativeTime from "dayjs/plugin/relativeTime";

dayjs.extend(relativeTime);

const getIcon = (step: BuildStep) => {
  switch (step.status) {
    case "Succeeded":
      return <Check size={16} />;
    case "Failed":
      return <X size={16} />;
    case "Running":
      return <CircularProgress size={16} color="inherit" />;
    default:
      return <Clock size={16} />;
  }
};

export interface BuildStepsProps {
  build: BuildDetailsResponse;
}

const getDisplayName = (step: BuildStep) => {
  switch (step.type) {
    case "BuildCompleted":
      return "Build Image";
    case "BuildInitiated":
      return "Initiated";
    case "BuildTriggered":
      return "Triggered";
    case "BuildRunning":
      return "Build Running";
    case "WorkloadUpdated":
      return "Workload Updated";
    default:
      return step.type;
  }
};

function BuildStepItem(props: { step: BuildStep; index: number }) {
  const { step, index } = props;
  const theme = useTheme();
  const getColor = () => {
    if (step.status === "Running") {
      return theme.palette.warning.main;
    }
    if (step.status === "Succeeded") {
      return theme.palette.success.main;
    }
    if (step.status === "Failed") {
      return theme.palette.error.main;
    }
    return theme.palette.primary.main;
  };
  return (
    <Stack
      direction="row"
      gap={1}
      sx={{
        backgroundColor: alpha(getColor(), 0.1),
        paddingX: 1,
        paddingY: 0.5,
        borderRadius: 1,
        alignItems: "center",
        justifyContent: "flex-start",
      }}
    >
      <Stack color={getColor()}>{getIcon(step)}</Stack>
      <Typography variant="caption">
        {index + 1}.
      </Typography>
      <Typography variant="body1">
        {getDisplayName(step)}
      </Typography>
    </Stack>
  );
}

export function BuildSteps(props: BuildStepsProps) {
  const { build } = props;
  return (
    <Stack spacing={1}>
      {build.steps?.map((step, index) => (
        <BuildStepItem step={step} key={`${step.type}-${index}`} index={index} />
      ))}
    </Stack>
  );
}

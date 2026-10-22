/**
 * Copyright (c) 2026, WSO2 LLC. (https://www.wso2.com).
 *
 * WSO2 LLC. licenses this file to you under the Apache License,
 * Version 2.0 (the "License"); you may not use this file except
 * in compliance with the License. You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing,
 * software distributed under the License is distributed on an
 * "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
 * KIND, either express or implied. See the License for the
 * specific language governing permissions and limitations
 * under the License.
 */

import { Box, Chip, Stack, Typography } from "@wso2/oxygen-ui";
import type { ParsedA2ASkill } from "../utils/a2aCard";

export interface A2ASkillItemProps {
  skill: ParsedA2ASkill;
}

interface ModeRowProps {
  label: string;
  modes: string[];
}

function ModeRow({ label, modes }: ModeRowProps) {
  if (modes.length === 0) {
    return null;
  }
  return (
    <Stack direction="row" spacing={0.5} alignItems="center" flexWrap="wrap" useFlexGap>
      <Typography variant="caption" color="text.secondary">
        {label}
      </Typography>
      {modes.map((mode, index) => (
        <Chip key={`${index}-${mode}`} label={mode} size="small" variant="outlined" />
      ))}
    </Stack>
  );
}

export function A2ASkillItem({ skill }: A2ASkillItemProps) {
  return (
    <Box sx={{ p: 1.5, border: 1, borderColor: "divider", borderRadius: 1 }}>
      <Stack spacing={0.5}>
        <Typography variant="subtitle2">{skill.name}</Typography>
        {skill.description && (
          <Typography variant="body2" color="text.secondary">
            {skill.description}
          </Typography>
        )}
        {skill.tags.length > 0 && (
          <Stack direction="row" spacing={0.5} flexWrap="wrap" useFlexGap>
            {skill.tags.map((tag, index) => (
              <Chip key={`${index}-${tag}`} label={tag} size="small" variant="outlined" />
            ))}
          </Stack>
        )}
        <ModeRow label="Input" modes={skill.inputModes} />
        <ModeRow label="Output" modes={skill.outputModes} />
        {skill.examples.map((example, index) => (
          <Typography key={`${index}-${example}`} variant="caption" color="text.secondary">
            e.g. {example}
          </Typography>
        ))}
      </Stack>
    </Box>
  );
}

/**
 * Copyright (c) 2026, WSO2 LLC. (https://www.wso2.com).
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

import { Box, Chip, Stack, Typography } from "@wso2/oxygen-ui";
import type {
  AgentRegistryA2ASkill,
  AgentRegistryEntryDetail,
  AgentRegistryEnvironmentDetail,
} from "@agent-management-platform/types";
import { SectionCard } from "./SectionCard";

/**
 * One skill: its name, description and tags in a bordered tile, as the agent's
 * Configure → A2A panel shows skills (A2AAgentCardPanel).
 */
function SkillTile({ skill }: { skill: AgentRegistryA2ASkill }) {
  return (
    <Box sx={{ p: 2, border: 1, borderColor: "divider", borderRadius: 1 }}>
      <Typography variant="subtitle2">{skill.name}</Typography>
      {skill.description && (
        <Typography variant="body2" color="text.secondary" sx={{ mt: 0.5 }}>
          {skill.description}
        </Typography>
      )}
      {skill.tags && skill.tags.length > 0 && (
        <Box display="flex" gap={0.75} flexWrap="wrap" sx={{ mt: 1.5 }}>
          {skill.tags.map((tag) => (
            <Chip key={tag} size="small" variant="outlined" label={tag} />
          ))}
        </Box>
      )}
    </Box>
  );
}

/**
 * What an A2A agent declares it can do: the skills on its A2A card in the environment
 * chosen at the top (each environment's card comes from the version deployed there).
 * Only A2A agents have this today, so other agents get no card at all.
 */
export function Capabilities({
  agent,
  env,
}: {
  agent: AgentRegistryEntryDetail;
  env: AgentRegistryEnvironmentDetail;
}) {
  if (agent.interface !== "a2a-agent") return null;
  const skills = env.a2aCard?.skills ?? [];

  return (
    <SectionCard title="Capabilities">
      {skills.length === 0 ? (
        <Typography variant="body2" color="text.secondary">
          No A2A card has been fetched here yet, so no skills are known.
        </Typography>
      ) : (
        <Stack spacing={1.5}>
          {skills.map((skill) => (
            <SkillTile key={skill.id} skill={skill} />
          ))}
        </Stack>
      )}
    </SectionCard>
  );
}

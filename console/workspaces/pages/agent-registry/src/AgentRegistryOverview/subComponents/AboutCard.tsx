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

import { useState } from "react";
import { Box, Button, Chip, Divider, Stack, Typography } from "@wso2/oxygen-ui";
import type {
  AgentRegistryA2ASkill,
  AgentRegistryEntryDetail,
  AgentRegistryEnvironmentDetail,
} from "@agent-management-platform/types";
import { ClampedText, SectionCard } from "../../AgentPage/AgentPageParts";

/** Skills shown before Show all. */
const SKILLS_SHOWN = 3;

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
 * What the agent is and what it can do, in one card: its description, then for an A2A agent
 * the skills on its A2A card in the environment chosen at the top (each environment's card
 * comes from the version deployed there).
 */
export function AboutCard({
  agent,
  env,
}: {
  agent: AgentRegistryEntryDetail;
  env: AgentRegistryEnvironmentDetail | undefined;
}) {
  const [showAll, setShowAll] = useState(false);
  const isA2A = agent.interface === "a2a-agent";
  const skills = env?.a2aCard?.skills ?? [];
  const shown = showAll ? skills : skills.slice(0, SKILLS_SHOWN);

  return (
    <SectionCard title="About">
      <Stack spacing={2.5} divider={isA2A ? <Divider flexItem /> : undefined}>
        {agent.description ? (
          <ClampedText text={agent.description} />
        ) : (
          <Typography variant="body2" color="text.secondary">
            No description
          </Typography>
        )}
        {isA2A && (
          <Box>
            <Typography variant="subtitle2" sx={{ mb: 1.25 }}>
              Skills
            </Typography>
            {skills.length === 0 ? (
              <Typography variant="body2" color="text.secondary">
                No A2A card has been fetched here yet, so no skills are known.
              </Typography>
            ) : (
              <Stack spacing={1.5}>
                {shown.map((skill) => (
                  <SkillTile key={skill.id} skill={skill} />
                ))}
                {skills.length > SKILLS_SHOWN && (
                  <Box>
                    <Button
                      size="small"
                      variant="text"
                      sx={{ px: 0 }}
                      onClick={() => setShowAll(!showAll)}
                    >
                      {showAll ? "Show fewer" : `Show all ${skills.length} skills`}
                    </Button>
                  </Box>
                )}
              </Stack>
            )}
          </Box>
        )}
      </Stack>
    </SectionCard>
  );
}

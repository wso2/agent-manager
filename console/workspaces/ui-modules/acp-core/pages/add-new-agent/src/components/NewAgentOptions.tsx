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

import { useCallback, useEffect, useRef } from "react";
import { Box, Skeleton } from "@wso2/oxygen-ui";
import { Bot } from "@wso2/oxygen-ui-icons-react";
import { generatePath, Navigate, useNavigate, useParams } from "react-router-dom";
import {
    PageLayout,
    StateNotice,
    type AgentTypeOption,
} from "@agent-management-platform/views";
import {
    useAgentTypePicker,
    useAgentWizardReturn,
} from "@agent-management-platform/shared-component";
import { ConsoleAction, useTrack } from "@agent-management-platform/api-client";
import { NewAgentTypeCard } from "./NewAgentTypeCard";

/**
 * The "Add a New Agent" page. The agent types are not defined here: each comes
 * from the UI module that can create that kind of agent (an `agent-type`
 * contribution), and its card leads to that module's create form. Modules can
 * also add content below the cards (a `panel` at addNewAgentOptions).
 *
 * With a single agent type and nothing else to show there is no choice to
 * make, so the page goes straight to that type's form (see useAgentTypePicker).
 */
export const NewAgentOptions = () => {
    const navigate = useNavigate();
    const { track } = useTrack();
    // Always offer a way out, even for a project with no agents yet.
    const { returnTo, wizardState } = useAgentWizardReturn();
    const { orgId, projectId } = useParams<{ orgId: string; projectId?: string }>();
    const { agentTypes, isPending, extraContent, soleAgentType } = useAgentTypePicker();

    const pathOf = useCallback((agentType: AgentTypeOption) => generatePath(agentType.route.path, {
        orgId: orgId ?? "",
        projectId: projectId ?? "default",
    }), [orgId, projectId]);

    // The wizard steps this choice stands for (see wizardSteps), in order.
    const trackSteps = useCallback((agentType: AgentTypeOption) => {
        (agentType.wizardSteps ?? [agentType.id]).forEach((step, index) => {
            track(ConsoleAction.WizardStepViewed, {
                wizard: "create-agent",
                step,
                step_index: index + 1,
            });
        });
    }, [track]);

    const handleSelect = useCallback((agentType: AgentTypeOption) => {
        trackSteps(agentType);
        navigate(pathOf(agentType), { state: wizardState });
    }, [navigate, pathOf, trackSteps, wizardState]);

    // Skipping the page still records the steps it stood for, once: keyed on
    // the type, not the object, which is rebuilt whenever contributions change.
    const trackedSkipRef = useRef<string | null>(null);
    useEffect(() => {
        if (!soleAgentType || trackedSkipRef.current === soleAgentType.key) return;
        trackedSkipRef.current = soleAgentType.key;
        trackSteps(soleAgentType);
    }, [soleAgentType, trackSteps]);

    if (soleAgentType) {
        // Replace, so the browser's back button skips this page too.
        return <Navigate to={pathOf(soleAgentType)} state={wizardState} replace />;
    }

    return (
        <PageLayout
            title="Add a New Agent"
            description="Choose how you want to get started."
            disableIcon
            backHref={returnTo}
            backLabel="Back"
        >
            <Box display="flex" flexDirection="column" gap={3} width={1}>
                {isPending ? (
                    // A type may still activate: hold off, so neither the cards
                    // nor the empty state flash before a redirect.
                    <Skeleton variant="rounded" height={330} sx={{ width: { xs: "100%", sm: 450 } }} />
                ) : agentTypes.length === 0 ? (
                    <StateNotice
                        icon={Bot}
                        title="No agent types available"
                        description="Agent types come from UI modules such as AI Gateway and Agent Runtime. Turn one on to create agents."
                        contained
                    />
                ) : (
                    <Box display="flex" flexDirection={{ xs: "column", sm: "row" }} flexWrap="wrap" gap={3}>
                        {agentTypes.map((agentType) => (
                            <NewAgentTypeCard
                                key={agentType.key}
                                title={agentType.title}
                                subheader={agentType.description}
                                icon={<agentType.illustration width={150} height={160} />}
                                ctaLabel={agentType.ctaLabel}
                                onClick={() => handleSelect(agentType)}
                            />
                        ))}
                    </Box>
                )}
                {extraContent.map(({ moduleName, component: Content }) => (
                    <Content key={moduleName} />
                ))}
            </Box>
        </PageLayout>
    );
};

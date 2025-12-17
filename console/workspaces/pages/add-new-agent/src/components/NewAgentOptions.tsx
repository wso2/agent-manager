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

import { Box } from "@wso2/oxygen-ui";
import { NewAgentTypeCard } from "./NewAgentTypeCard";
import { ImageList } from "@agent-management-platform/views";

interface NewAgentOptionsProps {
    onSelect: (option: 'new' | 'existing') => void;
}

export const NewAgentOptions = ({ onSelect }: NewAgentOptionsProps) => {
    const handleSelect = (type: string) => {
        onSelect(type as 'new' | 'existing');
    };

    return (
        <Box display="flex" flexDirection="row" gap={3} width={1}>
            <NewAgentTypeCard
                type="existing"
                title="Externally-Hosted Agent"
                subheader="Connect an existing agent running outside the platform and enable observability and governance."
                icon={<img src={ImageList.EXTERNAL_AGENT} width={150} alt="External Agent" />}
                onClick={handleSelect}
            />
            <NewAgentTypeCard
                type="new"
                title="Platform-Hosted Agent"
                subheader="Deploy and manage agents with full lifecycle support, including built-in CI/CD, scaling, observability, and governance."
                icon={<img src={ImageList.INTERNAL_AGENT} width={150} alt="Internal Agent" />}
                onClick={handleSelect}
            />
        </Box>
    );
};

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

import { Box, List, ListItem, ListItemIcon, ListItemText, Chip } from "@mui/material";
import { RocketLaunch, Link as LinkIcon, CheckCircleOutline } from "@mui/icons-material";
import { NewAgentTypeCard } from "./NewAgentTypeCard";

interface NewAgentOptionsProps {
    onSelect: (option: 'new' | 'existing') => void;
}

export const NewAgentOptions = ({ onSelect }: NewAgentOptionsProps) => {
    const handleSelect = (type: string) => {
        onSelect(type as 'new' | 'existing');
    };

    return (
        <Box display="flex" flexDirection="row" gap={3} pt={2} width={1}>
                <Box flex={1}>
                    <NewAgentTypeCard
                        type="new"
                        title="Deploy New Agent"
                        subheader="Build and deploy your AI agent from a GitHub repository"
                        icon={<RocketLaunch sx={{ fontSize: 40 }} />}
                        onClick={handleSelect}
                        content={
                            <Box>
                                <List dense disablePadding>
                                    <ListItem disablePadding sx={{ mb: 1 }}>
                                        <ListItemIcon sx={{ minWidth: 32 }}>
                                            <CheckCircleOutline fontSize="small" color="success" />
                                        </ListItemIcon>
                                        <ListItemText
                                            primary="Automatic build and deployment"
                                        />
                                    </ListItem>
                                    <ListItem disablePadding sx={{ mb: 1 }}>
                                        <ListItemIcon sx={{ minWidth: 32 }}>
                                            <CheckCircleOutline fontSize="small" color="success" />
                                        </ListItemIcon>
                                        <ListItemText
                                            primary="Connect GitHub repository"
                                        />
                                    </ListItem>
                                    <ListItem disablePadding sx={{ mb: 1 }}>
                                        <ListItemIcon sx={{ minWidth: 32 }}>
                                            <CheckCircleOutline fontSize="small" color="success" />
                                        </ListItemIcon>
                                        <ListItemText
                                            primary="Built-in observability and monitoring"
                                        />
                                    </ListItem>
                                    <ListItem disablePadding>
                                        <ListItemIcon sx={{ minWidth: 32 }}>
                                            <CheckCircleOutline fontSize="small" color="success" />
                                        </ListItemIcon>
                                        <ListItemText
                                            primary="Environment management included"
                                        />
                                    </ListItem>
                                </List>
                                <Box mt={2}>
                                    <Chip
                                        label="Recommended for new projects"
                                        size="small"
                                        color="primary"
                                        variant="outlined"
                                    />
                                </Box>
                            </Box>
                        }
                    />
                </Box>

                <Box flex={1}>
                    <NewAgentTypeCard
                        type="existing"
                        title="Connect Existing Agent"
                        subheader="Integrate an already deployed agent with the platform"
                        icon={<LinkIcon sx={{ fontSize: 40 }} />}
                        onClick={handleSelect}
                        content={
                            <Box>
                                <List dense disablePadding>
                                    <ListItem disablePadding sx={{ mb: 1 }}>
                                        <ListItemIcon sx={{ minWidth: 32 }}>
                                            <CheckCircleOutline fontSize="small" color="success" />
                                        </ListItemIcon>
                                        <ListItemText
                                            primary="Connect existing deployment"
                                        />
                                    </ListItem>
                                    <ListItem disablePadding sx={{ mb: 1 }}>
                                        <ListItemIcon sx={{ minWidth: 32 }}>
                                            <CheckCircleOutline fontSize="small" color="success" />
                                        </ListItemIcon>
                                        <ListItemText
                                            primary="Configure OpenTelemetry integration"
                                        />
                                    </ListItem>
                                    <ListItem disablePadding sx={{ mb: 1 }}>
                                        <ListItemIcon sx={{ minWidth: 32 }}>
                                            <CheckCircleOutline fontSize="small" color="success" />
                                        </ListItemIcon>
                                        <ListItemText
                                            primary="Keep your existing infrastructure"
                                        />
                                    </ListItem>
                                    <ListItem disablePadding>
                                        <ListItemIcon sx={{ minWidth: 32 }}>
                                            <CheckCircleOutline fontSize="small" color="success" />
                                        </ListItemIcon>
                                        <ListItemText
                                            primary="Full control over deployment"
                                        />
                                    </ListItem>
                                </List>
                                <Box mt={2}>
                                    <Chip
                                        label="For production agents"
                                        size="small"
                                        color="secondary"
                                        variant="outlined"
                                    />
                                </Box>
                            </Box>
                        }
                    />
                </Box>
            </Box>
    );
};

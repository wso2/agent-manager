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

import { Card, CardActionArea, CardContent, Box, Typography, alpha } from "@mui/material";
import { ArrowForward } from "@mui/icons-material";

interface NewAgentTypeCardProps {
    type: string;
    title: string;
    subheader: string;
    icon: React.ReactNode;
    content: React.ReactNode;
    onClick: (type: string) => void;
}

export const NewAgentTypeCard = (props: NewAgentTypeCardProps) => {
    const { type, title, subheader, icon, content, onClick } = props;

    const handleClick = () => {
        onClick(type);
    };

    return (
        <Card
            variant="outlined"
            sx={{
                height: '100%',
                transition: 'all 0.3s ease-in-out',
                '&:hover': {
                    boxShadow: 4,
                    borderColor: 'primary.main',
                    transform: 'translateY(-4px)',
                },
            }}
        >
            <CardActionArea
                onClick={handleClick}
                sx={{
                    height: '100%',
                    display: 'flex',
                    flexDirection: 'column',
                    alignItems: 'stretch',
                    justifyContent: 'flex-start',
                }}
            >
                <CardContent sx={{ flexGrow: 1, width: '100%', p: 3 }}>
                    {/* Icon Header */}
                    <Box
                        sx={{
                            mb: 2,
                            display: 'flex',
                            alignItems: 'center',
                            justifyContent: 'center',
                            width: 64,
                            height: 64,
                            borderRadius: 2,
                            bgcolor: (theme) => alpha(theme.palette.primary.main, 0.1),
                            color: 'primary.main',
                        }}
                    >
                        {icon}
                    </Box>

                    {/* Title and Subheader */}
                    <Typography variant="h5" component="div" gutterBottom>
                        {title}
                    </Typography>
                    <Typography variant="body2" color="text.secondary" sx={{ mb: 2 }}>
                        {subheader}
                    </Typography>

                    {/* Content */}
                    <Box sx={{ mb: 2 }}>
                        {content}
                    </Box>

                    {/* Call to Action */}
                    <Box
                        sx={{
                            display: 'flex',
                            alignItems: 'center',
                            gap: 1,
                            color: 'primary.main',
                            mt: 'auto',
                        }}
                    >
                        <Typography variant="body2" fontWeight="medium">
                            Get Started
                        </Typography>
                        <ArrowForward fontSize="small" />
                    </Box>
                </CardContent>
            </CardActionArea>
        </Card>
    );
};

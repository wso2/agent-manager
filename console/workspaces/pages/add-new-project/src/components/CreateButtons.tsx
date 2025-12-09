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

import { Box, Button} from "@wso2/oxygen-ui";
import { Rocket as RocketOutlined, Link } from "@wso2/oxygen-ui-icons-react";

interface CreateButtonsProps {
    isValid: boolean;
    isPending: boolean;
    onCancel: () => void;
    onSubmit: () => void;
    mode?: 'deploy' | 'connect';
}

export const CreateButtons = (
    { isPending, onCancel, onSubmit, mode = 'deploy' }: CreateButtonsProps
) => {
    const isConnectMode = mode === 'connect';    
    return (
        <Box display="flex" flexDirection="column" gap={1}>
            <Box display="flex" flexDirection="row" gap={1} alignItems="center">
                <Button variant="outlined" color="primary" size='medium' onClick={onCancel}>
                    Cancel
                </Button>
                <Button
                    variant="contained"
                    color="primary"
                    size='medium'
                    startIcon={isConnectMode ? 
                    <Link size={16} /> : 
                    <RocketOutlined size={16} />}
                    onClick={onSubmit}
                    disabled={isPending}
                >
                    {isConnectMode ? 'Register' : 'Create'}
                </Button>
            </Box>
        </Box>
    );
};


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

import { User as PersonOutlined } from "@wso2/oxygen-ui-icons-react";
import { Box, LinearProgress} from "@wso2/oxygen-ui";

export function FullPageLoader() {    return (
        <Box sx={{
            display: 'flex',
            flexDirection: 'column',
            justifyContent: 'center',
            alignItems: 'center',
            height: '100vh',
            width: '100vw'
        }}>
            <Box sx={{ display: 'flex', flexDirection:'column', justifyContent: 'center', alignItems: 'center', gap: 2 }}>
                <Box sx={{ fontSize: 100, display: 'inline-flex' }}>
                    <PersonOutlined size={100} color="primary" />
                </Box>
                <LinearProgress color="primary" value={50} sx={{ width: '100%' }} />
            </Box>
        </Box>
    );
}

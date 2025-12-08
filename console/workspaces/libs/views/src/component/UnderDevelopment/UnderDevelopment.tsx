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

import { Box, Typography } from "@wso2/oxygen-ui";
import { FadeIn } from "../FadeIn/FadeIn";
import Image from "../Image/Image";
import { ImageList } from "../Image";

export function UnderDevelopment() {
    return (
        <FadeIn>
            <Box sx={{
                display: 'flex',
                flexDirection: 'column',
                justifyContent: 'center',
                alignItems: 'center',
                height: '70vh',
                p: 2,
                gridGap: 2
            }}>
                <Image
                    src={ImageList.UNDER_DEVELOPMENT} alt="Under Development"
                    width={"30%"}
                    height={"30%"}
                />
                <Typography variant="h5" align="center" color="secondary.main">
                    Coming Soon...
                </Typography>
                <Typography variant="body1" align="center" color="text.secondary">
                    This functionality is currently under development
                    and will be released in an upcoming release.
                </Typography>
            </Box>
        </FadeIn>
    );
}


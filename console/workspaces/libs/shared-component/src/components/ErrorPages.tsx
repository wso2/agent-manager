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

import { Box, Button, Stack, Typography } from "@wso2/oxygen-ui";

function NotFoundErrorPage () {
    return (
        <Box
            sx={{
                minHeight: '80vh',
                width: '100%',
                display: 'flex',
                alignItems: 'center',
                justifyContent: 'center',
                padding: 4,
            }}
        >
            <Stack spacing={2} alignItems="center" sx={{ maxWidth: 480, textAlign: 'center' }}>
                <Typography variant="h1" color="text.secondary" sx={{ fontWeight: 700, fontSize: '6rem' }}>
                    404
                </Typography>
                <Typography variant="h5">
                    Page Not Found
                </Typography>
                <Typography variant="body1" color="text.secondary">
                    The page you are looking for does not exist or has been moved.
                </Typography>
                <Button variant="contained" color="primary" href="/">
                    Go to Home
                </Button>
            </Stack>
        </Box>
    )
}

function OopsErrorPage () {
    return (
        <Box
            sx={{
                minHeight: '80vh',
                width: '100%',
                display: 'flex',
                alignItems: 'center',
                justifyContent: 'center',
                padding: 4,
            }}
        >
            <Stack spacing={2} alignItems="center" sx={{ maxWidth: 480, textAlign: 'center' }}>
                <Typography variant="h1" color="text.secondary" sx={{ fontWeight: 700, fontSize: '6rem' }}>
                    Oops!
                </Typography>
                <Typography variant="h5">
                    Something went wrong.
                </Typography>
                <Typography variant="body1" color="text.secondary">
                    An unexpected error has occurred. Please try again later.
                </Typography>
                <Button variant="contained" color="primary" href="/">
                    Go to Home
                </Button>
            </Stack>
        </Box>
    )
}



export const ErrorPages = {
    NotFound: NotFoundErrorPage,
    Oops: OopsErrorPage
}

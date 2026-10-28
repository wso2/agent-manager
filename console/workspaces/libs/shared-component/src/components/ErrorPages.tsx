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

import { Box, Button, Card, CardContent, Stack, Typography } from "@wso2/oxygen-ui";
import { LogOut, RefreshCw, TriangleAlert } from "@wso2/oxygen-ui-icons-react";
import type { ReactNode } from "react";
import { withBasePath } from "@agent-management-platform/types";

interface ErrorLayoutProps {
    /** Visual anchor shown above the title — an icon or a large glyph. */
    visual: ReactNode;
    title: string;
    message: string;
    subTitle?: string;
    /** Recovery action, typically a button. */
    action: ReactNode;
}

/**
 * Shared, centered card layout for full-page error/empty states. Mirrors the
 * `NoDataFound` empty-state aesthetic (outlined card on a muted background) so
 * error screens feel consistent with the rest of the product.
 */
function ErrorLayout({ visual, title, message, subTitle, action }: ErrorLayoutProps) {
    return (
        <Stack
            alignItems="center"
            justifyContent="center"
            sx={{ minHeight: '90vh', width: '100%', p: 4 }}
        >
            <Card
                variant="outlined"
                sx={{
                    width: '100%',
                    maxWidth: 480,
                    animation: 'errorFadeIn 0.3s ease-in-out',
                    '@keyframes errorFadeIn': {
                        '0%': { opacity: 0 },
                        '100%': { opacity: 1 },
                    },
                }}
            >
                <CardContent sx={{ p: 5 }}>
                    <Stack spacing={2} alignItems="center" sx={{ textAlign: 'center' }}>
                        {visual}
                        <Typography variant="h4">
                            {title}
                        </Typography>
                        {subTitle && (
                            <Typography variant="subtitle1" color="text.secondary">
                                {subTitle}
                            </Typography>
                        )}
                        <Typography variant="body1" color="text.secondary">
                            {message}
                        </Typography>
                        <Stack
                            direction="row"
                            spacing={1.5}
                            useFlexGap
                            justifyContent="center"
                            sx={{ mt: 1, flexWrap: 'wrap' }}
                        >
                            {action}
                        </Stack>
                    </Stack>
                </CardContent>
            </Card>
        </Stack>
    );
}

// Shared error glyph used by the generic error states (Oops / CustomError).
const errorVisual = (
    <Box sx={{ color: 'error.main', display: 'flex' }}>
        <TriangleAlert size={56} strokeWidth={1.5} />
    </Box>
);

function NotFoundErrorPage() {
    return (
        <ErrorLayout
            visual={
                <Typography variant="h1" color="text.secondary" sx={{ fontWeight: 'bold', lineHeight: 1 }}>
                    404
                </Typography>
            }
            title="Page Not Found"
            message="The page you are looking for does not exist or has been moved."
            action={
                <Button variant="contained" color="primary" href={withBasePath("/")}>
                    Go to Home
                </Button>
            }
        />
    );
}

function OopsErrorPage() {
    return (
        <ErrorLayout
            visual={errorVisual}
            title="Something went wrong."
            message="An unexpected error has occurred. Please try again later."
            action={
                <Button variant="contained" color="primary" href={withBasePath("/")}>
                    Go to Home
                </Button>
            }
        />
    );
}

interface ErrorPageProps {
    message: string;
    title: string;
    subTitle?: string;
    /** When provided, shows a secondary "Sign Out" button next to "Try Again". */
    onLogout?: () => void;
}

function ErrorPage({ message, title, subTitle, onLogout }: ErrorPageProps) {
    return (
        <ErrorLayout
            visual={errorVisual}
            title={title}
            subTitle={subTitle}
            message={message}
            action={
                <>
                    {onLogout && (
                        <Button
                            variant="outlined"
                            color="primary"
                            startIcon={<LogOut size={18} />}
                            onClick={onLogout}
                        >
                            Sign out
                        </Button>
                    )}
                    <Button
                        variant="contained"
                        color="primary"
                        startIcon={<RefreshCw size={18} />}
                        onClick={() => window.location.reload()}
                    >
                        Try again
                    </Button>
                </>
            }
        />
    );
}

export const ErrorPages = {
    NotFound: NotFoundErrorPage,
    Oops: OopsErrorPage,
    CustomError: ErrorPage
}

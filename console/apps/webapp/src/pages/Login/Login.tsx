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

import { useAuthHooks } from "@agent-management-platform/auth";
import { useLocation } from "react-router-dom";
import {
  Button,
  Box,
  Typography,
  Grid,
  Stack,
  Paper,
  styled,
} from "@wso2/oxygen-ui";
import {
  Rocket,
  Binoculars,
  ShieldCheck,
  LucideLogIn,
} from "@wso2/oxygen-ui-icons-react";
import { useEffect } from "react";
import { FullPageLoader } from "@agent-management-platform/views";

const StyledPaper = styled(Paper)(({ theme }) => ({
  display: "flex",
  flexDirection: "column",
  alignItems: "center",
  justifyContent: "center",
  height: "100%",
  padding: theme.spacing(4),
}));

const items = [
  {
    icon: <Rocket size={32} />,
    title: "Deploy at Scale",
    description:
      "Deploy and run AI agents on OpenChoreo with production-ready configurations.",
  },
  {
    icon: <Binoculars size={32} />,
    title: "Full Observability",
    description:
      "Capture traces, metrics, and logs for complete visibility into agent behavior.",
  },
  {
    icon: <ShieldCheck size={32} />,
    title: "Governance",
    description:
      "Enforce policies, manage access controls, and ensure compliance across all agents.",
  },
];

export function Login() {
  const {
    isAuthenticated,
    login,
    userInfo,
    isLoadingUserInfo,
    isLoadingIsAuthenticated,
  } = useAuthHooks();

  const { state } = useLocation();
  const from = state?.from?.pathname || "/";

  // Check if we're handling an OAuth callback
  const isOAuthCallback = new URLSearchParams(window.location.search).has(
    "code"
  );

  useEffect(() => {
    // Only auto-trigger login if:
    // - Not authenticated
    // - Not loading
    // - NOT on OAuth callback (let SDK handle the code exchange)
    if (!isOAuthCallback && !isAuthenticated && !isLoadingUserInfo) {
      login();
    }
  }, [
    isAuthenticated,
    isLoadingIsAuthenticated,
    isOAuthCallback,
    isLoadingUserInfo,
  ]);

  // Handle redirect after successful authentication
  useEffect(() => {
    if (userInfo) {
      window.location.href = from;
    }
  }, [userInfo]);

  // Show loader while auth is in progress
  // For OAuth callback: show loader only while not yet authenticated (SDK is processing)
  if (
    isLoadingIsAuthenticated ||
    isLoadingUserInfo ||
    (isOAuthCallback && !isAuthenticated)
  ) {
    return (
      <>
        <FullPageLoader />
      </>
    );
  }

  return (
    <Box sx={{ height: "100vh", display: "flex", alignItems: "center" }}>
      <Grid
        container
        sx={{ flex: 1, justifyContent: "center", alignItems: "center" }}
      >
        <Grid
          size={{ xs: 12, md: 7 }}
          sx={{
            display: "flex",
            alignItems: "center",
            justifyContent: "center",
            textAlign: "left",
            position: "relative",
          }}
        >
          <Box>
            <Stack
              direction="column"
              alignItems="start"
              gap={5}
              maxWidth={580}
              display={{ xs: "none", md: "flex" }}
            >
              <Typography variant="h3">WSO2 Agent Manager</Typography>
              <Stack
                sx={{ flexDirection: "column", alignSelf: "center", gap: 4 }}
              >
                {items.map((item) => (
                  <Stack key={item.title} direction="row" sx={{ gap: 2 }}>
                    {item.icon}
                    <div>
                      <Typography gutterBottom sx={{ fontWeight: "medium" }}>
                        {item.title}
                      </Typography>
                      <Typography
                        variant="body2"
                        sx={{ color: "text.secondary" }}
                      >
                        {item.description}
                      </Typography>
                    </div>
                  </Stack>
                ))}
              </Stack>
            </Stack>
          </Box>
        </Grid>
        <StyledPaper sx={{ height: "fit-content", width: 500 }}>
          <Box
            sx={{
              alignItems: "center",
              justifyContent: "center",
              width: "100%",
              py: 4,
            }}
          >
            <Box sx={{ mb: 6, textAlign: "center" }}>
              <Typography variant="h3" gutterBottom>
                Sign in
              </Typography>
            </Box>
            <Button
              variant="outlined"
              onClick={login}
              fullWidth
              color="primary"
              size="large"
              startIcon={<LucideLogIn size={24} />}
            >
              Continue
            </Button>

            <Box component="footer" sx={{ mt: 4 }}>
              <Typography sx={{ textAlign: "center", color: "text.secondary" }}>
                © Copyright {new Date().getFullYear()}
              </Typography>
            </Box>
          </Box>
        </StyledPaper>
      </Grid>
    </Box>
  );
}

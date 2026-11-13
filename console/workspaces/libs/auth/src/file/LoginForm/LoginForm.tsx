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

import {
  Alert,
  alpha,
  Avatar,
  Box,
  Button,
  Card,
  CardContent,
  Divider,
  FormLabel,
  IconButton,
  InputAdornment,
  List,
  ListItem,
  ListItemAvatar,
  ListItemText,
  Stack,
  TextField,
  type Theme,
  Typography,
} from '@wso2/oxygen-ui';
import {
  Activity,
  ArrowRight,
  Eye,
  EyeOff,
  Lock,
  PencilRuler,
  Rocket,
  ShieldCheck,
  Sparkles,
  User,
} from '@wso2/oxygen-ui-icons-react';
import { Logo } from '@agent-management-platform/views';
import {
  useState,
  type ChangeEvent,
  type KeyboardEvent,
  type ReactNode,
} from 'react';

import { useFileAuth } from '../FileAuthContext';

const FEATURES: { icon: ReactNode; label: string }[] = [
  { icon: <PencilRuler size={18} />, label: 'Design agents and connect MCP tools' },
  { icon: <Rocket size={18} />, label: 'Deploy to any environment in minutes' },
  { icon: <ShieldCheck size={18} />, label: 'Secure agent identity and access' },
  { icon: <Activity size={18} />, label: 'Trace, evaluate, and monitor behaviour' },
  { icon: <Sparkles size={18} />, label: 'Govern model usage and cost' },
];

/**
 * The product's brand colour painted onto text rather than a surface. The
 * console's theme carries no gradient token, so this is built from the palette
 * rather than written as hex, and tracks a theme change with everything else.
 */
const gradientTextSx = (theme: Theme) =>
  ({
    backgroundClip: 'text',
    backgroundImage: `linear-gradient(120deg, ${theme.palette.primary.main}, ${theme.palette.primary.dark})`,
    color: 'transparent',
    WebkitBackgroundClip: 'text',
  }) as const;

/**
 * The single primary action on this page, lifted above a plain `contained`
 * button. The disabled state drops back to the theme's own treatment, so a
 * disabled button never reads as available.
 */
const signInButtonSx = (theme: Theme) =>
  ({
    backgroundImage: `linear-gradient(120deg, ${theme.palette.primary.main}, ${theme.palette.primary.dark})`,
    py: 1.5,
    '&:hover': {
      backgroundImage: `linear-gradient(120deg, ${theme.palette.primary.main}, ${theme.palette.primary.dark})`,
      filter: 'brightness(1.06)',
    },
    '&.Mui-disabled': { backgroundImage: 'none' },
  }) as const;

/** A static label above the field, never a floating TextField label. */
const fieldLabelSx = { color: 'text.primary', fontWeight: 500 } as const;

const fieldSx = {
  '& .MuiOutlinedInput-input': { fontSize: '1rem' },
  '& .MuiOutlinedInput-root': { bgcolor: 'background.paper' },
} as const;

const featureIconSx = (theme: Theme) =>
  ({
    bgcolor: alpha(theme.palette.primary.main, 0.1),
    color: 'primary.main',
    height: 40,
    width: 40,
  }) as const;

/**
 * The credential form shown at /login when the deployment authenticates from a
 * user file rather than an identity provider.
 *
 * There is no "forgot password" or "sign up": file-mode accounts are managed by
 * editing the server's user file, so offering either would promise something
 * the deployment cannot do.
 */
export const LoginForm = () => {
  const { signIn } = useFileAuth();
  const [username, setUsername] = useState('');
  const [password, setPassword] = useState('');
  const [showPassword, setShowPassword] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [submitting, setSubmitting] = useState(false);

  const canSubmit = username.trim() !== '' && password !== '' && !submitting;

  const submit = async () => {
    if (!canSubmit) {
      return;
    }
    setSubmitting(true);
    setError(null);
    try {
      await signIn(username.trim(), password);
      // No navigation here: Login.tsx watches isAuthenticated and redirects to
      // wherever the user was heading before they were sent to /login.
    } catch (caught) {
      setError(caught instanceof Error ? caught.message : 'Could not sign in.');
      // Clear only the password. Retyping a username that was never in doubt
      // is the kind of small insult a failed login does not need to add.
      setPassword('');
    } finally {
      setSubmitting(false);
    }
  };

  const onEnter = (event: KeyboardEvent<HTMLInputElement>) => {
    if (event.key === 'Enter') {
      void submit();
    }
  };

  return (
    <Box
      sx={{
        alignItems: 'center',
        display: 'flex',
        flexWrap: { md: 'nowrap', xs: 'wrap' },
        minHeight: '100vh',
        // Without a cap the two halves are each 50% of the viewport, so on a
        // wide monitor they drift to opposite edges with a few hundred pixels
        // of nothing between them. The cap keeps them reading as one layout.
        maxWidth: 1400,
        mx: 'auto',
        width: '100%',
      }}
    >
      {/* LEFT — what the product is. Dropped on small screens, where the
          credential card is the only thing worth the width. */}
      <Stack
        spacing={4}
        sx={{
          display: { md: 'flex', xs: 'none' },
          flex: '1 1 50%',
          justifyContent: 'center',
          minWidth: 0,
          p: { lg: 6, md: 5 },
        }}
      >
        <Box>
          <Logo style={{ height: 32, width: 'auto' }} />
        </Box>

        <Stack spacing={2.5} sx={{ maxWidth: 600 }}>
          <Typography
            component="h1"
            sx={{
              fontSize: { lg: '3.25rem', md: '2.5rem' },
              fontWeight: 500,
              letterSpacing: '-1.5px',
              lineHeight: 1.1,
            }}
          >
            Build, deploy, and govern your{' '}
            <Box component="span" sx={gradientTextSx}>
              AI agents
            </Box>
          </Typography>
          <Typography
            color="text.secondary"
            sx={{ fontSize: '1.125rem', lineHeight: 1.6, maxWidth: 490 }}
          >
            One platform to design, deploy, secure, and observe AI agents and the
            tools they depend on.
          </Typography>
        </Stack>

        <Divider sx={{ maxWidth: 600 }} />

        <List disablePadding sx={{ maxWidth: 600 }}>
          {FEATURES.map((feature) => (
            <ListItem disableGutters key={feature.label} sx={{ py: 1 }}>
              <ListItemAvatar sx={{ minWidth: 0, mr: 2.5 }}>
                <Avatar sx={featureIconSx} variant="rounded">
                  {feature.icon}
                </Avatar>
              </ListItemAvatar>
              <ListItemText
                primary={feature.label}
                slotProps={{ primary: { sx: { fontSize: '1.0625rem' } } }}
              />
            </ListItem>
          ))}
        </List>
      </Stack>

      {/* RIGHT — the credential card */}
      <Box
        sx={{
          alignItems: 'center',
          display: 'flex',
          flex: { md: '1 1 50%', xs: '1 1 100%' },
          justifyContent: 'center',
          p: { md: 6, xs: 3 },
          width: '100%',
        }}
      >
        <Card
          component="form"
          elevation={0}
          onSubmit={(event: React.FormEvent) => {
            event.preventDefault();
            void submit();
          }}
          sx={(theme) => ({
            bgcolor: 'background.paper',
            border: 1,
            borderColor: 'divider',
            borderRadius: 2,
            boxShadow: `0 1px 2px ${alpha(theme.palette.common.black, 0.04)}, 0 24px 56px ${alpha(
              theme.palette.common.black,
              0.1,
            )}`,
            maxWidth: 480,
            width: '100%',
          })}
        >
          <CardContent sx={{ p: { sm: 5, xs: 3 } }}>
            <Stack spacing={3}>
              {/* The logo moves into the card on small screens, where the
                  left panel it normally sits in is hidden. */}
              <Box sx={{ display: { md: 'none', xs: 'block' } }}>
                <Logo style={{ height: 28, width: 'auto' }} />
              </Box>

              <Stack spacing={0.75}>
                <Typography sx={{ letterSpacing: '-0.5px' }} variant="h4">
                  Sign in
                </Typography>
                <Typography color="text.secondary" variant="subtitle1">
                  Use the credentials configured for this deployment.
                </Typography>
              </Stack>

              {error && <Alert severity="error">{error}</Alert>}

              <Stack spacing={2.5}>
                <Stack spacing={1}>
                  <FormLabel htmlFor="login-username" sx={fieldLabelSx}>
                    Username
                  </FormLabel>
                  <TextField
                    autoComplete="username"
                    autoFocus
                    disabled={submitting}
                    fullWidth
                    id="login-username"
                    name="username"
                    onChange={(event: ChangeEvent<HTMLInputElement>) =>
                      setUsername(event.target.value)
                    }
                    onKeyDown={onEnter}
                    placeholder="e.g. admin"
                    size="medium"
                    slotProps={{
                      input: {
                        startAdornment: (
                          <InputAdornment position="start">
                            <User size={18} />
                          </InputAdornment>
                        ),
                      },
                    }}
                    sx={fieldSx}
                    type="text"
                    value={username}
                  />
                </Stack>

                <Stack spacing={1}>
                  <FormLabel htmlFor="login-password" sx={fieldLabelSx}>
                    Password
                  </FormLabel>
                  <TextField
                    autoComplete="current-password"
                    disabled={submitting}
                    fullWidth
                    id="login-password"
                    name="password"
                    onChange={(event: ChangeEvent<HTMLInputElement>) =>
                      setPassword(event.target.value)
                    }
                    onKeyDown={onEnter}
                    placeholder="Enter your password"
                    size="medium"
                    slotProps={{
                      input: {
                        endAdornment: (
                          <InputAdornment position="end">
                            <IconButton
                              aria-label={showPassword ? 'Hide password' : 'Show password'}
                              edge="end"
                              onClick={() => setShowPassword((visible) => !visible)}
                              size="small"
                            >
                              {showPassword ? <EyeOff size={18} /> : <Eye size={18} />}
                            </IconButton>
                          </InputAdornment>
                        ),
                        startAdornment: (
                          <InputAdornment position="start">
                            <Lock size={18} />
                          </InputAdornment>
                        ),
                      },
                    }}
                    sx={fieldSx}
                    type={showPassword ? 'text' : 'password'}
                    value={password}
                  />
                </Stack>

                <Button
                  disabled={!canSubmit}
                  endIcon={<ArrowRight size={18} />}
                  fullWidth
                  size="large"
                  sx={signInButtonSx}
                  type="submit"
                  variant="contained"
                >
                  {submitting ? 'Signing in…' : 'Sign in'}
                </Button>
              </Stack>
            </Stack>
          </CardContent>
        </Card>
      </Box>
    </Box>
  );
};

import { ArrowLeft } from '@wso2/oxygen-ui-icons-react';
import {
  Avatar,
  Box,
  Button,
  Container,
  Typography,
} from '@wso2/oxygen-ui';
import { ReactNode } from 'react';
import { Link } from 'react-router-dom';
import { FadeIn } from '../FadeIn';

export interface PageLayoutProps {
  children: ReactNode;
  backHref?: string;
  title?: string;
  backLabel?: string;
  description?: string;
  titleTail?: ReactNode;
  disableIcon?: boolean;
  actions?: ReactNode;
  disablePadding?: boolean;
}
export function PageLayout({
  children,
  title,
  backHref,
  backLabel,
  description,
  titleTail,
  actions,
  disablePadding = false,
  disableIcon = false,
}: PageLayoutProps) {
  return (
    <Box
      display="flex"
      flexDirection="column"
      overflow="auto"
      py={disablePadding ? 0 : 1.5}
      px={disablePadding ? 0 : 3}
      gap={2}
    >
      {backHref && (
        <Box display="flex" alignItems="center">
          <Button
            variant="text"
            color="inherit"
            size="small"
            component={Link}
            startIcon={<ArrowLeft size={16} />}
            to={backHref}
          >
            {backLabel || 'Back'}
          </Button>
        </Box>
      )}
      <Box
        flexGrow={1}
        display="flex"
        justifyContent="space-between"
        flexDirection="row"
        gap={2}
      >
        <Box display="flex" alignItems="center" gap={2}>
          <Box display="flex" flexDirection="column" gap={2}>
            <FadeIn>
              <Box display="flex" alignItems="start" justifyContent="start" gap={2}>
                {!disableIcon && (
                  <Avatar
                    variant="rounded"
                    sx={{
                      height: 72,
                      width: 72,
                      fontSize: "2rem",
                      "&.MuiAvatar-root":{
                        bgcolor: 'primary.main',
                        color: 'background.paper',
                      }
                    }}
                  >
                    {title?.substring(0, 1).toUpperCase()}
                  </Avatar>
                )}
                <Box
                  display="flex"
                  flexDirection="column"
                  gap={1}
                >
                  <Box display="flex" gap={2} alignItems="center">
                    <Typography
                      variant={backHref ? 'h3' : 'h2'}
                    >
                      {title}
                    </Typography>
                    {titleTail ? titleTail : <Box />}
                  </Box>
                  {description && (
                    <Typography
                      sx={{ maxWidth: '50vw' }}
                      variant="body2"
                    >
                      {description}
                    </Typography>
                  )}
                </Box>
              </Box>
            </FadeIn>
          </Box>
        </Box>
        <Box>{actions && <Box>{actions}</Box>}</Box>
      </Box>
      <Box flexGrow={1} display="flex" flexDirection="column">
        <Box gap={2}>{children}</Box>
      </Box>
    </Box>
  );
}

export function PageLayoutContent(
  props: Omit<PageLayoutProps, 'disablePadding'>
) {
  return (
    <Container maxWidth="lg" disableGutters>
      <PageLayout disablePadding={true} {...props} />
    </Container>
  );
}

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

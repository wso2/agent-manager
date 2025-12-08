import { Box } from "@wso2/oxygen-ui";

interface FadeInProps {
    children: React.ReactNode;
}   
export function FadeIn({ children }: FadeInProps) {
    return (
        <Box sx={{
            "animation": "fadeIn 0.3s ease-in-out",
            "@keyframes fadeIn": {
                "0%": {
                    opacity: 0,
                },
                "100%": {
                    opacity: 1,
                },
            },
        }}>
            {children}
        </Box>
    );
}



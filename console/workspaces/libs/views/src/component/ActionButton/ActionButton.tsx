import { Button, useTheme } from "@wso2/oxygen-ui";

export interface ActionButtonProps {
    children: React.ReactNode;
    size?: 'small' | 'medium' | 'large';
    startIcon?: React.ReactNode;
    endIcon?: React.ReactNode;
    onClick: () => void;
    disabled?: boolean;
    loading?: boolean;
}
export function ActionButton(props: ActionButtonProps) {
    const theme = useTheme();
    return (
        <Button
            {...props}
            variant="contained"
            sx={{
                transition: theme.transitions.create(['filter']),
                background: `linear-gradient(45deg, ${theme.palette.secondary.main} 0%, ${theme.palette.primary.main} 100%)`,
                borderRadius: 4,
                '&:hover': {
                    filter: 'brightness(1.1)',
                },

            }}
        />
    );
}

import React from 'react';
import { alpha, Box, BoxProps, useTheme } from '@wso2/oxygen-ui';
import clsx from 'clsx';

export interface MainActionPanelProps extends Omit<BoxProps, 'children'> {
    children: React.ReactNode;
    className?: string;
    variant?: 'elevated' | 'outlined' | 'filled';
    elevation?: number;
}

export function MainActionPanel({
    children,
    className,
    variant = 'elevated',
    elevation = 8,
    sx,
    ...boxProps
}: MainActionPanelProps) {
    const theme = useTheme();
    const getVariantStyles = () => {
        switch (variant) {
            case 'outlined':
                return {
                    borderTop: '1px solid',
                    borderColor: 'border.primary',
                    backgroundColor: 'background.paper',
                };
            case 'filled':
                return {
                    backgroundColor: 'surface.secondary',
                };
            case 'elevated':
            default:
                return {
                    backgroundColor: alpha(theme.palette.background.paper, 0.9),
                    boxShadow: `0 -${elevation}px ${elevation * 2}px rgba(0, 0, 0, 0.1)`,
                };
        }
    };

    return (
        <Box
            data-testid="MainActionPanel"
            className={clsx('main-action-panel', className)}
            sx={{
                position: 'absolute',
                bottom: 0,
                left: 0,
                width: '100%',
                // right: 0,
                zIndex: 1203,
                padding: 2,
                ...getVariantStyles(),
                ...sx,
                animation: 'slideUpDown 0.3s  ease-in',
                '@keyframes slideUpDown': {
                    '0%': {
                        transform: 'translateY(100%)',
                    },
                    '100%': {
                        transform: 'translateY(0px)',
                    },
                },
            }}
            {...boxProps}
        >
            {children}
        </Box>
    );
}

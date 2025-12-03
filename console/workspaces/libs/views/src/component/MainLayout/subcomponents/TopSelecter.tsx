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

import { alpha, Box, Button, ButtonBase, Divider, IconButton, List, ListItemButton, ListItemText, Popover, TextField, Typography, useTheme } from "@mui/material";
import { AddOutlined, CloseOutlined, KeyboardArrowDown, KeyboardArrowRightOutlined, KeyboardArrowUp, SearchOutlined } from "@mui/icons-material";
import { useState, useMemo } from "react";
import { FadeIn } from "../../FadeIn";

export interface Option {
    label: string;
    id: string;
}
export interface TopSelecterProps {
    label: string;
    onChange: (value: string) => void;
    options: Option[];
    selectedId?: string;
    disableClose: boolean;
    onClose?: () => void;
    onClick: () => void;
    onCreate?: () => void;
}

export function TopSelecter(props: TopSelecterProps) {
    const { label,
        onChange,
        options,
        selectedId,
        disableClose,
        onClose,
        onClick,
        onCreate
    } = props;
    const theme = useTheme();
    const [anchorEl, setAnchorEl] = useState<HTMLButtonElement | null>(null);
    const [searchQuery, setSearchQuery] = useState("");

    const handleClick = (event: React.MouseEvent<HTMLButtonElement>) => {
        event.stopPropagation();
        setAnchorEl(event.currentTarget);
    };

    const handleClose = () => {
        setAnchorEl(null);
        setSearchQuery("");
    };

    const handleSelectOption = (optionId: string) => {
        onChange(optionId);
        handleClose();
    };

    const filteredOptions = useMemo(() => {
        if (!searchQuery.trim()) {
            return options;
        }
        return options.filter((option) =>
            option.label.toLowerCase().includes(searchQuery.toLowerCase())
        );
    }, [options, searchQuery]);

    const selectedOption = options.find((opt) => opt.id === selectedId);

    const open = Boolean(anchorEl);
    const id = open ? 'top-selector-popover' : undefined;

    if (options.length === 0) {
        return null;
    }
    return (
        <>
            {selectedId ?
                <FadeIn>
                    <ButtonBase
                        onClick={onClick}
                        sx={{
                            display: "flex",
                            alignItems: "start",
                            gap: theme.spacing(4),
                            height: theme.spacing(7),
                            padding: theme.spacing(1),
                            borderRadius: theme.spacing(0.75),
                            transition: "all 0.3s ease",
                            border: `1px solid ${alpha(theme.palette.text.primary, 0.2)}`,
                            backgroundColor: theme.palette.background.paper,
                            "&:hover": {
                                backgroundColor: theme.palette.background.default,
                                border: `1px solid ${alpha(theme.palette.primary.light, 1)}`,
                                cursor: "pointer",
                                "& .selector-name-button": {
                                    color: theme.palette.text.primary,
                                },
                            },
                        }}
                    >
                        <Box display="flex" alignItems="flex-start" flexDirection="column" gap={theme.spacing(0.25)}>
                            <Typography variant="caption" color={theme.palette.text.secondary}>{label}</Typography>
                            <Box display="flex" alignItems="center" gap={theme.spacing(0.5)}>
                                <Typography className="selector-name-button" variant="body1" color={theme.palette.text.primary}>
                                    {selectedOption?.label || "Select an option"}
                                </Typography>
                                <IconButton
                                    size="small"
                                    onClick={handleClick}
                                    sx={{
                                        borderRadius: theme.spacing(0.5),
                                        padding: theme.spacing(0),
                                        border: `1px solid ${alpha(theme.palette.primary.light, 0)}`,
                                        "&:hover": {
                                            border: `1px solid ${theme.palette.primary.light}`,
                                            backgroundColor:
                                                alpha(theme.palette.primary.light, 0.2),
                                        },
                                    }}
                                    className="selector-name-button"
                                >
                                    {open ? <KeyboardArrowUp color="inherit" fontSize='inherit' /> : <KeyboardArrowDown color="inherit" fontSize='inherit' />}
                                </IconButton>
                            </Box>
                        </Box>
                        {onClose ? (
                            <IconButton
                                size="small"
                                sx={{
                                    padding: theme.spacing(0),
                                    "&:hover": {
                                        backgroundColor: alpha(theme.palette.primary.light, 0.2),
                                    },
                                }}
                                disabled={disableClose}
                                onClick={(e) => {
                                    e.stopPropagation();
                                    onClose();
                                }}
                            >
                                <CloseOutlined sx={{ fontSize: theme.spacing(1.75) }} />
                            </IconButton>
                        ) : <Box width={theme.spacing(1.75)} />}
                    </ButtonBase>
                </FadeIn>
                :
                <FadeIn>
                    <ButtonBase
                        onClick={handleClick}
                        sx={{
                            display: "flex",
                            justifyContent: "center",
                            alignItems: "center",
                            padding: theme.spacing(1),
                            borderRadius: theme.spacing(0.75),
                            color: theme.palette.text.primary,
                            transition: "all 0.3s ease",
                            border: `1px solid ${alpha(theme.palette.text.primary, 0.1)}`,
                            "&:hover": {
                                backgroundColor: alpha(theme.palette.background.default, 0.2),
                                border: `1px solid ${alpha(theme.palette.primary.light, 1)}`,
                                cursor: "pointer",
                            },
                        }}
                    >
                        {open ? <KeyboardArrowDown fontSize='small' /> : <KeyboardArrowRightOutlined fontSize='small' />}
                    </ButtonBase>
                </FadeIn>
            }
            <Popover
                id={id}
                open={open}
                anchorEl={anchorEl}
                onClose={handleClose}
                anchorOrigin={{
                    vertical: 'top',
                    horizontal: 'left',
                }}
                transformOrigin={{
                    vertical: 'top',
                    horizontal: 'left',
                }}
            >
                <Box px={1} py={1.5} display="flex" flexDirection="column" gap={0.5} sx={{ width: theme.spacing(37.5) }}>
                    <TextField
                        fullWidth
                        size="small"
                        placeholder="Search..."
                        value={searchQuery}
                        onChange={(e) => setSearchQuery(e.target.value)}
                        sx={{ marginBottom: theme.spacing(1), m: 0 }}
                        variant='standard'
                        slotProps={{ input: { endAdornment: <SearchOutlined fontSize='small' /> } }}
                    />
                    {
                        onCreate && (
                            <Box>
                                <Button
                                    sx={{ pl: theme.spacing(0.5) }}
                                    variant='text'
                                    startIcon={<AddOutlined fontSize='inherit' />}
                                    size='small'
                                    onClick={onCreate}
                                >
                                    Create {label}
                                </Button>
                            </Box>
                        )
                    }
                    <Divider />
                    <List sx={{ p: 0, m: 0 }}>
                        {filteredOptions.length > 0 ? (
                            filteredOptions.map((option) => (
                                <ListItemButton
                                    key={option.id}
                                    onClick={() => handleSelectOption(option.id)}
                                    sx={{
                                        padding: theme.spacing(0.5, 1),
                                        margin: theme.spacing(0, 0, 0.25, 0),
                                        backgroundColor: option.id === selectedId ? alpha(theme.palette.primary.light, 0.2) : 'transparent',
                                    }}
                                >
                                    <ListItemText
                                        primary={option.label}
                                    />
                                </ListItemButton>
                            ))
                        ) : (
                            <Typography
                                variant="body2"
                                color={theme.palette.text.secondary}
                                sx={{
                                    padding: theme.spacing(2),
                                    textAlign: 'center',
                                }}
                            >
                                No results found
                            </Typography>
                        )}
                    </List>
                </Box>
            </Popover>
        </>
    );
}

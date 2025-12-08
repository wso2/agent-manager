import React, { useEffect, useMemo, useState } from 'react';
import {
    Box,
    Button,
    Chip,
    TextField,
    Typography,
    Paper,
    useTheme,
    Alert,
    CircularProgress,
    Stack,
    InputAdornment,
    Select,
    MenuItem,
} from '@wso2/oxygen-ui';
import { ChevronDown as ArrowDropDown, Play as PlayArrow } from '@wso2/oxygen-ui-icons-react';
import { useGetAgentEndpoints } from '@agent-management-platform/api-client';
import { useParams } from 'react-router-dom';

export interface AgentInvokeTestProps {
    defaultBody?: Record<string, unknown>;
}

export function AgentInvokeTest({
    defaultBody = {
        thread_id: 123,
        question: "Hi, How can you help me?"
      }
      
}: AgentInvokeTestProps) {
    const theme = useTheme();
    const [endpoint, setEndpoint] = useState("");
    const [requestBody, setRequestBody] = useState(JSON.stringify(defaultBody, null, 2));
    const [response, setResponse] = useState<string | null>(null);
    const [error, setError] = useState<string | null>(null);
    const [isLoading, setIsLoading] = useState(false);
    const { agentId, orgId, projectId, envId } = useParams();

    const { data: endpoints } = useGetAgentEndpoints(
        {
            projName: projectId ?? '',
            orgName: orgId ?? '',
            agentName: agentId ?? '',
        }, {
        environment: envId ?? '',
    }
    )
    const endpointOptions = useMemo(() => {
        return Object.entries(endpoints ?? {}).map(
            ([key, value]) => ({ label: key, value: value.url }));
    }, [endpoints])

    useEffect(() => {
        if (endpointOptions.length > 0) {
            setEndpoint(endpointOptions[0].value + '/invocations');
        }
    }, [endpointOptions]);
    
    const handleRunTest = async () => {
        setError(null);
        setResponse(null);
        setIsLoading(true);

        try {
            // Validate JSON
            let parsedBody: object;
            try {
                parsedBody = JSON.parse(requestBody);
            } catch {
                setError('Invalid JSON format in request body');
                setIsLoading(false);
                return;
            }

            const apiResponse = await fetch(endpoint, {
                method: 'POST',
                headers: {
                    'Content-Type': 'application/json',
                    
                },
                body: JSON.stringify(parsedBody),
                referrerPolicy: ''
                
            });

            let responseData: unknown;
            const contentType = apiResponse.headers.get('content-type');
            if (contentType && contentType.includes('application/json')) {
                responseData = await apiResponse.json();
            } else {
                responseData = await apiResponse.text();
            }

            if (!apiResponse.ok) {
                const errorMessage = typeof responseData === 'string'
                    ? responseData
                    : JSON.stringify(responseData, null, 2);
                setError(`Request failed with status ${apiResponse.status}: ${errorMessage}`);
            } else {
                const responseText = typeof responseData === 'string'
                    ? responseData
                    : JSON.stringify(responseData, null, 2);
                setResponse(responseText);
            }
        } catch (err) {
            setError(err instanceof Error ? err.message : 'An error occurred while making the request');
        } finally {
            setIsLoading(false);
        }
    };

    const handleKeyDown = (event: React.KeyboardEvent) => {
        if ((event.metaKey || event.ctrlKey) && event.key === 'Enter') {
            event.preventDefault();
            handleRunTest();
        }
    };

    const characterCount = requestBody.length;
    const isJsonValid = (() => {
        try {
            JSON.parse(requestBody);
            return true;
        } catch {
            return false;
        }
    })();

    return (
        <Box display="flex" flexDirection="column" pt={2} gap={2} width="100%">
            {/* API Request Header */}
            <Box display="flex" alignItems="center" gap={2} flexWrap="wrap">

                <TextField
                    value={endpoint}
                    onChange={(e) => setEndpoint(e.target.value)}
                    placeholder="/agent/invoke"
                    variant="outlined"
                    size="small"
                    slotProps={{
                        input: {
                            startAdornment: (
                                <InputAdornment position="start">
                                    <Chip
                                        label="POST"
                                        size="small"
                                        color="default"
                                        sx={{
                                            height: 3,
                                            fontSize: '0.75rem',
                                        }}
                                    />
                                </InputAdornment>
                            ),
                            endAdornment: (
                                <InputAdornment position="end">
                                    <Button
                                        variant="text"
                                        size="small"
                                        endIcon={<ArrowDropDown fontSize="inherit" />}
                                        sx={{
                                            textTransform: 'none',
                                        }}
                                    >
                                        Samples
                                    </Button>
                                </InputAdornment>
                            ),
                        },
                    }}
                    sx={{
                        flex: 1,
                        minWidth: 30,
                        m: 0,
                        "& .MuiInputBase-root": {
                            padding: 0,
                        },
                    }}
                />
                {
                    endpointOptions.length > 1 && (
                        <Select
                            value={endpoint}
                            onChange={(e) => setEndpoint(e.target.value)}
                            variant="outlined"
                            size="small"
                        >
                            {
                                endpointOptions.map((option) => (
                                    <MenuItem key={option.value} value={option.value}>
                                        {option.label}
                                    </MenuItem>
                                ))
                            }
                        </Select>
                    )
                }
            </Box>

            {/* Request Body Editor */}

            <TextField
                multiline
                fullWidth
                value={requestBody}
                onChange={(e) => setRequestBody(e.target.value)}
                onKeyDown={handleKeyDown}
                placeholder='{"message": "Hello, how can you help me?", "context": {...}}'
                variant="outlined"
                error={!isJsonValid && requestBody.length > 0}
                sx={{
                    '& .MuiInputBase-root': {
                        fontFamily: 'monospace',
                        padding: 0,
                        fontSize: theme.typography.body2.fontSize,
                    },
                    '& .MuiInputBase-input': {
                        minHeight: 25,
                        padding: 2,
                    },
                }}
            />


            {/* Status Bar and Run Button */}
            <Box display="flex" justifyContent="space-between" alignItems="center" flexWrap="wrap" gap={2}>
                <Stack direction="row" spacing={2} alignItems="center">
                    <Typography variant="caption" color="text.secondary">
                        {characterCount} characters
                    </Typography>
                    <Chip
                        label="JSON format"
                        size="small"
                        variant="outlined"
                        sx={{
                            height: 3,
                        }}
                    />
                    <Typography variant="caption" color="text.secondary">
                        ⌘ + Enter
                    </Typography>
                </Stack>
                <Button
                    variant="contained"
                    color="primary"
                    onClick={handleRunTest}
                    disabled={isLoading || !isJsonValid}
                    startIcon={isLoading ? <CircularProgress size={16} /> : <PlayArrow />}
                    sx={{
                        textTransform: 'none',
                    }}
                >
                    {isLoading ? 'Running...' : 'Run Test'}
                </Button>
            </Box>

            {/* Error Display */}
            {error && (
                <Alert severity="error" onClose={() => setError(null)}>
                    {error}
                </Alert>
            )}

            {/* Response Display */}
            {response && (
                <Box display="flex" flexDirection="column" gap={1}>
                    <Typography variant="subtitle2" fontWeight="medium">
                        Response
                    </Typography>
                    <Paper
                        variant="outlined"
                        sx={{
                            backgroundColor: theme.palette.background.default,
                            padding: 2,
                            maxHeight: 50,
                            overflow: 'auto',
                        }}
                    >
                        <Typography
                            component="pre"
                            sx={{
                                fontFamily: 'monospace',
                                fontSize: theme.typography.body2.fontSize,
                                margin: 0,
                                whiteSpace: 'pre-wrap',
                                wordBreak: 'break-word',
                            }}
                        >
                            {response}
                        </Typography>
                    </Paper>
                </Box>
            )}
        </Box>
    );
}


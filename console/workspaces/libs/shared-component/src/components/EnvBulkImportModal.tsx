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

import { useState, useRef, useCallback, useMemo, ChangeEvent } from "react";
import {
    Box,
    Button,
    Dialog,
    DialogTitle,
    DialogContent,
    DialogActions,
    Typography,
    useTheme,
} from "@wso2/oxygen-ui";
import { FileText, Upload } from "@wso2/oxygen-ui-icons-react";
import { parseEnvContent, EnvVariable } from "../utils";

interface EnvBulkImportModalProps {
    open: boolean;
    onClose: () => void;
    onImport: (envVars: EnvVariable[]) => void;
}

export function EnvBulkImportModal({
    open,
    onClose,
    onImport,
}: EnvBulkImportModalProps) {
    const theme = useTheme();
    const [content, setContent] = useState("");
    const fileInputRef = useRef<HTMLInputElement>(null);

    // Parse content and get variables count
    const parsedVars = useMemo(() => parseEnvContent(content), [content]);
    const variablesCount = parsedVars.length;

    // Handle textarea change
    const handleContentChange = useCallback(
        (e: ChangeEvent<HTMLTextAreaElement>) => {
            setContent(e.target.value);
        },
        []
    );

    // Handle file upload
    const handleFileUpload = useCallback(
        (e: ChangeEvent<HTMLInputElement>) => {
            const file = e.target.files?.[0];
            if (!file) return;

            const reader = new FileReader();
            reader.onload = (event) => {
                const text = event.target?.result;
                if (typeof text === "string") {
                    setContent(text);
                }
            };
            reader.readAsText(file);

            // Reset input so same file can be selected again
            e.target.value = "";
        },
        []
    );

    // Trigger file input click
    const handleUploadClick = useCallback(() => {
        fileInputRef.current?.click();
    }, []);

    // Handle import button click
    const handleImport = useCallback(() => {
        if (variablesCount > 0) {
            onImport(parsedVars);
            setContent("");
            onClose();
        }
    }, [variablesCount, parsedVars, onImport, onClose]);

    // Handle cancel/close
    const handleClose = useCallback(() => {
        setContent("");
        onClose();
    }, [onClose]);

    return (
        <Dialog
            open={open}
            onClose={handleClose}
            maxWidth="sm"
            fullWidth
        >
            <DialogTitle>
                <Box display="flex" alignItems="center" gap={1}>
                    <FileText size={20} />
                    <Typography variant="h6">
                        Bulk Import Environment Variables
                    </Typography>
                </Box>
            </DialogTitle>

            <DialogContent>
                <Box display="flex" flexDirection="column" gap={2}>
                    <Typography variant="body2" color="text.secondary">
                        Paste your .env content below or upload a file.
                    </Typography>

                    {/* Textarea for pasting .env content */}
                    <Box
                        component="textarea"
                        value={content}
                        onChange={handleContentChange}
                        placeholder={`# Example format:\nAPI_KEY=your_api_key\nDATABASE_URL=postgres://...\nDEBUG="true"`}
                        sx={{
                            width: "100%",
                            minHeight: 200,
                            padding: 1.5,
                            fontFamily: "monospace",
                            fontSize: 13,
                            border: `1px solid ${theme.palette.divider}`,
                            borderRadius: 1,
                            resize: "vertical",
                            backgroundColor: theme.palette.background.paper,
                            color: theme.palette.text.primary,
                            "&:focus": {
                                outline: "none",
                                borderColor: theme.palette.primary.main,
                            },
                        }}
                    />

                    {/* File upload button */}
                    <Box>
                        <input
                            ref={fileInputRef}
                            type="file"
                            accept=".env,.txt,text/plain"
                            onChange={handleFileUpload}
                            style={{ display: "none" }}
                        />
                        <Button
                            variant="outlined"
                            size="small"
                            startIcon={<Upload size={16} />}
                            onClick={handleUploadClick}
                        >
                            Upload .env File
                        </Button>
                    </Box>

                    {/* Variables count indicator */}
                    <Typography
                        variant="body2"
                        color={variablesCount > 0 ? "success.main" : "text.secondary"}
                    >
                        {variablesCount > 0
                            ? `${variablesCount} variable${variablesCount !== 1 ? "s" : ""} detected`
                            : "No variables detected"}
                    </Typography>
                </Box>
            </DialogContent>

            <DialogActions>
                <Button onClick={handleClose}>Cancel</Button>
                <Button
                    variant="contained"
                    onClick={handleImport}
                    disabled={variablesCount === 0}
                >
                    Import
                </Button>
            </DialogActions>
        </Dialog>
    );
}

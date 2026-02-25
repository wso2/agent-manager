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
    alpha,
    Box,
    Button,
    Dialog,
    DialogTitle,
    DialogContent,
    DialogActions,
    Stack,
    TextField,
    Typography,
    useTheme,
} from "@wso2/oxygen-ui";
import { FileText, Upload } from "@wso2/oxygen-ui-icons-react";
import { parseEnvContent, EnvVariable } from "../utils";

const MAX_FILE_SIZE = 1024 * 1024; // 1MB

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
    const [fileError, setFileError] = useState<string | null>(null);
    const fileInputRef = useRef<HTMLInputElement>(null);

    // Parse content and get variables count
    const parseResult = useMemo(() => parseEnvContent(content), [content]);
    const validCount = parseResult.valid.length;
    const invalidKeys = parseResult.invalid;

    // Handle textarea change
    const handleContentChange = useCallback(
        (e: ChangeEvent<HTMLInputElement | HTMLTextAreaElement>) => {
            setContent(e.target.value);
        },
        []
    );

    // Handle file upload
    const handleFileUpload = useCallback(
        (e: ChangeEvent<HTMLInputElement>) => {
            const file = e.target.files?.[0];
            if (!file) return;

            setFileError(null);

            if (file.size > MAX_FILE_SIZE) {
                setFileError(`File is too large. Maximum size is ${MAX_FILE_SIZE / 1024}KB.`);
                e.target.value = "";
                return;
            }

            const reader = new FileReader();
            reader.onerror = () => {
                setFileError("Failed to read file. Please try again.");
                e.target.value = "";
            };
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
        if (validCount > 0) {
            onImport(parseResult.valid);
            setContent("");
            setFileError(null);
            onClose();
        }
    }, [validCount, parseResult.valid, onImport, onClose]);

    // Handle cancel/close
    const handleClose = useCallback(() => {
        setContent("");
        setFileError(null);
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
                        Import Environment Variables
                    </Typography>
                </Box>
            </DialogTitle>

            <DialogContent>
                <Stack spacing={2} sx={{ pt: 1 }}>
                    <Typography variant="body2" color="text.secondary">
                        Paste your .env content below or upload a file.
                    </Typography>

                    {/* Textarea for pasting .env content */}
                    <TextField
                        multiline
                        minRows={7}
                        fullWidth
                        value={content}
                        onChange={handleContentChange}
                        placeholder={`# Example format:\nAPI_KEY=your_api_key\nDATABASE_URL=postgres://...\nDEBUG="true"`}
                        inputProps={{
                            "aria-label": "Environment variables content",
                            style: { fontFamily: "monospace", fontSize: 13 },
                        }}
                    />

                    {/* File upload button */}
                    <Box>
                        <input
                            ref={fileInputRef}
                            type="file"
                            accept=".env,text/plain"
                            onChange={handleFileUpload}
                            style={{ display: "none" }}
                            aria-label="Upload environment variables file"
                        />
                        <Button
                            variant="outlined"
                            size="small"
                            startIcon={<Upload size={16} />}
                            onClick={handleUploadClick}
                        >
                            Upload .env File
                        </Button>
                        {fileError && (
                            <Typography variant="caption" color="error" sx={{ display: "block", mt: 0.5 }}>
                                {fileError}
                            </Typography>
                        )}
                    </Box>

                    {/* Variables count indicator */}
                    <Typography
                        variant="body2"
                        color={validCount > 0 ? "success.main" : "text.secondary"}
                    >
                        {validCount > 0
                            ? `${validCount} valid variable${validCount !== 1 ? "s" : ""} detected`
                            : "No valid variables detected"}
                    </Typography>

                    {/* Invalid keys warning */}
                    {invalidKeys.length > 0 && (
                        <Box
                            sx={{
                                padding: 1.5,
                                backgroundColor: alpha(theme.palette.error.light, 0.12),
                                borderRadius: 1,
                                border: `1px solid ${theme.palette.error.light}`,
                            }}
                        >
                            <Typography variant="body2" color="error.main" fontWeight="medium">
                                {invalidKeys.length} invalid key{invalidKeys.length !== 1 ? "s" : ""} skipped:
                            </Typography>
                            <Typography variant="body2" color="error.main" sx={{ fontFamily: "monospace", mt: 0.5 }}>
                                {invalidKeys.join(", ")}
                            </Typography>
                            <Typography variant="caption" color="text.secondary" sx={{ display: "block", mt: 0.5 }}>
                                Keys must start with a letter or underscore, and contain only letters, numbers, or underscores.
                            </Typography>
                        </Box>
                    )}
                </Stack>
            </DialogContent>

            <DialogActions>
                <Button onClick={handleClose}>Cancel</Button>
                <Button
                    variant="contained"
                    onClick={handleImport}
                    disabled={validCount === 0}
                >
                    Import
                </Button>
            </DialogActions>
        </Dialog>
    );
}

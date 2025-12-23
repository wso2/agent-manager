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

import { useState, useCallback } from "react";
import { Box, Button, Typography } from "@wso2/oxygen-ui";
import { Plus as Add, FileText } from "@wso2/oxygen-ui-icons-react";
import { EnvVariableEditor } from "@agent-management-platform/views";
import { EnvBulkImportModal } from "./EnvBulkImportModal";
import type { EnvVariable } from "../utils";

interface EnvironmentVariableProps {
    envVariables: Array<{ key: string; value: string }>;
    setEnvVariables: React.Dispatch<React.SetStateAction<Array<{ key: string; value: string }>>>;
    /** When true, the "Add" and "Import" buttons are hidden */
    hideAddButton?: boolean;
    /** When true, key fields are disabled so only values can be edited */
    keyFieldsDisabled?: boolean;
    /** When true, value fields are rendered as password type */
    isValueSecret?: boolean;
    /** Title for the environment variables form */
    title?: string;
    /** Description for the environment variables form */
    description?: string;
}

export const EnvironmentVariable = ({
    envVariables,
    setEnvVariables,
    hideAddButton = false,
    keyFieldsDisabled = false,
    isValueSecret = false,
    title = "Environment Variables (Optional)",
    description = "Set environment variables for your agent deployment.",
}: EnvironmentVariableProps) => {
    const [importModalOpen, setImportModalOpen] = useState(false);
    const isOneEmpty = envVariables.some((e) => !e?.key || !e?.value);

    const handleAdd = () => {
        setEnvVariables((prev) => [...prev, { key: '', value: '' }]);
    };

    const handleRemove = (index: number) => {
        setEnvVariables((prev) => prev.filter((_, i) => i !== index));
    };

    const handleChange = (index: number, field: 'key' | 'value', value: string) => {
        setEnvVariables((prev) =>
            prev.map((item, i) => (i === index ? { ...item, [field]: value } : item))
        );
    };

    const handleImport = useCallback((importedVars: EnvVariable[]) => {
        setEnvVariables((prev) => {
            // Filter out rows with no key (value may be intentionally empty)
            const nonEmpty = prev.filter((env) => env?.key);

            // Build map from existing vars; imported vars override on same key
            const existingMap = new Map<string, string>(nonEmpty.map((env) => [env.key, env.value]));
            importedVars.forEach((v) => existingMap.set(v.key, v.value));

            return Array.from(existingMap.entries()).map(([key, value]) => ({ key, value }));
        });
    }, [setEnvVariables]);

    const handleModalClose = useCallback(() => setImportModalOpen(false), []);

    return (
        <Box display="flex" flexDirection="column" gap={2} width="100%">
            <Typography variant="h6">{title}</Typography>
            <Typography variant="body2">{description}</Typography>
            <Box display="flex" flexDirection="column" gap={2}>
                {envVariables.map((envVar, index: number) => (
                    <EnvVariableEditor
                        key={index}
                        index={index}
                        keyValue={envVar.key}
                        valueValue={envVar.value}
                        onKeyChange={(value) => handleChange(index, 'key', value)}
                        onValueChange={(value) => handleChange(index, 'value', value)}
                        onRemove={() => handleRemove(index)}
                        keyDisabled={keyFieldsDisabled}
                        isValueSecret={isValueSecret}
                    />
                ))}
            </Box>
            {!hideAddButton && (
                <Box display="flex" justifyContent="flex-start" gap={1} width="100%">
                    <Button
                        startIcon={<Add fontSize="small" />}
                        disabled={isOneEmpty}
                        variant="outlined"
                        color="primary"
                        onClick={handleAdd}
                    >
                        Add
                    </Button>
                    <Button
                        startIcon={<FileText fontSize="small" />}
                        variant="outlined"
                        color="primary"
                        onClick={() => setImportModalOpen(true)}
                    >
                        Import
                    </Button>
                </Box>
            )}

            <EnvBulkImportModal
                open={importModalOpen}
                onClose={handleModalClose}
                onImport={handleImport}
            />
        </Box>
    );
};

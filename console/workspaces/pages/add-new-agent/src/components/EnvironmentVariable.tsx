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

import { useState, useCallback, useMemo } from "react";
import { Box, Button, Card, CardContent, Typography } from "@wso2/oxygen-ui";
import { Plus as Add, FileText } from "@wso2/oxygen-ui-icons-react";
import { useFieldArray, useFormContext, useWatch } from "react-hook-form";
import { EnvVariableEditor } from "@agent-management-platform/views";
import { EnvBulkImportModal, EnvVariable } from "@agent-management-platform/shared-component";

export const EnvironmentVariable = () => {
    const { control, formState: { errors }, register, setValue } = useFormContext();
    const { fields, append, remove } = useFieldArray({ control, name: 'env' });
    const watchedEnvValues = useWatch({ control, name: 'env' });
    const [importModalOpen, setImportModalOpen] = useState(false);

    // Memoize envValues to stabilize dependency for useCallback
    const envValues = useMemo(
        () => (watchedEnvValues || []) as EnvVariable[],
        [watchedEnvValues]
    );

    const isOneEmpty = envValues.some((e) => !e?.key || !e?.value);

    // Handle bulk import - merge imported vars with existing ones
    const handleImport = useCallback((importedVars: EnvVariable[]) => {
        const existingMap = new Map<string, number>();

        // Map existing keys to their indices
        envValues.forEach((env, index) => {
            if (env?.key) {
                existingMap.set(env.key, index);
            }
        });

        // Process imported variables
        const updatedEnv = [...envValues];
        const newVars: EnvVariable[] = [];

        importedVars.forEach((imported) => {
            if (existingMap.has(imported.key)) {
                // Update existing variable
                const idx = existingMap.get(imported.key)!;
                updatedEnv[idx] = { key: imported.key, value: imported.value };
            } else {
                // Add new variable
                newVars.push(imported);
            }
        });

        // Set updated values and append new ones
        setValue('env', updatedEnv);
        newVars.forEach((v) => append(v));
    }, [envValues, setValue, append]);

    return (
        <Card variant="outlined">
            <CardContent>
                <Box display="flex" flexDirection="row" alignItems="center" gap={1}>
                    <Typography variant="h5">
                        Environment Variables (Optional)
                    </Typography>
                </Box>
                <Box display="flex" flexDirection="column" py={2} gap={2}>
                    {fields.map((field, index) => (
                        <EnvVariableEditor
                            key={field.id}
                            fieldName="env"
                            index={index}
                            fieldId={field.id}
                            register={register}
                            errors={errors}
                            onRemove={() => remove(index)}
                        />
                    ))}
                </Box>
                <Box display="flex" gap={1}>
                    <Button startIcon={<Add fontSize="small" />} disabled={isOneEmpty} variant="outlined" color="primary" onClick={() => append({ key: '', value: '' })}>
                        Add
                    </Button>
                    <Button startIcon={<FileText fontSize="small" />} variant="outlined" color="primary" onClick={() => setImportModalOpen(true)}>
                        Bulk Import
                    </Button>
                </Box>

                <EnvBulkImportModal
                    open={importModalOpen}
                    onClose={() => setImportModalOpen(false)}
                    onImport={handleImport}
                />
            </CardContent>
        </Card>
    );
};

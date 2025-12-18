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

import { Box, Button, Typography } from "@wso2/oxygen-ui";
import { Plus as Add } from "@wso2/oxygen-ui-icons-react";
import { useFieldArray, useFormContext, useWatch } from "react-hook-form";
import { EnvVariableEditor } from "@agent-management-platform/views";

export const EnvironmentVariable = () => {
    const { control, formState: { errors }, register } = useFormContext();
    const { fields, append, remove } = useFieldArray({ control, name: 'env' });
    const envValues = useWatch({ control, name: 'env' }) || [];

    const isOneEmpty = envValues.some((e: any) => !e?.key || !e?.value);

    return (
        <Box display="flex" flexDirection="column" gap={2} width="100%">
            <Typography variant="h6">
                Environment Variables (Optional)
            </Typography>
            <Typography variant="body2">
                Set environment variables for your agent deployment.
            </Typography>
            <Box display="flex" flexDirection="column" gap={2}>
                {fields.map((field: any, index: number) => (
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
            <Box display="flex" justifyContent="flex-start" width="100%">
                <Button
                    startIcon={<Add fontSize="small" />}
                    disabled={isOneEmpty}
                    variant="outlined"
                    color="primary"
                    onClick={() => append({ key: '', value: '' })}
                >
                    Add
                </Button>
            </Box>
        </Box>
    );
};


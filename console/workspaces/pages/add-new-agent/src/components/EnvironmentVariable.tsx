import { Box, Button, Card, CardContent, Typography } from "@wso2/oxygen-ui";
import { Plus as Add } from "@wso2/oxygen-ui-icons-react";
import { useFieldArray, useFormContext, useWatch } from "react-hook-form";
import { EnvVariableEditor } from "@agent-management-platform/views";

export const EnvironmentVariable = () => {
    const { control, formState: { errors }, register } = useFormContext();
    const { fields, append, remove } = useFieldArray({ control, name: 'env' });
    const envValues = useWatch({ control, name: 'env' }) || [];

    const isOneEmpty = envValues.some((e: any) => !e?.key || !e?.value);

    return (
        <Card variant="outlined">
            <CardContent>
                <Box display="flex" flexDirection="row" alignItems="center" gap={1}>
                    <Typography variant="h5">
                        Environment Variables (Optional)
                    </Typography>
                </Box>
                <Box display="flex" flexDirection="column" py={2} gap={2}>
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
                <Button startIcon={<Add fontSize="small" />} disabled={isOneEmpty} variant="outlined" color="primary" onClick={() => append({ key: '', value: '' })}>
                    Add
                </Button>
            </CardContent>
        </Card>
    );
};

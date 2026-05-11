import React from "react";
import {
    Box,
    Button,
    FormControlLabel,
    IconButton,
    MenuItem,
    Select,
    Stack,
    Switch,
} from "@wso2/oxygen-ui";
import { Plus, X as CloseIcon } from "@wso2/oxygen-ui-icons-react";
import { TextInput } from "@agent-management-platform/views";

export type RuntimeConfigTypeOption = "string" | "number" | "boolean";

export interface RuntimeConfigRow {
    key: string;
    type: RuntimeConfigTypeOption;
    isSecrete: boolean;
}

export interface RuntimeConfigEditorProps {
    rows: RuntimeConfigRow[];
    onChange: (rows: RuntimeConfigRow[]) => void;
}

export const RuntimeConfigEditor: React.FC<RuntimeConfigEditorProps> = ({ rows, onChange }) => {
    const updateRow = <K extends keyof RuntimeConfigRow>(index: number, field: K, value: RuntimeConfigRow[K]) => {
        const next = [...rows];
        next[index] = { ...next[index], [field]: value };
        onChange(next);
    };

    const addRow = () => onChange([...rows, { key: "", type: "string", isSecrete: false }]);

    const removeRow = (index: number) => onChange(rows.filter((_, i) => i !== index));

    return (
        <Stack spacing={1}>
            {rows.map((row, i) => (
                <Stack key={i} direction="row" spacing={1} alignItems="center">
                    <Box sx={{ width: 200 }}>
                    <TextInput
                        placeholder="Key"
                        value={row.key}
                        onChange={(e) => updateRow(i, "key", e.target.value)}
                        fullWidth
                        size="small"
                    />
                    </Box>
                    <Select
                        size="small"
                        value={row.type}
                        onChange={(e) => updateRow(i, "type", e.target.value as RuntimeConfigTypeOption)}
                        sx={{ maxWidth: 130, width: 130 }}
                    >
                        <MenuItem value="string">string</MenuItem>
                        <MenuItem value="number">number</MenuItem>
                        <MenuItem value="boolean">boolean</MenuItem>
                    </Select>
                    <FormControlLabel
                        control={
                            <Switch
                                size="small"
                                checked={row.isSecrete}
                                onChange={(_, checked) => updateRow(i, "isSecrete", checked)}
                            />
                        }
                        label="Secret"
                        sx={{ mr: 0, minWidth: 90 }}
                    />
                    <IconButton
                        size="small"
                        onClick={() => removeRow(i)}
                        disabled={rows.length === 1}
                        aria-label="Remove row"
                    >
                        <CloseIcon size={16} />
                    </IconButton>
                </Stack>
            ))}
            <Box>
                <Button size="small" variant="outlined" startIcon={<Plus />} onClick={addRow}>
                    Add Runtime Key
                </Button>
            </Box>
        </Stack>
    );
};

export default RuntimeConfigEditor;

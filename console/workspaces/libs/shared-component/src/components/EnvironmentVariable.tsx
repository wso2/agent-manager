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

import { useState } from "react";
import {
  Alert,
  Box,
  Button,
  Card,
  Checkbox,
  Chip,
  FormControlLabel,
  IconButton,
  InputAdornment,
  Typography,
} from "@wso2/oxygen-ui";
import {
  Plus as Add,
  Edit,
  Trash2 as DeleteIcon,
  Eye,
  EyeOff,
} from "@wso2/oxygen-ui-icons-react";
import { TextInput } from "@agent-management-platform/views";

export interface EnvVariableItem {
  key: string;
  value: string;
  isSensitive?: boolean;
  secretRef?: string;
  /** Tracks if a secret value has been edited (for existing secrets) */
  isSecretEdited?: boolean;
}

interface EnvironmentVariableProps {
  envVariables: Array<EnvVariableItem>;
  setEnvVariables: React.Dispatch<React.SetStateAction<Array<EnvVariableItem>>>;
  /** When true, the "Add" button is hidden  */
  hideAddButton?: boolean;
  /** When true, key fields are disabled so only values can be edited */
  keyFieldsDisabled?: boolean;
  /** When true, value fields are rendered as password type */
  isValueSecret?: boolean;
  /** Title for the environment variables form */
  title?: string;
  /** Description for the environment variables form */
  description?: string;
  /** When true, sensitive env variables are treated as existing secrets (locked by default) */
  isExistingData?: boolean;
  /** When true, the title is hidden */
  hideTitle?: boolean;
}

interface NewEnvVarForm {
  key: string;
  value: string;
  isSensitive: boolean;
}

export const EnvironmentVariable = ({
  envVariables,
  setEnvVariables,
  hideAddButton = false,
  title = "Environment Variables (Optional)",
  hideTitle = false,
  description = "Set environment variables for your agent deployment.",
  isExistingData = false,
}: EnvironmentVariableProps) => {
  const [isAddFormOpen, setIsAddFormOpen] = useState(false);
  const [newEnvVar, setNewEnvVar] = useState<NewEnvVarForm>({
    key: "",
    value: "",
    isSensitive: false,
  });
  const [editingIndex, setEditingIndex] = useState<number | null>(null);
  const [editValue, setEditValue] = useState("");
  const [showEditPassword, setShowEditPassword] = useState(false);
  const [showNewPassword, setShowNewPassword] = useState(false);

  const handleOpenAddForm = () => {
    setIsAddFormOpen(true);
    setNewEnvVar({ key: "", value: "", isSensitive: false });
  };

  const handleCancelAdd = () => {
    setIsAddFormOpen(false);
    setNewEnvVar({ key: "", value: "", isSensitive: false });
    setShowNewPassword(false);
  };

  const handleAdd = () => {
    if (newEnvVar.key && newEnvVar.value) {
      setEnvVariables((prev) => [...prev, { ...newEnvVar }]);
      setIsAddFormOpen(false);
      setNewEnvVar({ key: "", value: "", isSensitive: false });
      setShowNewPassword(false);
    }
  };

  const handleRemove = (index: number) => {
    setEnvVariables((prev) => prev.filter((_, i) => i !== index));
  };

  const handleStartEdit = (index: number) => {
    setEditingIndex(index);
    // For secrets, start with empty value; for non-secrets, prefill with current value
    const envVar = envVariables[index];
    setEditValue(envVar.isSensitive ? "" : envVar.value);
    setShowEditPassword(false);
  };

  const handleCancelEdit = () => {
    setEditingIndex(null);
    setEditValue("");
    setShowEditPassword(false);
  };

  const handleSaveEdit = (index: number) => {
    if (editValue) {
      setEnvVariables((prev) =>
        prev.map((item, i) =>
          i === index
            ? { ...item, value: editValue, isSecretEdited: item.isSensitive }
            : item,
        ),
      );
    }
    setEditingIndex(null);
    setEditValue("");
    setShowEditPassword(false);
  };

  const isAddDisabled = !newEnvVar.key || !newEnvVar.value;

  return (
    <Box display="flex" flexDirection="column" gap={2} width="100%">
      {!hideTitle && <Typography variant="h6">{title}</Typography>}
      <Typography variant="body2">{description}</Typography>

      {/* Existing variables as read-only cards */}
      <Box display="flex" flexDirection="column" gap={1.5}>
        {envVariables.map((envVar, index: number) => {
          const isEditing = editingIndex === index;
          const isSecret = envVar.isSensitive;

          return (
            <Card key={index} variant="outlined" sx={{ p: 2 }}>
              {isEditing ? (
                // Edit mode
                <Box display="flex" flexDirection="column" gap={2}>
                  <Box display="flex" alignItems="center" gap={1}>
                    <Typography variant="body2" fontWeight={500}>
                      Name:
                    </Typography>
                    <Typography variant="body2">{envVar.key}</Typography>
                    {isSecret && (
                      <Chip
                        label="Secret"
                        size="small"
                        color="warning"
                        variant="outlined"
                      />
                    )}
                  </Box>
                  <TextInput
                    label="New Value"
                    type={isSecret && !showEditPassword ? "password" : "text"}
                    fullWidth
                    size="small"
                    value={editValue}
                    onChange={(e) => setEditValue(e.target.value)}
                    placeholder="Enter new value"
                    slotProps={
                      isSecret
                        ? {
                            input: {
                              endAdornment: (
                                <InputAdornment position="end">
                                  <IconButton
                                    size="small"
                                    onClick={() =>
                                      setShowEditPassword(!showEditPassword)
                                    }
                                    edge="end"
                                  >
                                    {showEditPassword ? (
                                      <EyeOff size={18} />
                                    ) : (
                                      <Eye size={18} />
                                    )}
                                  </IconButton>
                                </InputAdornment>
                              ),
                            },
                          }
                        : undefined
                    }
                  />
                  {isSecret && (
                    <Alert severity="warning" sx={{ py: 0.5 }}>
                      Updating a Secret value removes the previous value
                      permanently and cannot be restored.
                    </Alert>
                  )}
                  <Box display="flex" justifyContent="flex-end" gap={1}>
                    <Button
                      variant="outlined"
                      size="small"
                      onClick={handleCancelEdit}
                    >
                      Cancel
                    </Button>
                    <Button
                      variant="contained"
                      size="small"
                      onClick={() => handleSaveEdit(index)}
                      disabled={!editValue}
                    >
                      Update
                    </Button>
                  </Box>
                </Box>
              ) : (
                // Read-only view
                <Box
                  display="flex"
                  justifyContent="space-between"
                  alignItems="flex-start"
                >
                  <Box>
                    <Box display="flex" alignItems="center" gap={1} mb={0.5}>
                      <Typography variant="body2" fontWeight={500}>
                        Name:
                      </Typography>
                      <Typography variant="body2">{envVar.key}</Typography>
                      {isSecret && (
                        <Chip
                          label="Secret"
                          size="small"
                          color="warning"
                          variant="outlined"
                        />
                      )}
                    </Box>
                    <Box display="flex" alignItems="center" gap={1}>
                      <Typography variant="body2" fontWeight={500}>
                        Value:
                      </Typography>
                      <Typography variant="body2">
                        {isSecret ? "••••••••" : envVar.value}
                      </Typography>
                    </Box>
                  </Box>
                  <Box display="flex" gap={0.5}>
                    {isExistingData && (
                      <IconButton
                        size="small"
                        color="primary"
                        onClick={() => handleStartEdit(index)}
                        title="Edit value"
                      >
                        <Edit size={18} />
                      </IconButton>
                    )}
                    <IconButton
                      size="small"
                      color="error"
                      onClick={() => handleRemove(index)}
                      title="Delete"
                    >
                      <DeleteIcon size={18} />
                    </IconButton>
                  </Box>
                </Box>
              )}
            </Card>
          );
        })}
      </Box>

      {/* Add new variable form */}
      {isAddFormOpen && (
        <Card
          variant="outlined"
          sx={{ p: 2, borderColor: "primary.main", borderWidth: 2 }}
        >
          <Box display="flex" flexDirection="column" gap={2}>
            <TextInput
              label="Name"
              fullWidth
              size="small"
              value={newEnvVar.key}
              onChange={(e) =>
                setNewEnvVar((prev) => ({
                  ...prev,
                  key: e.target.value.replace(/\s/g, "_"),
                }))
              }
              placeholder="Enter a new key"
            />
            <TextInput
              label="Value"
              type={
                newEnvVar.isSensitive && !showNewPassword ? "password" : "text"
              }
              fullWidth
              size="small"
              value={newEnvVar.value}
              onChange={(e) =>
                setNewEnvVar((prev) => ({ ...prev, value: e.target.value }))
              }
              placeholder="Enter a value"
              slotProps={
                newEnvVar.isSensitive
                  ? {
                      input: {
                        endAdornment: (
                          <InputAdornment position="end">
                            <IconButton
                              size="small"
                              onClick={() =>
                                setShowNewPassword(!showNewPassword)
                              }
                              edge="end"
                            >
                              {showNewPassword ? (
                                <EyeOff size={18} />
                              ) : (
                                <Eye size={18} />
                              )}
                            </IconButton>
                          </InputAdornment>
                        ),
                      },
                    }
                  : undefined
              }
            />
            <FormControlLabel
              control={
                <Checkbox
                  checked={newEnvVar.isSensitive}
                  onChange={(e) =>
                    setNewEnvVar((prev) => ({
                      ...prev,
                      isSensitive: e.target.checked,
                    }))
                  }
                />
              }
              label="Mark as a Secret"
            />
            <Box display="flex" justifyContent="flex-end" gap={1}>
              <Button variant="outlined" onClick={handleCancelAdd}>
                Cancel
              </Button>
              <Button
                variant="contained"
                onClick={handleAdd}
                disabled={isAddDisabled}
              >
                Add
              </Button>
            </Box>
          </Box>
        </Card>
      )}

      {/* Add button */}
      {!hideAddButton && !isAddFormOpen && (
        <Box display="flex" justifyContent="flex-start" width="100%">
          <Button
            startIcon={<Add fontSize="small" />}
            variant="outlined"
            color="primary"
            onClick={handleOpenAddForm}
          >
            Add
          </Button>
        </Box>
      )}
    </Box>
  );
};

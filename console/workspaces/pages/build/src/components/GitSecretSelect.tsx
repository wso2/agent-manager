/**
 * Copyright (c) 2026, WSO2 LLC. (https://www.wso2.com).
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

import { useState, useCallback } from 'react';
import {
  FormControl,
  MenuItem,
  Select,
  Typography,
  Stack,
  Button,
  CircularProgress,
  FormHelperText,
  Box,
} from '@wso2/oxygen-ui';
import { useListGitSecrets } from '@agent-management-platform/api-client';
import { CreateGitSecretModal } from './CreateGitSecretModal';

interface GitSecretSelectProps {
  orgId: string;
  value: string | undefined;
  onChange: (value: string | undefined) => void;
  error?: string;
  disabled?: boolean;
}

export const GitSecretSelect = ({
  orgId,
  value,
  onChange,
  error,
  disabled,
}: GitSecretSelectProps) => {
  const [isModalOpen, setIsModalOpen] = useState(false);

  const {
    data: gitSecretsData,
    isLoading,
    refetch,
  } = useListGitSecrets({ orgName: orgId }, { limit: 100 });

  const secrets = gitSecretsData?.secrets || [];

  const handleSecretChange = useCallback(
    (event: { target: { value: string } }) => {
      const selectedValue = event.target.value;
      if (selectedValue === '__create_new__') {
        setIsModalOpen(true);
        return;
      }
      onChange(selectedValue || undefined);
    },
    [onChange]
  );

  const handleSecretCreated = useCallback(
    (secretName: string) => {
      onChange(secretName);
      refetch();
      setIsModalOpen(false);
    },
    [onChange, refetch]
  );

  return (
    <>
      <Box>
        <Typography variant="body2" sx={{ mb: 0.5, fontWeight: 500 }}>
          Git Secret (Optional)
        </Typography>
        <FormControl fullWidth error={!!error} size="small">
          <Select
            id="gitSecretRef"
            value={value || ''}
            onChange={handleSecretChange}
            displayEmpty
            disabled={disabled || isLoading}
            startAdornment={
              isLoading ? (
                <CircularProgress size={20} sx={{ mr: 1 }} />
              ) : undefined
            }
          >
            <MenuItem value="">
              <em>Public Repo (None)</em>
            </MenuItem>
            {secrets.map((secret) => (
              <MenuItem key={secret.name} value={secret.name}>
                <Typography>{secret.name}</Typography>
              </MenuItem>
            ))}
            <MenuItem value="__create_new__">
              <Stack direction="row" spacing={1} alignItems="center">
                <Button
                  variant="text"
                  size="small"
                  sx={{ textTransform: 'none', p: 0 }}
                >
                  + Create new git secret
                </Button>
              </Stack>
            </MenuItem>
          </Select>
          {error ? (
            <FormHelperText error>{error}</FormHelperText>
          ) : (
            <FormHelperText>
              Select a git secret for private repository authentication
            </FormHelperText>
          )}
        </FormControl>
      </Box>

      <CreateGitSecretModal
        open={isModalOpen}
        onClose={() => setIsModalOpen(false)}
        onSecretCreated={handleSecretCreated}
        orgId={orgId}
      />
    </>
  );
};

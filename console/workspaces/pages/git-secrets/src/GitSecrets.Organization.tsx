/**
 * Copyright (c) 2026, WSO2 LLC. (https://www.wso2.com).
 *
 * WSO2 LLC. licenses this file to you under the Apache License,
 * Version 2.0 (the "License"); you may not use this file except
 * in compliance with the License. You may obtain a copy of the License at
 *
 * http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing,
 * software distributed under the License is distributed on an
 * "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
 * KIND, either express or implied. See the License for the
 * specific language governing permissions and limitations
 * under the License.
 */

import React, { useState, useCallback } from "react";
import { useParams } from "react-router-dom";
import {
  Button,
  Stack,
  Typography,
  Paper,
  Table,
  TableBody,
  TableCell,
  TableContainer,
  TableHead,
  TableRow,
  IconButton,
  Chip,
  CircularProgress,
  Alert,
  Dialog,
  DialogTitle,
  DialogContent,
  DialogContentText,
  DialogActions,
} from "@wso2/oxygen-ui";
import { Delete } from "@wso2/oxygen-ui-icons-react";
import { PageLayout } from "@agent-management-platform/views";
import {
  useListGitSecrets,
  useDeleteGitSecret,
} from "@agent-management-platform/api-client";
import type { GitSecretResponse } from "@agent-management-platform/types";
import { CreateGitSecretModal } from "./components/CreateGitSecretModal";

export const GitSecretsOrganization: React.FC = () => {
  const { orgId } = useParams<{ orgId: string }>();
  const [isCreateModalOpen, setIsCreateModalOpen] = useState(false);
  const [deleteConfirmOpen, setDeleteConfirmOpen] = useState(false);
  const [secretToDelete, setSecretToDelete] = useState<string | null>(null);

  const {
    data: gitSecretsData,
    isLoading,
    error,
    refetch,
  } = useListGitSecrets({ orgName: orgId }, { limit: 100 });

  const { mutate: deleteSecret, isPending: isDeleting } = useDeleteGitSecret();

  const secrets = gitSecretsData?.secrets || [];

  const handleCreateClick = useCallback(() => {
    setIsCreateModalOpen(true);
  }, []);

  const handleCreateModalClose = useCallback(() => {
    setIsCreateModalOpen(false);
  }, []);

  const handleSecretCreated = useCallback(() => {
    setIsCreateModalOpen(false);
    refetch();
  }, [refetch]);

  const handleDeleteClick = useCallback((secretName: string) => {
    setSecretToDelete(secretName);
    setDeleteConfirmOpen(true);
  }, []);

  const handleDeleteConfirm = useCallback(() => {
    if (secretToDelete) {
      deleteSecret(
        { orgName: orgId, secretName: secretToDelete },
        {
          onSuccess: () => {
            setDeleteConfirmOpen(false);
            setSecretToDelete(null);
            refetch();
          },
        }
      );
    }
  }, [secretToDelete, orgId, deleteSecret, refetch]);

  const handleDeleteCancel = useCallback(() => {
    setDeleteConfirmOpen(false);
    setSecretToDelete(null);
  }, []);

  const formatDate = (dateString: string) => {
    try {
      return new Date(dateString).toLocaleDateString("en-US", {
        year: "numeric",
        month: "short",
        day: "numeric",
      });
    } catch {
      return dateString;
    }
  };

  const renderContent = () => {
    if (isLoading) {
      return (
        <Stack alignItems="center" justifyContent="center" sx={{ py: 8 }}>
          <CircularProgress />
          <Typography variant="body2" color="text.secondary" sx={{ mt: 2 }}>
            Loading git secrets...
          </Typography>
        </Stack>
      );
    }

    if (error) {
      return (
        <Alert severity="error" sx={{ m: 2 }}>
          Failed to load git secrets. Please try again later.
        </Alert>
      );
    }

    if (secrets.length === 0) {
      return (
        <Stack alignItems="center" justifyContent="center" sx={{ py: 8 }}>
          <Typography variant="h6" color="text.secondary">
            No git secrets configured
          </Typography>
          <Typography
            variant="body2"
            color="text.secondary"
            sx={{ mt: 1, mb: 3 }}
          >
            Create a git secret to access private repositories when building
            agents.
          </Typography>
          <Button variant="contained" onClick={handleCreateClick}>
            Create Git Secret
          </Button>
        </Stack>
      );
    }

    return (
      <TableContainer component={Paper} variant="outlined">
        <Table>
          <TableHead>
            <TableRow>
              <TableCell>Name</TableCell>
              <TableCell>Type</TableCell>
              <TableCell>Created</TableCell>
              <TableCell align="right">Actions</TableCell>
            </TableRow>
          </TableHead>
          <TableBody>
            {secrets.map((secret: GitSecretResponse) => (
              <TableRow key={secret.name} hover>
                <TableCell>
                  <Typography fontWeight="medium">{secret.name}</Typography>
                </TableCell>
                <TableCell>
                  <Chip
                    label="Basic Auth"
                    size="small"
                    variant="outlined"
                    color="primary"
                  />
                </TableCell>
                <TableCell>{formatDate(secret.createdAt)}</TableCell>
                <TableCell align="right">
                  <IconButton
                    size="small"
                    onClick={() => handleDeleteClick(secret.name)}
                    color="error"
                    disabled={isDeleting}
                  >
                    <Delete />
                  </IconButton>
                </TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
      </TableContainer>
    );
  };

  return (
    <PageLayout title="Git Secrets" disableIcon>
      <Stack spacing={3}>
        <Stack
          direction="row"
          justifyContent="space-between"
          alignItems="center"
        >
          <Typography variant="body1" color="text.secondary">
            Manage credentials for accessing private Git repositories. These
            secrets can be used when creating agents from private repositories.
          </Typography>
          {secrets.length > 0 && (
            <Button variant="contained" onClick={handleCreateClick}>
              Create Secret
            </Button>
          )}
        </Stack>

        {renderContent()}
      </Stack>

      <CreateGitSecretModal
        open={isCreateModalOpen}
        onClose={handleCreateModalClose}
        onSecretCreated={handleSecretCreated}
      />

      <Dialog open={deleteConfirmOpen} onClose={handleDeleteCancel}>
        <DialogTitle>Delete Git Secret</DialogTitle>
        <DialogContent>
          <DialogContentText>
            Are you sure you want to delete the git secret "{secretToDelete}"?
            This action cannot be undone. Any agents using this secret will no
            longer be able to access their private repositories.
          </DialogContentText>
        </DialogContent>
        <DialogActions>
          <Button onClick={handleDeleteCancel} disabled={isDeleting}>
            Cancel
          </Button>
          <Button
            onClick={handleDeleteConfirm}
            color="error"
            variant="contained"
            disabled={isDeleting}
          >
            {isDeleting ? "Deleting..." : "Delete"}
          </Button>
        </DialogActions>
      </Dialog>
    </PageLayout>
  );
};

export default GitSecretsOrganization;

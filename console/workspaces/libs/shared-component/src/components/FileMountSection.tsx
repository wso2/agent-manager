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

import { Box, Button } from "@wso2/oxygen-ui";
import { Plus as Add } from "@wso2/oxygen-ui-icons-react";
import { FileMountEditor } from "@agent-management-platform/views";

export interface FileMountItem {
  key: string;
  mountPath: string;
  value: string;
  isSensitive?: boolean;
  secretRef?: string;
  isSecretEdited?: boolean;
}

interface FileMountSectionProps {
  fileMounts: FileMountItem[];
  setFileMounts: React.Dispatch<React.SetStateAction<FileMountItem[]>>;
}

export function FileMountSection({
  fileMounts,
  setFileMounts,
}: FileMountSectionProps) {
  const isOneEmpty = fileMounts.some((f) => !f.key || !f.mountPath);

  const handleAdd = () => {
    setFileMounts((prev) => [
      ...prev,
      { key: "", mountPath: "", value: "", isSensitive: false },
    ]);
  };

  const handleRemove = (index: number) => {
    setFileMounts((prev) => prev.filter((_, i) => i !== index));
  };

  const handleChange = (
    index: number,
    field: "key" | "mountPath" | "value" | "isSensitive",
    value: string | boolean,
  ) => {
    setFileMounts((prev) =>
      prev.map((item, i) =>
        i === index
          ? {
              ...item,
              [field]: value,
              ...(field === "value" && item.secretRef
                ? { isSecretEdited: true }
                : {}),
            }
          : item,
      ),
    );
  };

  return (
    <Box display="flex" flexDirection="column" gap={2}>
      {fileMounts.map((item, index) => {
        const siblingKeys = new Set(
          fileMounts.flatMap((f, i) =>
            i !== index && f.key ? [f.key] : [],
          ),
        );
        const keyError =
          item.key && siblingKeys.has(item.key)
            ? "Duplicate file name"
            : undefined;
        return (
          <FileMountEditor
            key={`file-${index}`}
            index={index}
            keyValue={item.key}
            mountPathValue={item.mountPath}
            contentValue={item.value}
            isSensitive={item.isSensitive}
            onKeyChange={(v) => handleChange(index, "key", v)}
            onMountPathChange={(v) => handleChange(index, "mountPath", v)}
            onContentChange={(v) => handleChange(index, "value", v)}
            onSensitiveChange={(v) => handleChange(index, "isSensitive", v)}
            onRemove={() => handleRemove(index)}
            keyError={keyError}
            isExistingSecret={!!item.secretRef}
          />
        );
      })}
      <Box>
        <Button
          startIcon={<Add fontSize="small" />}
          disabled={isOneEmpty}
          variant="outlined"
          color="primary"
          size="small"
          onClick={handleAdd}
        >
          Add File
        </Button>
      </Box>
    </Box>
  );
}

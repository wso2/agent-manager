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

import { Box, IconButton, Tooltip } from "@wso2/oxygen-ui";
import { Copy as ContentCopy } from "@wso2/oxygen-ui-icons-react";
import { useState } from "react";
import { ConsoleAction, useTrack } from "@agent-management-platform/api-client";
import { Prism as SyntaxHighlighter } from "react-syntax-highlighter";
import { materialOceanic } from "react-syntax-highlighter/dist/esm/styles/prism";

interface CodeBlockProps {
    code: string;
    language?: string;
    showCopyButton?: boolean;
    /**
     * Identifies this block among its siblings, so only the one just copied
     * shows "Copied!". May be interpolated per item when a list renders several
     * blocks — which is exactly why it must not be used as the analytics label.
     */
    fieldId?: string;
    /**
     * Stable analytics label for this snippet, e.g. "token-curl". Must be a
     * constant: it becomes the snippet_type dimension, so an interpolated value
     * would export whatever it interpolates (a customer's provider or agent
     * name) and shard the metric one bucket per item. Defaults to fieldId,
     * which is safe only while that is itself a constant — pass this explicitly
     * whenever fieldId is not.
     */
    analyticsId?: string;
}

export const CodeBlock = ({
    code,
    language = "bash",
    showCopyButton = true,
    fieldId = "code",
    analyticsId
}: CodeBlockProps) => {
    const [copiedField, setCopiedField] = useState<string | null>(null);
    const { track } = useTrack();

    const handleCopy = async () => {
        try {
            await navigator.clipboard.writeText(code);
            setCopiedField(fieldId);
            // Which snippet was taken, never its contents — these blocks render
            // connection strings, setup commands and, on some pages, keys.
            track(ConsoleAction.CopySnippet, {
                snippet_type: analyticsId ?? fieldId,
                language,
            });
            setTimeout(() => setCopiedField(null), 2000);
        } catch {
            // Failed to copy - silently fail
        }
    };

    return (
        <Box
            sx={{
                position: 'relative',
                borderRadius: 1,
                overflow: 'hidden',
                '& pre': {
                    margin: 0,
                    padding: 2,
                    borderRadius: 1,
                    fontSize: '0.875rem',
                }
            }}
        >
            {showCopyButton && (
                <Tooltip title={copiedField === fieldId ? 'Copied!' : 'Copy code'}>
                    <IconButton
                        onClick={handleCopy}
                        size="small"
                        sx={{
                            position: 'absolute',
                            right: 8,
                            top: 8,
                            zIndex: 1,
                            color: 'grey.400',
                        }}
                    >
                        <ContentCopy size={16} />
                    </IconButton>
                </Tooltip>
            )}
            <SyntaxHighlighter
                language={language}
                style={
                   materialOceanic
                }
                customStyle={{
                    margin: 0
                }}
            >
                {code}
            </SyntaxHighlighter>
        </Box>
    );
};

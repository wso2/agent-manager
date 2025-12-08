import { Box, IconButton, Tooltip } from "@wso2/oxygen-ui";
import { Copy as ContentCopy } from "@wso2/oxygen-ui-icons-react";
import { useState } from "react";
import { Prism as SyntaxHighlighter } from "react-syntax-highlighter";
import { materialOceanic } from "react-syntax-highlighter/dist/esm/styles/prism";

interface CodeBlockProps {
    code: string;
    language?: string;
    showCopyButton?: boolean;
    fieldId?: string;
}

export const CodeBlock = ({ 
    code, 
    language = "bash", 
    showCopyButton = true,
    fieldId = "code" 
}: CodeBlockProps) => {
    const [copiedField, setCopiedField] = useState<string | null>(null);

    const handleCopy = async () => {
        try {
            await navigator.clipboard.writeText(code);
            setCopiedField(fieldId);
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


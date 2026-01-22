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

import { Box } from "@wso2/oxygen-ui";
import Markdown from "react-markdown";

interface MarkdownViewProps {
  content: string;
}

export function MarkdownView({ content }: MarkdownViewProps) {
  return (
    <Box
      sx={{
        fontSize: "0.75rem",
        color: "text.secondary",
        lineHeight: 1.5,
        "& p": {
          margin: 0,
          marginBottom: "0.5rem",
          "&:last-child": {
            marginBottom: 0,
          },
        },
        "& h1, & h2, & h3, & h4, & h5, & h6": {
          margin: 0,
          marginTop: "0.75rem",
          marginBottom: "0.5rem",
          fontSize: "0.875rem",
          fontWeight: 600,
          color: "text.secondary",
          "&:first-child": {
            marginTop: 0,
          },
        },
        "& h1": { fontSize: "0.875rem" },
        "& h2": { fontSize: "0.8125rem" },
        "& h3": { fontSize: "0.8125rem" },
        "& h4, & h5, & h6": { fontSize: "0.75rem" },
        "& strong, & b": {
          fontWeight: 500,
          color: "text.primary",
        },
        "& em, & i": {
          fontStyle: "italic",
        },
        "& ul, & ol": {
          margin: 0,
          marginBottom: "0.5rem",
          paddingLeft: "1.25rem",
          "&:last-child": {
            marginBottom: 0,
          },
        },
        "& li": {
          marginBottom: "0.25rem",
          "&:last-child": {
            marginBottom: 0,
          },
        },
        "& a": {
          color: "text.secondary",
          textDecoration: "underline",
          "&:hover": {
            color: "text.primary",
          },
        },
        "& blockquote": {
          margin: 0,
          marginBottom: "0.5rem",
          paddingLeft: "0.75rem",
          borderLeft: "2px solid",
          borderColor: "divider",
          color: "text.secondary",
          fontStyle: "italic",
        },
        "& hr": {
          margin: "0.5rem 0",
          border: "none",
          borderTop: "1px solid",
          borderColor: "divider",
        },
        "& pre": {
          backgroundColor: "action.hover",
          padding: 1,
          borderRadius: 1,
          overflow: "auto",
          margin: 0,
          marginBottom: "0.5rem",
          "&:last-child": {
            marginBottom: 0,
          },
        },
        "& code": {
          backgroundColor: "action.hover",
          padding: "2px 4px",
          borderRadius: "4px",
          fontFamily: "monospace",
          fontSize: "0.6875rem",
        },
        "& pre code": {
          backgroundColor: "transparent",
          padding: 0,
        },
        "& table": {
          width: "100%",
          borderCollapse: "collapse",
          marginBottom: "0.5rem",
          fontSize: "0.6875rem",
        },
        "& th, & td": {
          padding: "0.25rem 0.5rem",
          border: "1px solid",
          borderColor: "divider",
        },
        "& th": {
          fontWeight: 500,
          backgroundColor: "action.hover",
        },
      }}
    >
      <Markdown>{content}</Markdown>
    </Box>
  );
}

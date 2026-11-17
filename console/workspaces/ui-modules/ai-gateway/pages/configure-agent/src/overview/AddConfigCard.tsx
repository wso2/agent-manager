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

import { Card, CardContent, Tooltip } from "@wso2/oxygen-ui";
import { Plus } from "@wso2/oxygen-ui-icons-react";
import { Link } from "react-router-dom";

interface AddConfigCardProps {
    /** Shown as the tooltip on hover, e.g. "Configure LLM" / "Configure MCP". */
    label: string;
    href: string;
}

/**
 * The trailing "add" tile in an EnvConfigGroup's card list — a small square,
 * dashed, clickable sibling to ConfigListCard that links to the Configure
 * Agent page to add another LLM provider / MCP server. Just a "+"; the intent
 * is spelled out in the tooltip.
 */
export const AddConfigCard: React.FC<AddConfigCardProps> = ({ label, href }) => (
    <Tooltip title={label}>
        <Card
            variant="outlined"
            component={Link}
            to={href}
            aria-label={label}
            sx={{
                flex: "0 0 auto",
                alignSelf: "stretch",
                width: 68,
                minHeight: 68,
                display: "flex",
                alignItems: "center",
                justifyContent: "center",
                textDecoration: "none",
                color: "text.secondary",
                borderStyle: "dashed",
                transition: "border-color 120ms, background-color 120ms, color 120ms",
                "&:hover": {
                    borderColor: "primary.main",
                    color: "primary.main",
                    backgroundColor: "action.hover",
                },
            }}
        >
            <CardContent
                sx={{
                    display: "flex",
                    alignItems: "center",
                    justifyContent: "center",
                    p: 0,
                    "&:last-child": { pb: 0 },
                }}
            >
                <Plus size={22} />
            </CardContent>
        </Card>
    </Tooltip>
);

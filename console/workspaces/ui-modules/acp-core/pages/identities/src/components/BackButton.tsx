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

import React from "react";
import { Button } from "@wso2/oxygen-ui";
import { ArrowLeft } from "@wso2/oxygen-ui-icons-react";
import { Link } from "react-router-dom";

interface BackButtonProps {
  to: string;
  label: string;
}

export const BackButton: React.FC<BackButtonProps> = ({ to, label }) => (
  <Button
    component={Link}
    to={to}
    variant="text"
    startIcon={<ArrowLeft size={16} />}
    sx={{ mb: 2 }}
  >
    {label}
  </Button>
);

export default BackButton;

/**
 * Copyright (c) 2026, WSO2 LLC. (https://www.wso2.com).
 *
 * WSO2 LLC. licenses this file to you under the Apache License,
 * Version 2.0 (the "License"); you may not use this file except
 * in compliance with the License. You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing,
 * software distributed under the License is distributed on an
 * "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
 * KIND, either express or implied. See the License for the
 * specific language governing permissions and limitations
 * under the License.
 */

import React from "react";
import { Form, Skeleton, Stack } from "@wso2/oxygen-ui";

interface EditFormSkeletonProps {
  /** Number of placeholder tabs to render. Defaults to 2. */
  tabs?: number;
}

/**
 * Placeholder for the identity edit pages (User / Role / Group) while their
 * data loads. Mirrors the real layout — a tabbed form with a
 * header, description, an input, and a couple of list rows — so the page
 * keeps its shape instead of flashing a centered spinner.
 */
export const EditFormSkeleton: React.FC<EditFormSkeletonProps> = ({
  tabs = 2,
}) => (
  <Form.Stack spacing={3}>
    <Stack
      direction="row"
      spacing={3}
      sx={{ borderBottom: 1, borderColor: "divider", pb: 1.5 }}
    >
      {Array.from({ length: tabs }).map((_, index) => (
        <Skeleton key={index} variant="text" width={72} />
      ))}
    </Stack>

    <Skeleton variant="text" width="30%" height={28} />
    <Skeleton variant="text" width="55%" />
    <Skeleton variant="rounded" height={40} />

    <Stack spacing={1}>
      <Skeleton variant="rounded" height={56} />
      <Skeleton variant="rounded" height={56} />
      <Skeleton variant="rounded" height={56} />
    </Stack>
  </Form.Stack>
);

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

import { Span } from "@agent-management-platform/types";
import { InfoField } from "./InfoField";
import { InfoSection } from "./InfoSection";

interface BasicInfoSectionProps {
    span: Span;
}

export function BasicInfoSection({ span }: BasicInfoSectionProps) {
    return (
        <InfoSection title="Basic Information">
            <InfoField label="Span ID" value={span.spanId} isMonospace />
            <InfoField label="Trace ID" value={span.traceId} isMonospace />
            {span.parentSpanId && (
                <InfoField label="Parent Span ID" value={span.parentSpanId} isMonospace />
            )}
            <InfoField label="Name" value={span.name} />
            <InfoField label="Service" value={span.service} />
        </InfoSection>
    );
}


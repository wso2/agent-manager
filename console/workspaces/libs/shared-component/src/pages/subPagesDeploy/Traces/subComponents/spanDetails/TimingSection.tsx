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

interface TimingSectionProps {
    span: Span;
}

export function TimingSection({ span }: TimingSectionProps) {
    return (
        <InfoSection title="Timing">
            <InfoField 
                label="Start Time" 
                value={new Date(span.startTime).toLocaleString()} 
            />
            <InfoField 
                label="End Time" 
                value={new Date(span.endTime).toLocaleString()} 
            />
            <InfoField 
                label="Duration" 
                value={`${span.durationInNanos / 1000} ms`} 
            />
        </InfoSection>
    );
}


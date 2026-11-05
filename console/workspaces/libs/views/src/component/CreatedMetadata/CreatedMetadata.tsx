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

import { formatRelativeTime } from '@wso2/oxygen-ui';
import { Clock, UserPen } from '@wso2/oxygen-ui-icons-react';
import { PageMetaItem } from '../PageMeta';

export interface CreatedMetadataProps {
  createdAt?: string;
}

/** Renders "Created ... ago" from an ISO timestamp; used as a page subheader on overview pages. */
export function CreatedMetadata({ createdAt }: CreatedMetadataProps) {
  if (!createdAt) return null;

  return (
    <PageMetaItem icon={<Clock size={12} />} label="Created">
      {formatRelativeTime(new Date(createdAt))}
    </PageMetaItem>
  );
}

export interface CreatedByMetadataProps {
  /** Display name of the creator; the row is skipped when unknown. */
  createdBy?: string;
  label?: string;
}

/** Renders the creator's name beside the other page metadata. */
export function CreatedByMetadata({ createdBy, label = 'Created by' }: CreatedByMetadataProps) {
  if (!createdBy) return null;

  return (
    <PageMetaItem icon={<UserPen size={12} />} label={label}>
      {createdBy}
    </PageMetaItem>
  );
}

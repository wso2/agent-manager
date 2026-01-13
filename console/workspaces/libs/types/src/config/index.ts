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

import {AuthReactConfig} from '@asgardeo/auth-react'
import { TraceListTimeRange } from '../api/traces';
import dayjs from 'dayjs';
export interface AppConfig {
  authConfig: AuthReactConfig;
  apiBaseUrl: string;
  disableAuth: boolean;
  instrumentationUrl: string;
}

// Extend the Window interface to include our config
declare global {
  interface Window {
    __RUNTIME_CONFIG__: AppConfig;
  }
}

export const globalConfig: AppConfig = window.__RUNTIME_CONFIG__;

export const getTimeRange = (timeRange: TraceListTimeRange) => {
  switch (timeRange) {
    case TraceListTimeRange.TEN_MINUTES:
      return { startTime: dayjs().subtract(10, 'minutes').toISOString(), endTime: dayjs().toISOString() };
    case TraceListTimeRange.THIRTY_MINUTES:
      return { startTime: dayjs().subtract(30, 'minutes').toISOString(), endTime: dayjs().toISOString() };
    case TraceListTimeRange.ONE_HOUR:
      return { startTime: dayjs().subtract(1, 'hour').toISOString(), endTime: dayjs().toISOString() };
    case TraceListTimeRange.THREE_HOURS:
      return { startTime: dayjs().subtract(3, 'hours').toISOString(), endTime: dayjs().toISOString() };
    case TraceListTimeRange.SIX_HOURS:
      return { startTime: dayjs().subtract(6, 'hours').toISOString(), endTime: dayjs().toISOString() };
    case TraceListTimeRange.TWELVE_HOURS:
      return { startTime: dayjs().subtract(12, 'hours').toISOString(), endTime: dayjs().toISOString() };
    case TraceListTimeRange.ONE_DAY:
      return { startTime: dayjs().subtract(1, 'day').toISOString(), endTime: dayjs().toISOString() };
    case TraceListTimeRange.THREE_DAYS:
      return { startTime: dayjs().subtract(3, 'days').toISOString(), endTime: dayjs().toISOString() };
    case TraceListTimeRange.SEVEN_DAYS:
      return { startTime: dayjs().subtract(7, 'days').toISOString(), endTime: dayjs().toISOString() };
    case TraceListTimeRange.THIRTY_DAYS:
      return { startTime: dayjs().subtract(30, 'days').toISOString(), endTime: dayjs().toISOString() };
  }
}

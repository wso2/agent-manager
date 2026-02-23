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
import { sub, Duration } from 'date-fns';
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

const buildRange = (duration: Duration) => {
  const endTime = new Date();

  return {
    startTime: sub(endTime, duration).toISOString(),
    endTime: endTime.toISOString(),
  };
};

export const getTimeRange = (timeRange: TraceListTimeRange) => {
  switch (timeRange) {
    case TraceListTimeRange.TEN_MINUTES:
      return buildRange({ minutes: 10 });
    case TraceListTimeRange.THIRTY_MINUTES:
      return buildRange({ minutes: 30 });
    case TraceListTimeRange.ONE_HOUR:
      return buildRange({ hours: 1 });
    case TraceListTimeRange.THREE_HOURS:
      return buildRange({ hours: 3 });
    case TraceListTimeRange.SIX_HOURS:
      return buildRange({ hours: 6 });
    case TraceListTimeRange.TWELVE_HOURS:
      return buildRange({ hours: 12 });
    case TraceListTimeRange.ONE_DAY:
      return buildRange({ days: 1 });
    case TraceListTimeRange.THREE_DAYS:
      return buildRange({ days: 3 });
    case TraceListTimeRange.SEVEN_DAYS:
      return buildRange({ days: 7 });
    case TraceListTimeRange.THIRTY_DAYS:
      return buildRange({ days: 30 });
  }
}

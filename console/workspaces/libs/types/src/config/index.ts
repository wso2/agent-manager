import {AuthReactConfig} from '@asgardeo/auth-react'
import { TraceListTimeRange } from '../api/traces';
import dayjs from 'dayjs';
export interface AppConfig {
  authConfig: AuthReactConfig;
  apiBaseUrl: string;
  obsApiBaseUrl: string;
  disableAuth: boolean;
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
  }
}

import { AppConfig } from './config';

declare global {
  interface Window {
    __RUNTIME_CONFIG__: AppConfig;
  }
}

export {};


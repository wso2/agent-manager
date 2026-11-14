/**
 * Copyright (c) 2026, WSO2 LLC. (https://www.wso2.com).
 *
 * WSO2 LLC. licenses this file to you under the Apache License,
 * Version 2.0 (the "License"); you may not use this file except
 * in compliance with the License.
 */

import eslintConfig from "@agent-management-platform/eslint-config";
import { FlatCompat } from "@eslint/eslintrc";

const compat = new FlatCompat();

const sourceFiles = [
  "**/*.ts",
  "**/*.tsx",
  "**/*.js",
  "**/*.jsx",
  "**/*.mjs",
  "**/*.cjs",
];

const scopeToSourceFiles = (config) =>
  config.files ? config : { ...config, files: sourceFiles };

export default [
  ...eslintConfig.map(scopeToSourceFiles),
  ...compat.extends("plugin:storybook/recommended").map(scopeToSourceFiles),
  {
    ignores: [
      "**/dist/**",
      "**/node_modules/**",
      "**/coverage/**",
      "**/.storybook/**",
      "**/storybook-static/**",
      "**/*.config.js",
      "**/*.config.cjs",
    ],
  },
];

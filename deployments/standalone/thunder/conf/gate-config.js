// Copyright 2025 The ThunderID Authors
// SPDX-License-Identifier: Apache-2.0

/* eslint-disable no-underscore-dangle */

window.__THUNDERID_RUNTIME_CONFIG__ = {
  brand: {
    product_name: "ThunderID",
    favicon: {
      light: "assets/images/favicon.ico",
      dark: "assets/images/favicon-inverted.ico",
    },
  },
  client: {
    base: "/gate",
  },
  // Defaults to the origin this app is served from. Required only when the server's
  // external URL differs.
  server: {
    public_url: "@@THUNDER_PUBLIC_URL@@",
  },
};

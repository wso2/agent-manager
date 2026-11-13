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
    base: "/console",
    client_id: "CONSOLE",
    scopes: ['openid', 'profile', 'email', 'group', 'ou', 'system'],
    resource_identifier: {{ q (print .ThunderPublicURL "/mcp") }},
  },
  // Defaults to the origin this app is served from. Required only when the server's
  // external URL differs.
  server: {
    public_url: {{ q .ThunderPublicURL }},
  },
  // Location of the login gate, used to build the OAuth redirect URI shown when configuring
  // social/OIDC connections. Omit to default to `${server public_url}/gate/callback`.
  gate_client: {
    hostname: {{ q .ThunderHost }},
    port: {{ .ThunderPort }},
    scheme: {{ q .ThunderScheme }},
  },
};

"""Exercise the supplied MCP server and client without AWS or a model provider."""

from dataclasses import replace

import asyncio
import base64
import importlib.util
import json
import os
from pathlib import Path
import socket
import threading
import time
import unittest
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from urllib.parse import parse_qs
from unittest.mock import patch

import httpx
import uvicorn
from agent import build_agent
from config import Config
from mcp_client import AgentIDAuth, create_mcp_client

ROOT = Path(__file__).resolve().parents[1]
spec = importlib.util.spec_from_file_location(
    "insurance_mcp_server", ROOT / "mcp-server/server.py"
)
server_module = importlib.util.module_from_spec(spec)
spec.loader.exec_module(server_module)


class MCPIntegrationTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        with patch.dict(os.environ, {"INSURANCE_MCP_UPSTREAM_KEY": "test-upstream"}):
            app = server_module.create_app()
        sock = socket.socket()
        sock.bind(("127.0.0.1", 0))
        cls.upstream_url = f"http://127.0.0.1:{sock.getsockname()[1]}/mcp"
        cls.uvicorn = uvicorn.Server(
            uvicorn.Config(app, log_level="critical", lifespan="on")
        )
        cls.server_thread = threading.Thread(
            target=cls.uvicorn.run, kwargs={"sockets": [sock]}, daemon=True
        )
        cls.server_thread.start()
        deadline = time.monotonic() + 10
        while not cls.uvicorn.started and time.monotonic() < deadline:
            time.sleep(0.01)
        if not cls.uvicorn.started:
            raise RuntimeError("MCP test server failed to start")

        class Proxy(BaseHTTPRequestHandler):
            def log_message(self, *args):
                pass

            def do_POST(self):
                body = self.rfile.read(int(self.headers.get("Content-Length", "0")))
                if self.path == "/token":
                    cls.tokens.append(
                        (self.headers.get("Authorization"), parse_qs(body.decode()))
                    )
                    result = json.dumps(
                        {
                            "access_token": "test-access",
                            "token_type": "Bearer",
                            "expires_in": 300,
                        }
                    ).encode()
                    self.send_response(200)
                    self.send_header("Content-Type", "application/json")
                    self.end_headers()
                    self.wfile.write(result)
                    return
                if (
                    cls.denied
                    or self.headers.get("Authorization") != "Bearer test-access"
                ):
                    self.send_response(403)
                    self.end_headers()
                    return
                cls.forwarded += 1
                headers = {
                    k: v
                    for k, v in self.headers.items()
                    if k.lower()
                    in {
                        "content-type",
                        "accept",
                        "mcp-protocol-version",
                        "mcp-session-id",
                    }
                }
                headers["X-Insurance-Upstream-Key"] = "test-upstream"
                response = httpx.post(cls.upstream_url, headers=headers, content=body)
                self.send_response(response.status_code)
                self.send_header(
                    "Content-Type",
                    response.headers.get("content-type", "application/json"),
                )
                self.end_headers()
                self.wfile.write(response.content)

        cls.proxy = ThreadingHTTPServer(("127.0.0.1", 0), Proxy)
        cls.proxy_thread = threading.Thread(target=cls.proxy.serve_forever, daemon=True)
        cls.proxy_thread.start()
        cls.proxy_url = f"http://127.0.0.1:{cls.proxy.server_port}/proxy-mcp"

    @classmethod
    def tearDownClass(cls):
        cls.proxy.shutdown()
        cls.proxy.server_close()
        cls.proxy_thread.join(timeout=5)
        cls.uvicorn.should_exit = True
        cls.server_thread.join(timeout=5)

    def setUp(self):
        type(self).tokens = []
        type(self).forwarded = 0
        type(self).denied = False
        self.env = {
            "OPENAI_API_KEY": "test-openai",
            "USE_MCP": "true",
            "INSURANCE_MCP_URL": self.proxy_url,
            "AMP_AGENTID_CLIENT_ID": "test-client",
            "AMP_AGENTID_CLIENT_SECRET": "test-secret",
            "AMP_AGENTID_TOKEN_ENDPOINT": self.proxy_url.replace(
                "/proxy-mcp", "/token"
            ),
            "AMP_AGENTID_SCOPES": "read-policies",
        }
        with patch.dict(os.environ, self.env, clear=True):
            self.cfg = Config.from_env()

    def test_real_mcp_discovery_lookup_and_proxy_denial(self):
        with create_mcp_client(self.cfg) as client:
            tools = client.list_tools_sync()
            self.assertEqual(
                {t.tool_name for t in tools}, {"list_policies", "lookup_policy"}
            )
            result = client.call_tool_sync("call-1", "list_policies", {})
            self.assertIn("OZ-AUTO-4417", json.dumps(result))
            result = client.call_tool_sync(
                "call-2", "lookup_policy", {"policy_number": "oz-home-2280"}
            )
            self.assertIn("OZ-HOME-2280", json.dumps(result))
            agent = build_agent(self.cfg, tools)
            self.assertEqual(
                set(agent.tool_registry.registry), {"list_policies", "lookup_policy"}
            )
            self.assertEqual(
                len(self.tokens), 1, "token should be reused before expiry"
            )
            auth, form = self.tokens[0]
            self.assertEqual(
                auth, "Basic " + base64.b64encode(b"test-client:test-secret").decode()
            )
            self.assertEqual(
                form,
                {
                    "grant_type": ["client_credentials"],
                    "scope": ["read-policies"],
                    "resource": [self.proxy_url],
                },
            )

    def test_agent_uses_remote_policy_tool_in_converse_round_trip(self):
        model_calls = []

        class ModelHandler(BaseHTTPRequestHandler):
            def log_message(self, *args):
                pass

            def do_POST(self):
                body = json.loads(self.rfile.read(int(self.headers["Content-Length"])))
                model_calls.append(body)
                if len(model_calls) == 1:
                    content = [
                        {
                            "toolUse": {
                                "toolUseId": "lookup",
                                "name": "list_policies",
                                "input": {},
                            }
                        }
                    ]
                    reason = "tool_use"
                else:
                    content = [{"text": "The returned policy is OZ-AUTO-4417."}]
                    reason = "end_turn"
                self.send_response(200)
                self.send_header("Content-Type", "application/json")
                self.end_headers()
                self.wfile.write(
                    json.dumps(
                        {
                            "output": {
                                "message": {"role": "assistant", "content": content}
                            },
                            "stopReason": reason,
                            "usage": {
                                "inputTokens": 10,
                                "outputTokens": 10,
                                "totalTokens": 20,
                            },
                            "metrics": {"latencyMs": 1},
                        }
                    ).encode()
                )

        model = ThreadingHTTPServer(("127.0.0.1", 0), ModelHandler)
        thread = threading.Thread(target=model.serve_forever, daemon=True)
        thread.start()
        try:
            cfg = replace(
                self.cfg,
                model_provider="bedrock-gateway",
                bedrock_model_id="amazon.nova-micro-v1:0",
                llm_gateway_url=f"http://127.0.0.1:{model.server_port}",
                llm_gateway_api_key="test-gateway",
            )
            with create_mcp_client(cfg) as client:
                agent = build_agent(cfg, client.list_tools_sync())
                self.assertIn("OZ-AUTO-4417", str(agent("List my policies.")))
            self.assertEqual(len(model_calls), 2)
            result_messages = [
                m
                for m in model_calls[1]["messages"]
                if any("toolResult" in c for c in m["content"])
            ]
            self.assertEqual(len(result_messages), 1)
            self.assertIn("OZ-AUTO-4417", json.dumps(result_messages))
            tool_names = {
                t["toolSpec"]["name"] for t in model_calls[0]["toolConfig"]["tools"]
            }
            self.assertEqual(tool_names, {"list_policies", "lookup_policy"})
        finally:
            model.shutdown()
            model.server_close()
            thread.join(timeout=5)

    def test_application_lifespan_connects_and_closes_mcp(self):
        from fastapi.testclient import TestClient

        with patch.dict(os.environ, self.env, clear=True):
            import app as application
        with patch.object(application, "CONFIG", self.cfg):
            with TestClient(application.app) as client:
                self.assertEqual(client.get("/health").status_code, 200)
                self.assertEqual(
                    {t.tool_name for t in application.MCP_TOOLS},
                    {"list_policies", "lookup_policy"},
                )
            self.assertIsNone(application.MCP_TOOLS)

    def test_proxy_denial_never_reaches_upstream(self):
        before = 0
        # HTTP denial closes the SDK transport; shutdown propagates the failure.
        with self.assertRaisesRegex(
            RuntimeError, "Connection to the MCP server was closed"
        ):
            with create_mcp_client(self.cfg) as client:
                client.list_tools_sync()
                before = self.forwarded
                type(self).denied = True
                result = client.call_tool_sync("denied", "list_policies", {})
                self.assertEqual(result["status"], "error")
        self.assertEqual(self.forwarded, before)

    def test_upstream_requires_separate_key(self):
        response = httpx.post(
            self.upstream_url, headers={"Authorization": "Bearer test-access"}
        )
        self.assertEqual(response.status_code, 401)

    def test_mcp_mode_has_no_local_fallback(self):
        with self.assertRaisesRegex(RuntimeError, "local fallback is disabled"):
            build_agent(self.cfg)
        with patch.dict(os.environ, {"OPENAI_API_KEY": "test"}, clear=True):
            agent = build_agent(Config.from_env())
        self.assertIn("file_claim", agent.tool_registry.registry)

    def test_config_rejects_missing_credentials_and_insecure_endpoints(self):
        for variable in [
            "INSURANCE_MCP_URL",
            "AMP_AGENTID_CLIENT_ID",
            "AMP_AGENTID_CLIENT_SECRET",
            "AMP_AGENTID_TOKEN_ENDPOINT",
            "AMP_AGENTID_SCOPES",
        ]:
            env = dict(self.env)
            del env[variable]
            with (
                self.subTest(variable=variable),
                patch.dict(os.environ, env, clear=True),
            ):
                with self.assertRaises(RuntimeError):
                    Config.from_env()
        for url in [
            "http://example.com/mcp",
            "https://user:secret@example.com/mcp",
            "https://example.com/mcp?key=secret",
        ]:
            with patch.dict(
                os.environ, self.env | {"INSURANCE_MCP_URL": url}, clear=True
            ):
                with self.assertRaises(RuntimeError):
                    Config.from_env()

    def test_token_refresh_and_failure(self):
        async def run():
            count = 0
            fail = False

            def handle(request):
                nonlocal count
                if request.url.path == "/token":
                    count += 1
                    if fail:
                        return httpx.Response(401)
                    return httpx.Response(
                        200,
                        json={
                            "access_token": f"token-{count}",
                            "token_type": "Bearer",
                            "expires_in": 60,
                        },
                    )
                self.assertEqual(
                    request.headers["Authorization"], f"Bearer token-{count}"
                )
                return httpx.Response(200)

            auth = AgentIDAuth(self.cfg)
            async with httpx.AsyncClient(
                transport=httpx.MockTransport(handle), auth=auth
            ) as client:
                await client.post(self.proxy_url)
                await client.post(self.proxy_url)
                self.assertEqual(count, 1)
                auth._expires_at = 0
                await client.post(self.proxy_url)
                self.assertEqual(count, 2)
                auth._expires_at = 0
                fail = True
                with self.assertRaisesRegex(RuntimeError, "token request failed"):
                    await client.post(self.proxy_url)
                with self.assertRaisesRegex(RuntimeError, "Unexpected MCP request URL"):
                    await client.post("https://other.example/mcp")

        asyncio.run(run())

    def test_server_requires_key_at_startup(self):
        with patch.dict(os.environ, {}, clear=True):
            with self.assertRaises(RuntimeError):
                server_module.create_app()


if __name__ == "__main__":
    unittest.main()

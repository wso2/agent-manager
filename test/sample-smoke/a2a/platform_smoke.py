"""Build, deploy and verify the A2A sample through Agent Manager."""
from __future__ import annotations

from datetime import datetime, timedelta, timezone
import json
from pathlib import Path
import re
import subprocess
import sys
import time
import uuid

from a2a_smoke import ROOT, check_evidence, invoke
from smoke import request, stop_process, wait_ready

# Reuse the existing platform authentication/request client, not a second OAuth implementation.
sys.path.insert(0, str(ROOT / "test/instrumentation-matrix"))
from heavy.amp_client import AmpClient, DeployedAgent, IdpCredentials
from harness.classify import classify_span


def kubectl(*args, data=None):
    return subprocess.run(["kubectl", *args], input=data, text=True, capture_output=True, check=True, timeout=60).stdout


def validate_source(build, workflow, sha, repo, branch, app_path="/samples/a2a-agent"):
    params = build["buildParameters"]
    # The REST API deliberately abbreviates commitId; WorkflowRun retains all 40 digits.
    revision = workflow["spec"]["workflow"]["parameters"]["repository"]
    if (params.get("commitId") != sha[:8] or params.get("repoUrl") != repo
            or params.get("branch") != branch or params.get("appPath") != app_path
            or revision.get("url") != repo or revision.get("revision", {}).get("commit") != sha):
        raise AssertionError("Build source does not match the requested repository and exact commit")


def trace_matches(spans, marker):
    # A populated trace list is insufficient: require this invocation's model span.
    return any(classify_span(span) == "llm" and marker in json.dumps(span) for span in spans)


def wait_until(description, poll, timeout=600):
    deadline = time.monotonic() + timeout
    while time.monotonic() < deadline:
        result = poll()
        if result:
            return result
        time.sleep(5)
    raise TimeoutError(f"Timed out waiting for {description}")


def build_revision(client, builds_path, sha):
    # Creation starts a branch build. Drain it before requesting the immutable
    # revision so an older build cannot deploy over the build under test.
    def initial_finished():
        builds = client._request("GET", builds_path, expect=(200,)).json()["builds"]
        return builds and all(b.get("status") in ("Completed", "Failed") for b in builds)
    wait_until("initial branch build to finish", initial_finished, timeout=1200)
    build = client._request("POST", builds_path + "?commitId=" + sha, expect=(202,)).json()
    build_name = build["buildName"]
    def finished():
        detail = client._request("GET", builds_path + "/" + build_name, expect=(200,)).json()
        if detail.get("status") == "Failed":
            raise RuntimeError(f"Pinned build {build_name} failed for commit {sha}")
        if detail.get("status") == "Completed":
            if not detail.get("imageId"):
                raise AssertionError(f"Pinned build {build_name} completed without an imageId")
            return detail
        return None
    return wait_until(f"pinned build {build_name}", finished, timeout=1200)


def start_forward(namespace, target, port, log_path):
    log = log_path.open("w")
    process = subprocess.Popen(["kubectl", "-n", namespace, "port-forward", target, f"0:{port}"], stdout=log, stderr=subprocess.STDOUT, start_new_session=True)
    def url():
        match = re.search(r"127.0.0.1:(\d+)", log_path.read_text())
        if process.poll() is not None:
            raise RuntimeError(f"Port-forward exited; see {log_path}")
        return f"http://127.0.0.1:{match[1]}" if match else None
    try:
        return process, log, wait_until(f"port-forward to {target}", url, timeout=30)
    except BaseException:
        stop_process(process)
        log.close()
        raise


def install_stub(namespace, script_path=None):
    kubectl("create", "namespace", namespace)
    script = (script_path or Path(__file__).parent / "a2a_model_stub.py").read_text()
    labels = {"app": "a2a-model"}
    resources = [
        {"apiVersion": "v1", "kind": "ConfigMap", "metadata": {"name": "model-script", "namespace": namespace}, "data": {"model_stub.py": script}},
        {"apiVersion": "apps/v1", "kind": "Deployment", "metadata": {"name": "model", "namespace": namespace}, "spec": {
            "replicas": 1, "selector": {"matchLabels": labels}, "template": {"metadata": {"labels": labels}, "spec": {
                "automountServiceAccountToken": False,
                "containers": [{"name": "model", "image": "python:3.11-slim", "command": ["python", "/stub/model_stub.py"],
                    "ports": [{"containerPort": 8099}], "volumeMounts": [{"name": "script", "mountPath": "/stub", "readOnly": True}],
                    "readinessProbe": {"httpGet": {"path": "/health", "port": 8099}},
                    "resources": {"requests": {"cpu": "25m", "memory": "32Mi"}, "limits": {"cpu": "250m", "memory": "128Mi"}},
                    "securityContext": {"runAsNonRoot": True, "runAsUser": 10001, "allowPrivilegeEscalation": False, "capabilities": {"drop": ["ALL"]}}}],
                "volumes": [{"name": "script", "configMap": {"name": "model-script"}}],
            }}}},
        {"apiVersion": "v1", "kind": "Service", "metadata": {"name": "model", "namespace": namespace}, "spec": {
            "selector": labels, "ports": [{"port": 8099, "targetPort": 8099}]}},
    ]
    kubectl("apply", "-f", "-", data=json.dumps({"apiVersion": "v1", "kind": "List", "items": resources}))
    kubectl("-n", namespace, "rollout", "status", "deployment/model", "--timeout=120s")
    return f"http://model.{namespace}.svc.cluster.local:8099/v1"


def allow_model_egress(namespace, name, environment, stub_namespace):
    # Sandboxed agents deny arbitrary private-network egress by default. Permit
    # this test agent to reach only this run's model pod on its one HTTP port.
    policy = {
        "apiVersion": "networking.k8s.io/v1", "kind": "NetworkPolicy",
        "metadata": {"name": "smoke-model-egress", "namespace": namespace},
        "spec": {
            "podSelector": {"matchLabels": {"openchoreo.dev/component": name,
                "openchoreo.dev/project": name, "openchoreo.dev/environment": environment}},
            "policyTypes": ["Egress"],
            "egress": [{"to": [{"namespaceSelector": {"matchLabels": {
                "kubernetes.io/metadata.name": stub_namespace}},
                "podSelector": {"matchLabels": {"app": "a2a-model"}}}],
                "ports": [{"protocol": "TCP", "port": 8099}]}],
        },
    }
    kubectl("apply", "-f", "-", data=json.dumps(policy))


def poll_trace(client, deployed, observer, marker, started, reports):
    session = client._session
    params = {"organization": deployed.org, "project": deployed.project_name,
              "agent": deployed.agent_name, "environment": deployed.environment,
              "startTime": (started - timedelta(seconds=5)).isoformat(),
              "endTime": (started + timedelta(minutes=10)).isoformat(), "limit": 100}
    def get(path, query=None):
        response = session.get(observer.rstrip("/") + path, params=query,
                               headers={"Authorization": f"Bearer {client.access_token()}"}, timeout=30)
        response.raise_for_status()
        return response.json()
    def poll():
        listing = get("/api/v1/traces", params)
        (reports / "trace-list.json").write_text(json.dumps(listing, indent=2) + "\n")
        for trace in listing.get("traces", []):
            tid = trace["traceId"]
            summaries = get(f"/api/v1/traces/{tid}/spans", params)
            spans = [get(f"/api/v1/traces/{tid}/spans/{span['spanId']}") for span in summaries.get("spans", [])]
            if trace_matches(spans, marker):
                (reports / "trace-spans.json").write_text(json.dumps(spans, indent=2) + "\n")
                return tid
        return None
    return wait_until("this invocation's model span in Observer", poll, timeout=180)


def project_deleted(name):
    # Project GET currently maps OpenChoreo's missing-project response to 500.
    # After DELETE is acknowledged, require actual CR removal instead of
    # treating that 500 (or a deletionTimestamp) as successful cleanup.
    result = json.loads(kubectl("get", "projects.openchoreo.dev", "-A",
                                "--field-selector", f"metadata.name={name}", "-o", "json"))
    return not result["items"]


def diagnostics(namespace, reports):
    commands = {
        "pods.txt": ("get", "pods", "-A", "-o", "wide"),
        "events.txt": ("get", "events", "-A", "--field-selector", "type=Warning"),
        "model.log": ("-n", namespace, "logs", "deployment/model", "--tail=100"),
    }
    # Only logs from resources named for this test, before deletion. Avoid dumping
    # WorkflowRun specs, which may contain injected identity credentials.
    try:
        pods = json.loads(kubectl("get", "pods", "-A", "-o", "json"))["items"]
        for pod in pods:
            meta = pod["metadata"]
            if namespace in meta["name"] or meta["namespace"] == namespace:
                commands[f"{meta['namespace']}-{meta['name']}.log"] = (
                    "-n", meta["namespace"], "logs", meta["name"], "--all-containers=true", "--tail=100")
    except Exception as exc:
        print(f"Pod log discovery failed: {exc}", file=sys.stderr)
    for name, args in commands.items():
        try:
            (reports / name).write_text(kubectl(*args))
        except Exception as exc:
            print(f"Diagnostic {name} unavailable: {exc}", file=sys.stderr)


def run(args, sample):
    sample_path = sample.get("path", "samples/a2a-agent")
    port = sample.get("port", 9099)
    invoke_sample = sample.get("invoke", invoke)
    verify_evidence = sample.get("check_evidence", check_evidence)
    if not re.fullmatch(r"[0-9a-f]{40}", args.sha):
        raise ValueError("--sha must be the full 40-character PR head commit SHA")
    if not re.fullmatch(r"https://github.com/[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+", args.repo):
        raise ValueError("--repo must be the HTTPS GitHub repository containing the PR commit")
    # Reject accidentally claiming that a remote build covers uncommitted sample edits.
    dirty = subprocess.check_output(["git", "status", "--porcelain", "--untracked-files=all", "--", sample_path], cwd=ROOT, text=True)
    # New test wrapper does not affect the deployed application; all other sample edits matter.
    dirty = [line for line in dirty.splitlines() if line[3:] not in {sample_path + "/README.md", sample_path + "/scripts/smoke_test.sh"}]
    if dirty:
        raise RuntimeError("Uncommitted sample source changes cannot be tested by a remote Git build; run local smoke first and use a published PR SHA for platform smoke")
    reports = args.reports.resolve()
    reports.mkdir(parents=True, exist_ok=True)
    name = sample.get("prefix", "a2a-smoke-") + uuid.uuid4().hex[:12]
    marker = "smoke-" + uuid.uuid4().hex
    client = AmpClient(args.api, IdpCredentials(args.token_url, args.client_id, args.client_secret), org=args.org, environment=args.environment)
    deployed = DeployedAgent(args.org, name, name, args.environment, "", "")
    base = f"/api/v1/orgs/{args.org}/projects/{name}"
    phase = "model stub startup"
    print(f"==> {phase}", flush=True)
    namespace_created = project_attempted = False
    forwards = []
    stub_local = None
    egress_namespace = None
    cleanup_errors = []
    try:
        # Namespace is unique to this run; cleanup also runs after partial creation.
        namespace_created = True
        model_url = install_stub(name, sample.get("stub"))
        forward, forward_log, stub_local = start_forward(name, "service/model", 8099, reports / "model-forward.log")
        forwards.append((forward, forward_log))
        wait_ready(stub_local + "/health", forward)
        phase = "project and agent creation"
        print(f"==> {phase}", flush=True)
        project_attempted = True
        client._request("POST", f"/api/v1/orgs/{args.org}/projects", json={"name": name, "displayName": name, "deploymentPipeline": args.pipeline}, expect=(202,))
        env_values = sample["env"](model_url, args.sha)
        env = [{"key": key, "value": value, "isSensitive": False} for key, value in env_values.items()]
        client._request("POST", base + "/agents", json={
            "name": name, "displayName": name,
            "provisioning": {"type": "internal", "repository": {"url": args.repo, "branch": args.branch, "appPath": "/" + sample_path}},
            "agentType": sample.get("agent_type", {"type": "agent-api", "subType": "a2a-agent"}),
            "build": {"type": "buildpack", "buildpack": {"language": "python", "languageVersion": "3.11", "runCommand": "python main.py"}},
            "configurations": {"enableAutoInstrumentation": True, "env": env},
            "inputInterface": {"type": "HTTP", "port": port},
        }, expect=(202,))
        phase = "source build"
        print(f"==> {phase}", flush=True)
        detail = build_revision(client, base + f"/agents/{name}/builds", args.sha)
        deployed.image_id = detail["imageId"]
        build = detail
        runs = json.loads(kubectl("get", "workflowruns.openchoreo.dev", "-A", "--field-selector", f"metadata.name={build['buildName']}", "-o", "json"))["items"]
        workflow = next(w for w in runs if w["metadata"]["name"] == build["buildName"])
        (reports / "build.json").write_text(json.dumps(detail, indent=2) + "\n")
        # Save just the source provenance, never identity env injected into the workflow.
        (reports / "source.json").write_text(json.dumps(workflow["spec"]["workflow"]["parameters"]["repository"], indent=2) + "\n")
        validate_source(detail, workflow, args.sha, args.repo, args.branch, "/" + sample_path)
        phase = "deployment readiness"
        print(f"==> {phase}", flush=True)
        def pinned_deployment():
            deployments = client._request("GET", base + f"/agents/{name}/deployments", expect=(200,)).json()
            deployment = deployments.get(args.environment, {})
            if deployment.get("imageId") == deployed.image_id and deployment.get("status") == "active":
                endpoints = deployment.get("endpoints") or []
                if not endpoints:
                    raise AssertionError("Pinned deployment is active but exposes no endpoint")
                return endpoints[0]["url"]
            return None
        deployed.endpoint_url = wait_until("active deployment of the pinned image", pinned_deployment)
        deployed.api_key = client._mint_api_key(args.org, name)
        headers = {"X-API-Key": deployed.api_key}
        # Probe health on the actual workload, then exercise the published gateway.
        selector = f"openchoreo.dev/component={name},openchoreo.dev/project={name},openchoreo.dev/environment={args.environment}"
        pods = json.loads(kubectl("get", "pods", "-A", "-l", selector, "-o", "json"))["items"]
        pod = next(p for p in pods if p.get("status", {}).get("phase") == "Running")
        egress_namespace = pod["metadata"]["namespace"]
        allow_model_egress(egress_namespace, name, args.environment, name)
        proc, log, health_url = start_forward(pod["metadata"]["namespace"], "pod/" + pod["metadata"]["name"], port, reports / "health-forward.log")
        forwards.append((proc, log))
        health = wait_ready(health_url + "/health", proc)
        if health.get("status") != "ok":
            raise AssertionError("Deployed A2A health check failed")
        phase = "published endpoint model round trip"
        print(f"==> {phase}", flush=True)
        started = datetime.now(timezone.utc)
        result = invoke_sample(deployed.endpoint_url, marker, headers, version=args.sha)
        evidence = request(stub_local + "/evidence")
        verify_evidence(evidence, marker)
        phase = "trace ingestion"
        print(f"==> {phase}", flush=True)
        trace_id = poll_trace(client, deployed, args.observer, marker, started, reports)
        (reports / "result.json").write_text(json.dumps({"sha": args.sha, "image": deployed.image_id, "traceId": trace_id, "response": result, "model": evidence}, indent=2) + "\n")
        print(f"PASS: PR {args.sha} → build → deploy → endpoint → A2A round trips → trace {trace_id}")
    except BaseException:
        print(f"FAIL: {phase}; diagnostics: {reports}", file=sys.stderr)
        diagnostics(name, reports)
        raise
    finally:
        if stub_local is not None:
            try:
                (reports / "model-evidence.json").write_text(json.dumps(request(stub_local + "/evidence"), indent=2) + "\n")
            except Exception as exc:
                print(f"Could not collect model evidence: {exc}", file=sys.stderr)
        if egress_namespace is not None:
            try:
                kubectl("-n", egress_namespace, "delete", "networkpolicy", "smoke-model-egress", "--ignore-not-found")
            except Exception as exc:
                cleanup_errors.append(f"model egress policy: {exc}")
        # Explicit deletes (also on build failures); do not silently swallow leaked resources.
        if project_attempted:
            for path in (base + f"/agents/{name}", base):
                try:
                    def delete():
                        response = client._request("DELETE", path, expect=(200, 202, 204, 404, 409))
                        return response.status_code != 409
                    wait_until(f"cleanup of {path}", delete, timeout=120)
                    if path == base:
                        wait_until(f"project CR deletion of {name}", lambda: project_deleted(name), timeout=120)
                    else:
                        wait_until(f"deletion of {path}", lambda: client._request("GET", path, expect=(200, 404)).status_code == 404, timeout=120)
                except Exception as exc:
                    cleanup_errors.append(f"{path}: {exc}")
        for process, log in reversed(forwards):
            try:
                stop_process(process)
            except Exception as exc:
                cleanup_errors.append(f"port-forward: {exc}")
            finally:
                log.close()
        if namespace_created:
            try:
                kubectl("delete", "namespace", name, "--ignore-not-found", "--wait=false")
            except Exception as exc:
                cleanup_errors.append(f"namespace {name}: {exc}")
        if cleanup_errors:
            (reports / "cleanup-errors.txt").write_text("\n".join(cleanup_errors) + "\n")
            if sys.exc_info()[0] is None:
                raise RuntimeError(f"Smoke passed but cleanup failed: {cleanup_errors}")
            print(f"Cleanup failures: {cleanup_errors}", file=sys.stderr)

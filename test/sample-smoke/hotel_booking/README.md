# Hotel Booking deployment and trace smoke

The PR workflow builds Agent Manager and its instrumentation/observer from the
checkout using `amp-dev-stack`. It also builds the Hotel API from this checkout,
loads the image into k3d, and starts it alongside a local OpenAI-compatible model
fixture. Both fixture containers run without credentials or service-account tokens.

The agent is created against the PR head branch. Once its automatic branch build
finishes, the harness explicitly requests `POST .../builds?commitId=<full PR SHA>`.
Only the returned build is tested. The build API, WorkflowRun commit, and active
image must match before calling the public gateway with a test agent API key.
The model fixture calls the real hotel search tool and requires the Tokyo hotel
result before returning the invocation marker. Observer must contain that marker
on a model span scoped to this agent and invocation time window.

## Local run

Requires Docker, k3d, kubectl, Python 3.11, and a running dev stack. Use an isolated
dev stack, never a production cluster. The checkout HEAD must match the published
SHA; application source must be clean. The model fixture and harness may be
edited locally. Build the stack with the same `make setup-*` targets used by
`.github/actions/amp-dev-stack/action.yaml` if it is not already available.

```bash
python3.11 -m venv /tmp/hotel-platform-env
/tmp/hotel-platform-env/bin/pip install requests
export IDP_CLIENT_SECRET='<local-dev-bootstrap-credential>'
/tmp/hotel-platform-env/bin/python test/sample-smoke/hotel_booking/platform_smoke.py \
  --repo https://github.com/<your-fork>/agent-manager \
  --branch <branch> --sha "$(git rev-parse HEAD)" \
  --reports /tmp/hotel-platform-reports
```

Defaults: k3d cluster `openchoreo-local-setup`, API `localhost:9000`, Observer
`localhost:9098`, IDP token endpoint `localhost:8090/oauth2/token`. Override with
`--cluster`, `--api`, `--observer`, or `--token-url` for another local dev stack.
No paid service or repository secret is required. CI uses the dev stack's public
bootstrap test credential. Policy vector search remains disabled.

Resources use unique names. A temporary egress policy allows only this agent to
reach this run's fixture pod on TCP 8099/9091. Cleanup removes the agent, project,
fixture namespace, policy, port-forwards, and host fixture image. The project is
deleted through Agent Manager, then its absence is confirmed in OpenChoreo to
avoid the known project detail endpoint's 500-after-deletion behavior. Real
cleanup errors still fail the job. Failure artifacts contain logs and restricted
cluster state, without dumping workflow specs or credential environment values.
Existing nightly tests remain unchanged.

Run the build-selection and trace-correlation guards independently:

```bash
/tmp/hotel-platform-env/bin/python -m unittest discover \
  -s test/sample-smoke/hotel_booking -p 'test_*.py' -v
```

A build that remains `Succeeded` for five minutes fails with a reconciliation
error: the image finished building, but the workload record was not updated.
`Completed` is still required before proceeding. The platform job is intentionally
heavy and retains its 100-minute overall limit. Both jobs passed on the standard
Ubuntu runner in [run 41060857774](https://github.com/wso2/agent-manager/actions/runs/41060857774);
this confirms the deployments API shape used by the test and capacity for that run.

# Connect the insurance agent on Amazon ECS to Agent Manager

Use an existing ECS cluster and VPC to deploy a Linux/x86-64 Fargate agent task
with the Bedrock gateway adapter. The optional MCP service adds tool authorization.
Registration does not deploy the application or secure its inbound endpoint.

## Prerequisites

- AWS CLI v2, Docker with `linux/amd64` build support, and permissions to push
  ECR images, register/run/stop ECS tasks, manage services, and pass the execution role.
  Commands below assume the standard `aws` partition.
- Existing ECS cluster and private subnets with outbound HTTPS through NAT.
  AWS VPC endpoints do not replace egress to Agent Manager SaaS, gateways, and identity endpoints.
- A test client with private connectivity (VPN or an existing instance in the VPC).
  Permit inbound TCP 8000 to the agent task only from that client/security group.
- A task execution role trusted by `ecs-tasks.amazonaws.com`, with
  `AmazonECSTaskExecutionRolePolicy`, `secretsmanager:GetSecretValue` on the exact
  exercise secret ARNs, and `kms:Decrypt` on a customer-managed key if required.
  Create this in IAM using the **Elastic Container Service Task** use case if needed.
  The deployer needs `iam:PassRole` for this role. No application `taskRoleArn`
  is needed: the agent uses a gateway key, not AWS inference credentials.
- Agent Manager SaaS organization/project/environment permissions and Bedrock
  model access. Follow [BEDROCK-GATEWAY.md](BEDROCK-GATEWAY.md) for provider setup.

Both images use Python 3.11. The agent pins Strands Agents 1.58.0,
MCP 1.29.0, and amp-instrumentation 0.4.1. Use Fargate Linux 1.4.0 or later for
individual Secrets Manager JSON fields. This exercise incurs usage charges.

## 1. Get the sample

```bash
git clone https://github.com/wso2/agent-manager.git
cd agent-manager/samples/insurance-support-agent
```

## 2. Register and store credentials

1. In Agent Manager, open your project, choose **Add Agent**, select
   **Externally-Hosted Agent**, and register **Insurance Support Agent**.
2. In **Setup Agent**, select a token duration, choose **Generate**, and copy
   the telemetry token and `AMP_OTEL_ENDPOINT`.
3. Register and attach the Bedrock provider using the linked guide. Keep its
   upstream Bedrock credential in the provider; copy the agent's gateway URL/key.
4. In Secrets Manager, create an **Other type of secret** named
   `insurance-demo/agent` with JSON keys `AMP_AGENT_API_KEY` and
   `LLM_GATEWAY_API_KEY`. Copy the full secret ARN including its generated suffix.

Keep secrets out of images, Git, and task-definition `environment` fields.
ECS reads secrets at startup, so replace the task after credential rotation.

## 3. Build and push

Run from `samples/insurance-support-agent`. Substitute your existing cluster.
Skip repository/log-group creation if reusing resources from an earlier run.

```bash
export AWS_REGION=us-east-1
export ECS_CLUSTER="<your-existing-cluster>"
AWS_ACCOUNT_ID=$(aws sts get-caller-identity --query Account --output text)
ECR_REGISTRY="$AWS_ACCOUNT_ID.dkr.ecr.$AWS_REGION.amazonaws.com"
export IMAGE_TAG=$(git rev-parse --short=12 HEAD)
aws ecr create-repository --repository-name insurance-agent --region "$AWS_REGION"
aws logs create-log-group --log-group-name /ecs/insurance-agent --region "$AWS_REGION"
aws ecr get-login-password --region "$AWS_REGION" |
  docker login --username AWS --password-stdin "$ECR_REGISTRY"
docker build --platform linux/amd64 -f agent/Dockerfile.ecs \
  -t "$ECR_REGISTRY/insurance-agent:$IMAGE_TAG" agent
docker push "$ECR_REGISTRY/insurance-agent:$IMAGE_TAG"
```

The agent Dockerfile runs as a non-root user and starts
`amp-instrument python main.py`.

## 4. Register and run the agent task

Copy [ecs/agent-task-definition.json](ecs/agent-task-definition.json) into the
sample directory and replace every `<...>` placeholder: execution-role ARN, full
image URI, Region, gateway/telemetry URLs, and secret ARN. Keep Region/model consistent with
the provider. The template uses `amazon.nova-micro-v1:0` in `us-east-1`, 0.5 vCPU,
1 GiB RAM, and `X86_64` to match the image. Do not commit the filled file.

```bash
cp ecs/agent-task-definition.json agent-task-definition.json
```

After filling it, register and run the task:

```bash
TASK_DEFINITION_ARN=$(aws ecs register-task-definition \
  --cli-input-json file://agent-task-definition.json --region "$AWS_REGION" \
  --query taskDefinition.taskDefinitionArn --output text)
TASK_ARN=$(aws ecs run-task --cluster "$ECS_CLUSTER" \
  --task-definition "$TASK_DEFINITION_ARN" --launch-type FARGATE \
  --platform-version 1.4.0 --region "$AWS_REGION" \
  --network-configuration \
  'awsvpcConfiguration={subnets=[<private-subnet-id>],securityGroups=[<agent-security-group-id>],assignPublicIp=DISABLED}' \
  --query 'tasks[0].taskArn' --output text)
aws ecs wait tasks-running --cluster "$ECS_CLUSTER" --tasks "$TASK_ARN" --region "$AWS_REGION"
aws ecs describe-tasks --cluster "$ECS_CLUSTER" --tasks "$TASK_ARN" --region "$AWS_REGION"
```

If `run-task` returns `None`, rerun without `--query` to inspect `failures` before
waiting. In **ECS → Cluster → Tasks → task → Networking**, find the private IP.
From your connected test client:

```bash
export AGENT_URL="http://<task-private-ip>:8000"
curl --fail "$AGENT_URL/health"
curl --fail "$AGENT_URL/chat" -H 'Content-Type: application/json' \
  -d '{"session_id":"policy-baseline-1","message":"List my insurance policies using your tools."}'
```

Expect `OZ-AUTO-4417`, `OZ-HOME-2280`, and `OZ-TRAV-9153`. For stopped tasks,
inspect `stoppedReason`/container `reason`; for startup or export problems check
`/ecs/insurance-agent` in CloudWatch. Check gateway URL/key, Bedrock key expiry,
model permissions, telemetry endpoint/token, and network connectivity.

This is a standalone task. For an existing ECS service, register the revised task
definition and update that service through your normal deployment workflow.
Session IDs are caller-supplied and state is in memory. Use TLS, user authentication,
and customer-level authorization before allowing production callers.

## 5. Observe the agent

1. Open the agent's **Traces** page and select the request's time window.
2. Open the trace and find the agent, model, and `list_policies` spans. Confirm
   the tool result contains the three policy IDs.
3. Compare span durations and errors to separate slow model calls from slow tools.
   Nested framework and SDK spans can describe the same model call, so do not sum
   every token count.
4. With a new session ID, ask for policy `OZ-UNKNOWN`. The lookup returns an
   `error` field as data, not a span error, so inspect the content too.

## 6. Add a gateway policy

Follow [Add a prompt policy](BEDROCK-GATEWAY.md#add-a-prompt-policy). Send a
policy-list request and `Write a poem about space travel`, each with a new session
ID, before and after attaching the policy.

## 7. Evaluate and monitor

1. Send three requests with distinct session IDs: list all policies, look up
   `OZ-UNKNOWN`, and file a claim for `OZ-AUTO-4417`. With local tools, the claim
   request calls `file_claim` against in-memory data. With `USE_MCP=true`, the
   agent has no claim tool. Record the trace IDs.
2. Choose **Evaluation**, then choose **Add Monitor**. Select **Past Traces**, set the
   request window, add **Step Success Rate** with `min_success_rate=1.0`, and
   choose **Create Monitor**.
3. For a window containing only policy-list requests, create a second monitor
   with **Content Coverage** and `required_strings` set to
   `["OZ-AUTO-4417", "OZ-HOME-2280", "OZ-TRAV-9153"]`.
4. Review each score and explanation. Executions with no tool steps are skipped,
   not scored as zero, and the `OZ-UNKNOWN` lookup can still score as successful.
5. After one prompt, policy, model, or tool change, repeat the same requests and
   compare runs. For new traffic, create a **Future Traces** monitor with a
   five-minute interval.

## 8. Deploy the MCP extension

1. Create ECR repository `insurance-mcp`, log group `/ecs/insurance-mcp`, and
   Secrets Manager JSON secret `insurance-demo/mcp` with a generated nonempty
   `INSURANCE_MCP_UPSTREAM_KEY`. Grant the execution role access to the secret.
2. Build from the sample root and push:

   ```bash
   docker build --platform linux/amd64 -f mcp-server/Dockerfile \
     -t "$ECR_REGISTRY/insurance-mcp:$IMAGE_TAG" .
   docker push "$ECR_REGISTRY/insurance-mcp:$IMAGE_TAG"
   ```

3. Fill and register [ecs/mcp-task-definition.json](ecs/mcp-task-definition.json)
   with the same `register-task-definition` command. Create an **IP** target
   group in your VPC using HTTP port 8001, health path `/health`, and HTTP 200.
   ECS registers target IPs; do not register them manually.
4. Create an internet-facing Application Load Balancer in two public subnets
   in separate Availability Zones. Permit inbound HTTPS 443 from the SaaS gateway
   egress addresses if available; otherwise permit internet HTTPS for this
   synthetic example and authenticate with the upstream secret. The MCP task
   security group permits TCP 8001 **only from the ALB security group**. Permit
   the ALB outbound access to that task port.
5. Use a domain you control and an AWS Certificate Manager certificate in the
   same Region. Configure an HTTPS 443 listener forwarding to the target group
   and a DNS record pointing the domain to the ALB. Preserve the upstream secret
   header. Do not use a certificate-mismatched raw ALB hostname as the HTTPS URL.
6. In **ECS → Cluster → Services → Create**, select the MCP task definition,
   Fargate Linux 1.4.0 or later, desired count 1, private subnets, and the MCP task
   security group. Disable public IP assignment. Attach the existing ALB listener
   and IP target group to container `insurance-mcp`, port 8001. Set a 60-second
   health-check grace period and wait for a healthy target.
7. Verify `https://<mcp-domain>/health` returns `{"status":"ok"}` and an
   unauthenticated `/mcp` request returns HTTP 401. Follow
   [MCP-TOOLS.md](MCP-TOOLS.md) to register the upstream, configure scopes/roles,
   and attach the proxy. `/health` is public and returns only static status.
8. Set `USE_MCP=true` and the MCP/AgentID variables from that guide on the agent.
   Add `AMP_AGENTID_CLIENT_SECRET` to its JSON secret and task-definition `secrets`.
   Register a new revision and replace the task. Verify allowed and denied access.
   Failed MCP authorization can prevent startup; inspect logs instead of bypassing it.

The upstream key belongs only in the MCP container and proxy connection. The
agent receives AgentID credentials and calls the proxy, not the upstream server.

## Clean up

Suspend evaluation monitors. Delete the MCP service (or scale it to zero first),
stop standalone tasks, and remove any test agent service that would replace tasks.
Delete test ALBs/listeners and target groups, then test security groups/DNS records;
preserve shared certificates and networking. Remove test images/repositories,
log groups, secrets according to your recovery policy, and dedicated IAM resources.
Deregister task definitions and delete eligible revisions.

Remove Agent Manager bindings, roles/scopes, proxy, dedicated provider, and agent;
revoke dedicated credentials and apply trace retention. Deleting monitors removes
their run history/scores: retain needed evidence first. Check for retained ALBs,
NAT gateways, public IPv4 addresses, and storage that can still incur charges.

## References and validation boundary

- [ECS task execution role](https://docs.aws.amazon.com/AmazonECS/latest/developerguide/task_execution_IAM_role.html)
- [Secrets Manager values in ECS](https://docs.aws.amazon.com/AmazonECS/latest/developerguide/secrets-envvar-secrets-manager.html)
- [Application Load Balancers with ECS](https://docs.aws.amazon.com/AmazonECS/latest/developerguide/alb.html)

These templates require your AWS and Agent Manager values. Offline sample tests
do not establish a successful deployment in either service.

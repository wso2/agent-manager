# Copyright (c) 2026, WSO2 LLC. (https://www.wso2.com).
#
# WSO2 LLC. licenses this file to you under the Apache License,
# Version 2.0 (the "License"); you may not use this file except
# in compliance with the License.
# You may obtain a copy of the License at
#
# http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing,
# software distributed under the License is distributed on an
# "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
# KIND, either express or implied.  See the License for the
# specific language governing permissions and limitations
# under the License.

# Shadow agent runner image (linux/amd64 and linux/arm64).
# Build context: shadow-agent-runner/ (the build action's default context).
# Debian slim, not Alpine: tree-sitter publishes no musllinux arm64 wheels.

############### STAGE 1: BUILD THE VENV ###############
FROM public.ecr.aws/docker/library/python:3.12-slim AS builder
ARG VERSION=0.0.0-dev

ENV PIP_DISABLE_PIP_VERSION_CHECK=1 \
    PIP_NO_CACHE_DIR=1

WORKDIR /build

# Build backend for the runner's own wheel; it stays in this stage.
RUN pip install "hatchling==1.32.4" && python -m venv /opt/venv

# Runtime dependencies: exact, hash-checked, binary wheels only. Never resolve live.
COPY locks/requirements-linux-py312.txt ./
RUN /opt/venv/bin/pip install --require-hashes --no-deps --only-binary=:all: -r requirements-linux-py312.txt

# The runner itself, stamped with the build VERSION. pip is then removed from the venv.
COPY pyproject.toml README.md ./
COPY src/ ./src/
RUN case "$VERSION" in ''|*[!A-Za-z0-9._+-]*) echo "invalid VERSION '$VERSION'" >&2; exit 1;; esac \
    && sed -i "s/^__version__ = .*/__version__ = \"${VERSION}\"/" src/amp_shadow_runner/_version.py \
    && grep -qx "__version__ = \"${VERSION}\"" src/amp_shadow_runner/_version.py \
    && pip wheel --no-deps --no-build-isolation --wheel-dir /wheels . \
    && /opt/venv/bin/pip install --no-deps /wheels/*.whl \
    && /opt/venv/bin/pip uninstall --yes pip

############### STAGE 2: RUNTIME IMAGE ###############
FROM public.ecr.aws/docker/library/python:3.12-slim
ARG VERSION=0.0.0-dev

LABEL org.opencontainers.image.source="https://github.com/wso2/agent-manager"
LABEL org.opencontainers.image.title="amp-shadow-runner"
LABEL org.opencontainers.image.description="WSO2 Agent Manager shadow agent runner: finds AI agents in source code repositories"
LABEL org.opencontainers.image.licenses="Apache-2.0"
LABEL org.opencontainers.image.version="${VERSION}"

# Non-root user 10001 by default. The console runs the image with
# --user "$(id -u):$(id -g)", so nothing may depend on uid 10001: the only
# writable paths the runner needs are /tmp and the /out mount, and the root
# file system can be mounted read-only.
# pip is not needed at runtime, so the base image's copy is removed too.
RUN groupadd --system --gid 10001 amp \
    && useradd --system --uid 10001 --gid 10001 --home-dir /tmp --no-create-home \
       --shell /usr/sbin/nologin amp \
    && python -m pip uninstall --yes pip

COPY --from=builder /opt/venv /opt/venv

ENV PATH="/opt/venv/bin:${PATH}" \
    HOME=/tmp \
    PYTHONDONTWRITEBYTECODE=1 \
    PYTHONUNBUFFERED=1

WORKDIR /tmp
USER 10001:10001

# Each command runs once and exits, so there is nothing to health-check.
HEALTHCHECK NONE

ENTRYPOINT ["amp-shadow-runner"]
CMD ["--help"]

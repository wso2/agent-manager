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

# Production Dockerfile for AMP Evaluation Monitor Job
# Installs amp-evaluation SDK from PyPI registry
# Build context: evaluation-job directory

FROM python:3.11-alpine

# Build arg for SDK version - must be provided for production builds
ARG AMP_EVALUATION_VERSION

LABEL org.opencontainers.image.source="https://github.com/wso2/ai-agent-management-platform"
LABEL org.opencontainers.image.description="AMP Evaluation Monitor - AI Agent evaluation job for monitoring workflows"
LABEL org.opencontainers.image.licenses="Apache-2.0"

WORKDIR /app

# Install system dependencies
RUN apk add --no-cache git

# Install amp-evaluation SDK from PyPI
RUN pip install --no-cache-dir amp-evaluation==${AMP_EVALUATION_VERSION}

# Copy the job entrypoint
COPY main.py ./

# Create non-root user
RUN adduser -D -u 1000 ampuser && \
    chown -R ampuser:ampuser /app

USER ampuser

# Set Python to run in unbuffered mode for better logging
ENV PYTHONUNBUFFERED=1

# Run the monitor job
ENTRYPOINT ["python", "main.py"]

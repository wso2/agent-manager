# Copyright (c) 2025, WSO2 LLC. (https://www.wso2.com).
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

"""
Environment variable constants for WSO2 Agent Management Platform instrumentation.
This module centralizes all environment variable names used for configuration.
"""

# Application Configuration
AMP_AGENT_NAME = "AMP_AGENT_NAME"
AMP_OTEL_ENDPOINT = "AMP_OTEL_ENDPOINT"
AMP_AGENT_API_KEY = "AMP_AGENT_API_KEY"
AMP_TRACE_CONTENT = "AMP_TRACE_CONTENT"

# Downstream environment variables that get set for Traceloop
TRACELOOP_TRACE_CONTENT = "TRACELOOP_TRACE_CONTENT"
TRACELOOP_METRICS_ENABLED = "TRACELOOP_METRICS_ENABLED"
OTEL_EXPORTER_OTLP_INSECURE = "OTEL_EXPORTER_OTLP_INSECURE"

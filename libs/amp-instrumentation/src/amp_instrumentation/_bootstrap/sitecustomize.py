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
This module is automatically loaded by Python at startup when PYTHONPATH includes
the _bootstrap directory. It initializes WSO2 AMP instrumentation before any user code runs.
"""

import logging
import sys
from amp_instrumentation._bootstrap.initialization import (
    configure_logging,
    initialize_instrumentation,
)

# Initialize automatically when this module is loaded
try:
    # Configure logging for the entire package
    configure_logging()

    # Get logger for this module
    logger = logging.getLogger(__name__)

    initialize_instrumentation()
    logger.info("WSO2 AMP instrumentation initialized successfully")
except Exception as e:
    # Print error directly to stderr to ensure visibility
    print(f"ERROR: Failed to initialize WSO2 AMP instrumentation: {e}", file=sys.stderr)
    # Also log for debug mode
    logger = logging.getLogger(__name__)
    logger.exception(f"Failed to initialize WSO2 AMP instrumentation: {e}")

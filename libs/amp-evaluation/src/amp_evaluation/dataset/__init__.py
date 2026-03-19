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

"""
Dataset module - everything related to datasets and tasks.

Public API:
    >>> from amp_evaluation.dataset import (
    ...     Task, Dataset, Constraints, TrajectoryStep,
    ...     generate_id,
    ...     load_dataset_from_json, load_dataset_from_csv, save_dataset_to_json,
    ... )
"""

from .models import Task, Dataset, Constraints, TrajectoryStep, generate_id
from .loader import load_dataset_from_json, load_dataset_from_csv, save_dataset_to_json

__all__ = [
    # Data models
    "Task",
    "Dataset",
    "Constraints",
    "TrajectoryStep",
    "generate_id",
    # Loading/saving
    "load_dataset_from_json",
    "load_dataset_from_csv",
    "save_dataset_to_json",
]

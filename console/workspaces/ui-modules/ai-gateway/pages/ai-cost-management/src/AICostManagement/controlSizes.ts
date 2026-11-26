/**
 * Copyright (c) 2026, WSO2 LLC. (https://www.wso2.com).
 *
 * WSO2 LLC. licenses this file to you under the Apache License,
 * Version 2.0 (the "License"); you may not use this file except
 * in compliance with the License. You may obtain a copy of the License at
 *
 * http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing,
 * software distributed under the License is distributed on an
 * "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
 * KIND, either express or implied. See the License for the
 * specific language governing permissions and limitations
 * under the License.
 */

/**
 * Single-line controls that sit in one row share a height, so the row lines up. A toolbar
 * matches the shared search bar (40px); a form matches the console's text inputs (35.7px).
 * A plain select has 10px of padding where a text input has 8.5px, and a toggle button
 * group is taller still, so they are set here once instead of drifting apart.
 */

/** A select or a picker beside the search bar, or beside a field of that height. */
export const TOOLBAR_FIELD_SX = {
  "& .MuiInputBase-root": { minHeight: 40 },
} as const;

/** A select among a form's text inputs. */
export const FORM_SELECT_SX = {
  "& .MuiSelect-select": { py: "8.5px" },
} as const;

/** The buttons of a toggle group beside a form's text inputs. */
export const FORM_TOGGLE_BUTTON_SX = { height: 35.7, py: 0 } as const;

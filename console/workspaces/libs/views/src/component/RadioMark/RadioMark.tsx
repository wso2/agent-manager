/**
 * Copyright (c) 2026, WSO2 LLC. (https://www.wso2.com).
 *
 * WSO2 LLC. licenses this file to you under the Apache License,
 * Version 2.0 (the "License"); you may not use this file except
 * in compliance with the License.
 * You may obtain a copy of the License at
 *
 * http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing,
 * software distributed under the License is distributed on an
 * "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
 * KIND, either express or implied.  See the License for the
 * specific language governing permissions and limitations
 * under the License.
 */

import { Box } from "@wso2/oxygen-ui";
import { Check } from "@wso2/oxygen-ui-icons-react";

export interface RadioMarkProps {
  selected: boolean;
}

/**
 * The round marker at the end of a selection card: an empty ring, or a filled disc with a
 * check once selected. Decorative; the card it sits in carries the radio semantics.
 */
export function RadioMark({ selected }: RadioMarkProps) {
  return (
    <Box
      aria-hidden="true"
      sx={{
        width: 20,
        height: 20,
        flexShrink: 0,
        borderRadius: "50%",
        display: "flex",
        alignItems: "center",
        justifyContent: "center",
        border: "2px solid",
        borderColor: selected ? "primary.main" : "text.disabled",
        bgcolor: selected ? "primary.main" : "transparent",
        color: "primary.contrastText",
      }}
    >
      {selected && <Check size={12} strokeWidth={3} />}
    </Box>
  );
}

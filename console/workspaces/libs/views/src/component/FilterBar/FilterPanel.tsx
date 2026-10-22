/**
 * Copyright (c) 2026, WSO2 LLC. (https://www.wso2.com).
 *
 * WSO2 LLC. licenses this file to you under the Apache License,
 * Version 2.0 (the "License"); you may not use this file except
 * in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing,
 * software distributed under the License is distributed on an
 * "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
 * KIND, either express or implied. See the License for the
 * specific language governing permissions and limitations
 * under the License.
 */

import React, { type ReactNode, useCallback, useEffect } from "react";
import {
  Button,
  Grow,
  IconButton,
  Paper,
  Popper,
  type PopperProps,
  Stack,
  Typography,
} from "@wso2/oxygen-ui";
import { X as CloseIcon } from "@wso2/oxygen-ui-icons-react";

export interface FilterPanelProps {
  /** The Filters button the panel hangs from. */
  anchorEl: HTMLElement;
  /** For the Filters button's aria-controls. */
  id: string;
  /** The heading and the panel's accessible name. */
  title: string;
  width: number;
  /** Blocks Filter, e.g. while a value is invalid. */
  applyDisabled?: boolean;
  onReset: () => void;
  onApply: () => void;
  onClose: () => void;
  /** The fields; they scroll only if the window is short. */
  children: ReactNode;
}

// Space between the Filters button and the panel.
const PANEL_GAP = 6;
const WINDOW_BOTTOM_MARGIN = 16;
const MAX_HEIGHT_PROPERTY = "--filter-panel-max-height";

// Places the panel below the button and caps its height at the space left above the
// window's bottom. Popper reruns this on resize and scroll.
const PANEL_MODIFIERS: PopperProps["modifiers"] = [
  { name: "offset", options: { offset: [0, PANEL_GAP] } },
  {
    name: "fitBelowAnchor",
    enabled: true,
    phase: "beforeWrite",
    fn: ({ state }) => {
      const anchorBottom = state.elements.reference.getBoundingClientRect().bottom;
      state.elements.popper.style.setProperty(
        MAX_HEIGHT_PROPERTY,
        `${window.innerHeight - anchorBottom - PANEL_GAP - WINDOW_BOTTOM_MARGIN}px`,
      );
    },
  },
];

/** Panel under the Filters button that edits a draft; Filter applies it, Escape discards it. */
export const FilterPanel: React.FC<FilterPanelProps> = ({
  anchorEl,
  id,
  title,
  width,
  applyDisabled = false,
  onReset,
  onApply,
  onClose,
  children,
}) => {
  // A callback ref, as the panel mounts in the portal after this component.
  const focusOnMount = useCallback(
    (el: HTMLElement | null) => el?.focus({ preventScroll: true }),
    [],
  );

  // Escape anywhere closes the panel; an open menu takes its own Escape first.
  useEffect(() => {
    const onKeyDown = (e: KeyboardEvent) => {
      if (e.key === "Escape") onClose();
    };
    document.addEventListener("keydown", onKeyDown);
    return () => document.removeEventListener("keydown", onKeyDown);
  }, [onClose]);

  return (
    <Popper
      open
      anchorEl={anchorEl}
      placement="bottom-end"
      transition
      // Not a tooltip; the panel inside is the landmark.
      role="presentation"
      modifiers={PANEL_MODIFIERS}
      sx={{ zIndex: (theme) => theme.zIndex.mobileStepper }}
    >
      {({ TransitionProps, placement }) => (
        <Grow
          {...TransitionProps}
          style={{
            transformOrigin: placement.startsWith("top") ? "right bottom" : "right top",
          }}
        >
          <Paper
            ref={focusOnMount}
            id={id}
            component="aside"
            aria-label={title}
            tabIndex={-1}
            elevation={8}
            sx={{
              maxHeight: `var(${MAX_HEIGHT_PROPERTY})`,
              width,
              maxWidth: "calc(100vw - 32px)",
              display: "flex",
              flexDirection: "column",
              border: 1,
              borderColor: "divider",
              borderRadius: 1.5,
              overflow: "hidden",
              outline: "none",
              // Opaque, so the list underneath doesn't show through.
              bgcolor: "background.default",
              backgroundImage: "none",
              backdropFilter: "none",
            }}
          >
            <Stack
              direction="row"
              alignItems="center"
              justifyContent="space-between"
              sx={{ pl: 2.5, pr: 1.5, py: 1.5, borderBottom: 1, borderColor: "divider" }}
            >
              <Typography variant="h6" component="h2">
                {title}
              </Typography>
              <IconButton
                size="small"
                aria-label={`Close ${title.toLowerCase()}`}
                onClick={onClose}
              >
                <CloseIcon size={18} />
              </IconButton>
            </Stack>
            <Stack sx={{ flex: 1, minHeight: 0, overflowY: "auto", px: 2.5, py: 2 }}>
              {children}
            </Stack>
            <Stack
              direction="row"
              alignItems="center"
              justifyContent="space-between"
              sx={{ px: 2.5, py: 1.5, borderTop: 1, borderColor: "divider" }}
            >
              <Button variant="text" onClick={onReset}>
                Reset
              </Button>
              <Button variant="contained" disabled={applyDisabled} onClick={onApply}>
                Filter
              </Button>
            </Stack>
          </Paper>
        </Grow>
      )}
    </Popper>
  );
};

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

import { useMemo, useState } from "react";
import {
  Alert,
  Box,
  Button,
  Dialog,
  DialogActions,
  DialogContent,
  DialogTitle,
  Divider,
  Form,
  IconButton,
  InputAdornment,
  Link,
  MenuItem,
  Stack,
  Typography,
} from "@wso2/oxygen-ui";
import { ExternalLink, X } from "@wso2/oxygen-ui-icons-react";
import { Link as RouterLink, useLocation, useNavigate } from "react-router-dom";
import {
  useCreateBudget,
  useListBudgets,
  useListEventDestinations,
} from "@agent-management-platform/api-client";
import { returnToState, TextInput } from "@agent-management-platform/views";
import { INPUT_LIMITS } from "@agent-management-platform/types";
import {
  budgetFromForm,
  duplicateNameError,
  formFor,
  handleFor,
  serverFieldError,
  startQuery,
  validateBudgetForm,
  type BudgetFormState,
  type BudgetStart,
} from "../AICostManagement/budgetForm";
import {
  Channels,
  Segmented,
  Thresholds,
} from "../AICostManagement/subComponents/BudgetForm";
import { PERIOD_LABELS } from "../AICostManagement/subComponents/budgetLabels";
import { FORM_SELECT_SX } from "../AICostManagement/controlSizes";

export interface QuickBudgetDialogProps {
  open: boolean;
  onClose: () => void;
  orgName: string;
  /** What the budget is for, filled in and fixed (UX 2). */
  start: BudgetStart;
  /** "Project", "Agent" or "LLM provider", and the entity's name as shown. */
  kind: string;
  label: string;
  /** The full create page, which "Advanced create" opens with the same start. */
  createHref: string;
  /** A budget's detail page, opened after it is created. */
  viewHref: (budgetName: string) => string;
}

/**
 * The quick form for a budget started from a project, agent or LLM provider page: name,
 * amount, period, action and warnings, with the entity filled in. "Advanced create"
 * opens the full form with the same entity (UX 2).
 */
export function QuickBudgetDialog({
  open,
  onClose,
  orgName,
  start,
  kind,
  label,
  createHref,
  viewHref,
}: QuickBudgetDialogProps) {
  const navigate = useNavigate();
  const location = useLocation();
  const budgets = useListBudgets({ orgName }, { limit: 100 });
  const destinations = useListEventDestinations(orgName, open);
  const create = useCreateBudget();
  const [initialForm] = useState<BudgetFormState>(() => formFor(start));
  const [form, setForm] = useState<BudgetFormState>(initialForm);
  const [submitted, setSubmitted] = useState(false);
  const [serverErrors, setServerErrors] = useState<Record<string, string>>({});
  const set = <K extends keyof BudgetFormState>(
    key: K,
    value: BudgetFormState[K],
  ) => {
    setServerErrors({});
    setForm((f) => ({ ...f, [key]: value }));
  };
  const duplicateName = duplicateNameError(
    form.displayName,
    budgets.data?.budgets ?? [],
  );
  const errors: Record<string, string | undefined> = {
    ...(submitted ? validateBudgetForm(form) : {}),
    ...(submitted && duplicateName ? { displayName: duplicateName } : {}),
    ...serverErrors,
  };
  // Typed input is not thrown away by a stray click outside or Escape; Cancel and the
  // close button still close.
  const dirty = JSON.stringify(form) !== JSON.stringify(initialForm);
  const closeUnlessDirty = (
    _: unknown,
    reason?: "backdropClick" | "escapeKeyDown",
  ) => {
    if (dirty && (reason === "backdropClick" || reason === "escapeKeyDown"))
      return;
    onClose();
  };
  const taken = useMemo(
    () => (budgets.data?.budgets ?? []).map((b) => b.metadata.name),
    [budgets.data],
  );
  const channelOptions = (destinations.data?.destinations ?? []).map(
    (d) => d.name,
  );

  const save = () => {
    setSubmitted(true);
    if (Object.keys(validateBudgetForm(form)).length || duplicateName) return;
    const body = budgetFromForm(form, {
      name: handleFor(form.displayName, taken),
    });
    create.mutate(
      { params: { orgName }, body },
      {
        onSuccess: (b) => {
          onClose();
          navigate(viewHref(b.metadata.name));
        },
        onError: (err) => {
          const { field, message } = serverFieldError(err);
          // The dialog has name, amount, period, action and warnings; the rest show above.
          const shown = ["displayName", "amount", "thresholds", "channels"];
          setServerErrors({
            [shown.includes(field) ? field : "form"]: message,
          });
        },
      },
    );
  };

  return (
    <Dialog open={open} onClose={closeUnlessDirty} fullWidth maxWidth="sm">
      <DialogTitle sx={{ pr: 6 }}>
        Add budget for {label}
        <IconButton
          aria-label="Close"
          onClick={onClose}
          sx={{ position: "absolute", right: 12, top: 12 }}
        >
          <X size={18} />
        </IconButton>
      </DialogTitle>
      <DialogContent>
        <Stack spacing={2.5} sx={{ pt: 0.5 }}>
          <Stack
            direction="row"
            justifyContent="space-between"
            alignItems="center"
            sx={{
              border: 1,
              borderColor: "divider",
              borderRadius: 1.5,
              px: 2,
              py: 1.5,
            }}
          >
            <Typography variant="body2" color="text.secondary">
              {kind}
            </Typography>
            <Typography variant="body2" fontWeight={600}>
              {label}
            </Typography>
          </Stack>
          <TextInput
            label="Name"
            required
            fullWidth
            value={form.displayName}
            onChange={(e) => set("displayName", e.target.value)}
            placeholder={`e.g. ${label} monthly budget`}
            error={!!errors.displayName}
            helperText={errors.displayName}
            maxLength={INPUT_LIMITS.NAME}
            hideCharacterCount
          />
          <Stack direction={{ xs: "column", sm: "row" }} spacing={2}>
            <TextInput
              label="Amount"
              required
              fullWidth
              value={form.amount}
              onChange={(e) => set("amount", e.target.value)}
              placeholder="e.g. 1000"
              error={!!errors.amount}
              helperText={errors.amount}
              inputMode="decimal"
              slotProps={{
                input: {
                  startAdornment: (
                    <InputAdornment position="start">$</InputAdornment>
                  ),
                  endAdornment: (
                    <InputAdornment position="end">USD</InputAdornment>
                  ),
                },
              }}
            />
            <TextInput
              select
              label="Period"
              fullWidth
              sx={FORM_SELECT_SX}
              value={form.period}
              onChange={(e) =>
                set("period", e.target.value as BudgetFormState["period"])
              }
            >
              {(["day", "week", "month"] as const).map((p) => (
                <MenuItem key={p} value={p}>
                  {PERIOD_LABELS[p]}
                </MenuItem>
              ))}
            </TextInput>
          </Stack>
          <Form.ElementWrapper
            label="When the limit is reached"
            name="When the limit is reached"
          >
            <Box sx={{ maxWidth: 280 }}>
              <Segmented
                label="When the limit is reached"
                value={form.action}
                onChange={(v) => set("action", v)}
                options={[
                  { value: "block", label: "Block" },
                  { value: "allow", label: "Allow" },
                ]}
              />
            </Box>
            <Typography variant="body2" color="text.secondary" sx={{ mt: 1 }}>
              {form.action === "block"
                ? "Refuse requests at the limit."
                : "Only warns."}
            </Typography>
          </Form.ElementWrapper>
          <Divider />
          <Box>
            <Typography variant="subtitle1" sx={{ mb: 1 }}>
              Alerts{" "}
              <Typography
                component="span"
                variant="body2"
                color="text.secondary"
              >
                (optional)
              </Typography>
            </Typography>
            <Thresholds
              value={form.thresholds}
              onChange={(v) => set("thresholds", v)}
              error={errors.thresholds}
            />
          </Box>
          {form.thresholds.length > 0 && (
            <Channels
              value={form.channels}
              onChange={(v) => set("channels", v)}
              options={channelOptions}
              list={{
                noun: "notification channels",
                loading: destinations.isLoading,
                failed: !!destinations.error,
                retry: () => void destinations.refetch(),
              }}
              error={errors.channels}
            />
          )}
          {errors.form ? (
            <Alert severity="error">{errors.form}</Alert>
          ) : (
            submitted &&
            Object.values(errors).some(Boolean) && (
              <Alert severity="error">Fix the highlighted fields.</Alert>
            )
          )}
          <Link
            component={RouterLink}
            to={`${createHref}${startQuery(start)}`}
            state={{ ...returnToState(location), draft: form }}
            underline="hover"
            sx={{ display: "inline-flex", alignItems: "center", gap: 0.5 }}
          >
            Advanced create
            <ExternalLink size={14} />
          </Link>
        </Stack>
      </DialogContent>
      <DialogActions sx={{ px: 3, pb: 2.5 }}>
        <Button
          variant="outlined"
          onClick={onClose}
          disabled={create.isPending}
        >
          Cancel
        </Button>
        <Button variant="contained" onClick={save} disabled={create.isPending}>
          Create budget
        </Button>
      </DialogActions>
    </Dialog>
  );
}

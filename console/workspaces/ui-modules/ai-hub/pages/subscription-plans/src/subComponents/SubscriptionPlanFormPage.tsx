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

import { useEffect, useMemo, useRef, useState, type FormEvent } from "react";
import {
  AdapterDateFns,
  Alert,
  AlertTitle,
  Box,
  Button,
  CircularProgress,
  DatePickers,
  Form,
  FormControl,
  FormHelperText,
  FormLabel,
  Grid,
  Stack,
  Typography,
} from "@wso2/oxygen-ui";
import { SearchX } from "@wso2/oxygen-ui-icons-react";
import { generatePath, useNavigate, useParams } from "react-router-dom";
import {
  SUBSCRIPTION_PLAN_EXISTS,
  subscriptionPlanErrorCode,
  useCreateSubscriptionPlan,
  useGetSubscriptionPlan,
  useUpdateSubscriptionPlan,
} from "@agent-management-platform/api-client";
import { useUnsavedChangesGuard } from "@agent-management-platform/shared-component";
import { PageLayout, StateNotice, TextInput, useReturnTo } from "@agent-management-platform/views";
import { absoluteRouteMap, type SubscriptionPlan } from "@agent-management-platform/types";
import { fromExpiryTime, toExpiryTime } from "./expiryTime";
import { PlanConditionsEditor } from "./PlanConditionsEditor";
import {
  checkConditions,
  draftsFromConditions,
  MAX_DESCRIPTION_CHARS,
  type ConditionDraft,
} from "./planConditions";

/** The form's element id, so the header's Save button can submit it. */
const FORM_ID = "subscription-plan-form";

/** The longest plan handle the API keeps. */
export const MAX_HANDLE_CHARS = 40;
/** The longest plan name the API keeps. */
export const MAX_NAME_CHARS = 255;

/** Characters as the API counts them (runes), not UTF-16 code units. */
const charCount = (value: string) => Array.from(value).length;

/** The URL-friendly handle the API needs on create; the user never sees it. */
export const slugify = (value: string): string =>
  value
    .trim()
    .toLowerCase()
    .replace(/[^a-z0-9]+/g, "-")
    .replace(/^-+|-+$/g, "")
    .slice(0, MAX_HANDLE_CHARS)
    .replace(/-+$/, "");

type FormState = {
  displayName: string;
  description: string;
  /** The picked calendar day (local), or null for no expiry. An Invalid Date while it is being
   * typed. */
  expiryDate: Date | null;
};

const formOf = (plan?: SubscriptionPlan): FormState => ({
  description: plan?.description ?? "",
  displayName: plan?.displayName ?? "",
  expiryDate: plan?.expiryTime ? fromExpiryTime(plan.expiryTime) : null,
});

/** Create (`…/subscription-plans/add`) or edit (`…/subscription-plans/edit/:planId`) a plan.
 * Status is not edited here: it is the switch in the list, so a new plan is ACTIVE and an edit
 * keeps it. */
export function SubscriptionPlanFormPage() {
  const { orgId = "", planId } = useParams<{ orgId: string; planId?: string }>();
  const isEdit = Boolean(planId);
  const navigate = useNavigate();
  const listPath = generatePath(absoluteRouteMap.children.org.children.subscriptionPlans.path, {
    orgId,
  });
  const returnTo = useReturnTo(listPath);

  // The plan with its usage: a plan in use keeps its name and conditions (the server refuses a
  // change);
  // its description and expiry can still change.
  const planQuery = useGetSubscriptionPlan({ orgName: orgId, planId });
  const plan = planQuery.data;
  const inUse = isEdit && plan?.usage?.inUse === true;

  const createPlan = useCreateSubscriptionPlan();
  const updatePlan = useUpdateSubscriptionPlan();
  const busy = createPlan.isPending || updatePlan.isPending;

  const [form, setForm] = useState<FormState>(() => formOf());
  const [conditions, setConditions] = useState<ConditionDraft[]>([]);
  const [session, setSession] = useState(0);
  const [submitted, setSubmitted] = useState(false);
  const [nameVisited, setNameVisited] = useState(false);
  const [conflict, setConflict] = useState(false);
  const [failure, setFailure] = useState<string | null>(null);
  // The plan the form was filled from (null for a new plan), so a refetch (e.g. on window focus)
  // doesn't replace the user's edits or reset what counts as unsaved.
  const loadedFor = useRef<string | null | undefined>(undefined);
  // What the form was loaded with, so only the user's changes make it dirty.
  const initial = useRef<{ form: FormState; conditions: string }>({
    conditions: "[]",
    form: formOf(),
  });

  useEffect(() => {
    if (isEdit && plan?.id !== planId) return;
    const target = planId ?? null;
    if (loadedFor.current === target) return;
    loadedFor.current = target;
    const loaded = formOf(plan);
    const drafts = draftsFromConditions(plan?.conditions);
    setForm(loaded);
    setConditions(drafts);
    setSession((current) => current + 1);
    initial.current = { conditions: JSON.stringify(checkConditions(drafts).body), form: loaded };
  }, [isEdit, plan, planId]);

  const trimmedName = form.displayName.trim();
  const trimmedDescription = form.description.trim();
  const slug = slugify(trimmedName);
  // A plan in use keeps the conditions it has: they are neither checked nor sent.
  const conditionsCheck = checkConditions(conditions);
  const hasConditionsError = !inUse && conditionsCheck.body === null;

  // An empty name is reported before a bad handle, and a taken handle only after a real submit.
  const nameError =
    trimmedName.length === 0
      ? "Enter a display name"
      : charCount(trimmedName) > MAX_NAME_CHARS
        ? `Use at most ${MAX_NAME_CHARS} characters`
        : !isEdit && slug.length === 0
          ? "Use letters or numbers in the name"
          : conflict
            ? "A plan with this name already exists"
            : null;
  const showNameError = (submitted || nameVisited) && nameError !== null;
  const descriptionLength = charCount(trimmedDescription);
  const descriptionError =
    descriptionLength > MAX_DESCRIPTION_CHARS
      ? `Use at most ${MAX_DESCRIPTION_CHARS.toLocaleString()} characters (${descriptionLength.toLocaleString()} now).`
      : null;
  const hasExpiryError = form.expiryDate !== null && Number.isNaN(form.expiryDate.getTime());

  const isDirty = useMemo(() => {
    const start = initial.current;
    const day = (date: Date | null) =>
      date && !Number.isNaN(date.getTime()) ? toExpiryTime(date) : String(date);
    return (
      form.displayName !== start.form.displayName ||
      form.description !== start.form.description ||
      day(form.expiryDate) !== day(start.form.expiryDate) ||
      JSON.stringify(conditionsCheck.body) !== start.conditions
    );
  }, [form, conditionsCheck.body]);
  const { allowNavigation } = useUnsavedChangesGuard(isDirty && !busy);

  const canSubmit =
    !nameError && !descriptionError && !hasConditionsError && !hasExpiryError && !busy;

  const leave = () => allowNavigation(() => navigate(returnTo));

  const handleSubmit = (event: FormEvent) => {
    event.preventDefault();
    setSubmitted(true);
    setFailure(null);
    if (!canSubmit) return;
    const expiryTime = form.expiryDate ? toExpiryTime(form.expiryDate) : undefined;

    if (isEdit && plan) {
      updatePlan.mutate(
        {
          // The editable fields only. null, not undefined: an omitted expiryTime means "keep",
          // so clearing the date is sent explicitly. A plan in use keeps its conditions, so they
          // are left out; the description is sent only when it changed ("" clears it).
          body: {
            displayName: trimmedName,
            expiryTime: expiryTime ?? null,
            id: plan.id,
            status: plan.status,
            ...(inUse ? {} : { conditions: conditionsCheck.body ?? [] }),
            ...(trimmedDescription !== (plan.description ?? "").trim()
              ? { description: trimmedDescription }
              : {}),
          },
          params: { orgName: orgId, planId: plan.id },
        },
        { onSuccess: leave },
      );
      return;
    }

    createPlan.mutate(
      {
        body: {
          conditions: conditionsCheck.body ?? [],
          displayName: trimmedName,
          expiryTime,
          id: slug,
          status: "ACTIVE",
          ...(trimmedDescription ? { description: trimmedDescription } : {}),
        },
        params: { orgName: orgId },
      },
      {
        // A taken handle shows on the name field (the name is the culprit); anything else as the
        // server's reason.
        onError: (error) => {
          if (subscriptionPlanErrorCode(error) === SUBSCRIPTION_PLAN_EXISTS) {
            setConflict(true);
            return;
          }
          setFailure(
            (error as { message?: string } | null)?.message || "Failed to create the plan.",
          );
        },
        onSuccess: leave,
      },
    );
  };

  const title = isEdit ? "Edit subscription plan" : "Add subscription plan";

  // Cancel and Create/Save sit at the bottom left of the form, as on the console's other create and
  // edit pages (MCP servers, LLM providers), with the same short labels.
  const actions = (
    <Stack direction="row" justifyContent="flex-start" spacing={1}>
      <Button disabled={busy} onClick={leave} variant="outlined">
        Cancel
      </Button>
      <Button
        disabled={busy || (submitted && !canSubmit)}
        startIcon={busy ? <CircularProgress color="inherit" size={16} /> : undefined}
        form={FORM_ID}
        type="submit"
        variant="contained"
      >
        {busy ? (isEdit ? "Saving…" : "Adding…") : isEdit ? "Save changes" : "Add plan"}
      </Button>
    </Stack>
  );

  if (isEdit && planQuery.error) {
    return (
      <PageLayout
        backHref={returnTo}
        backLabel="Back to Subscription Plans"
        disableIcon
        title={title}
      >
        <StateNotice
          actions={
            <Button onClick={() => navigate(listPath)} variant="outlined">
              Back to Subscription Plans
            </Button>
          }
          contained
          description="The plan could not be loaded. It may have been deleted."
          icon={SearchX}
          title="Plan not found"
          tone="error"
        />
      </PageLayout>
    );
  }

  return (
    <PageLayout
      backHref={returnTo}
      backLabel="Back to Subscription Plans"
      description={
        isEdit
          ? "Update the conditions and availability of this plan."
          : "Define the conditions and availability of this plan."
      }
      disableIcon
      isLoading={isEdit && planQuery.isLoading}
      title={title}
    >
      <Box component="form" id={FORM_ID} noValidate onSubmit={handleSubmit}>
        <Stack spacing={3}>
          <Form.Stack spacing={3}>
            {inUse && (
              <Alert severity="info">
                This plan is in use. To change its name or conditions, create a new plan.
              </Alert>
            )}
            {conflict && (
              <Alert severity="error">
                <AlertTitle sx={{ fontWeight: 600 }}>Plan not created</AlertTitle>A plan named
                &quot;{trimmedName}&quot; already exists in this organization. Choose a different
                name.
              </Alert>
            )}
            {failure && <Alert severity="error">{failure}</Alert>}

            {/* One column: name, description, conditions, expiry. */}
            <Grid container spacing={2}>
              <Grid size={{ xl: 8, xs: 12 }}>
                <TextInput
                  autoFocus={!isEdit}
                  disabled={inUse || busy}
                  error={showNameError}
                  helperText={showNameError ? nameError : undefined}
                  hideCharacterCount={charCount(form.displayName) < MAX_NAME_CHARS - 20}
                  label="Plan name"
                  required
                  maxLength={MAX_NAME_CHARS}
                  onBlur={() => setNameVisited(true)}
                  onChange={(event) => {
                    setForm((current) => ({ ...current, displayName: event.target.value }));
                    setConflict(false);
                  }}
                  placeholder="e.g. Gold"
                  value={form.displayName}
                />
              </Grid>
              <Grid size={{ xl: 8, xs: 12 }}>
                <TextInput
                  disabled={busy}
                  error={Boolean(descriptionError)}
                  helperText={descriptionError ?? undefined}
                  label="Description (optional)"
                  minRows={2}
                  multiline
                  onChange={(event) =>
                    setForm((current) => ({ ...current, description: event.target.value }))
                  }
                  placeholder="What subscribers get on this plan"
                  value={form.description}
                />
              </Grid>
              <Grid size={{ xl: 8, xs: 12 }}>
                <PlanConditionsEditor
                  check={conditionsCheck}
                  disabled={busy}
                  key={session}
                  onChange={setConditions}
                  readOnly={inUse}
                  showAllErrors={submitted}
                  value={conditions}
                />
              </Grid>
              <Grid size={{ xl: 8, xs: 12 }}>
                <FormControl error={hasExpiryError} fullWidth>
                  <FormLabel htmlFor="subscription-plan-expiry">
                    Expiry date{" "}
                    <Typography color="text.secondary" component="span" variant="inherit">
                      (optional)
                    </Typography>
                  </FormLabel>
                  <DatePickers.LocalizationProvider dateAdapter={AdapterDateFns}>
                    <DatePickers.DatePicker
                      disabled={busy}
                      disablePast
                      onChange={(date: Date | null) =>
                        setForm((current) => ({ ...current, expiryDate: date }))
                      }
                      slotProps={{
                        field: { clearable: true },
                        textField: {
                          error: hasExpiryError,
                          id: "subscription-plan-expiry",
                          size: "small",
                        },
                      }}
                      value={form.expiryDate}
                    />
                  </DatePickers.LocalizationProvider>
                  <FormHelperText sx={{ mx: 0 }}>
                    {hasExpiryError
                      ? "Enter a valid date"
                      : "The plan stops accepting subscriptions at the end of this day, in your time zone."}
                  </FormHelperText>
                </FormControl>
              </Grid>
            </Grid>
          </Form.Stack>
          {actions}
        </Stack>
      </Box>
    </PageLayout>
  );
}

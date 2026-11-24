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

import {
  useEffect,
  useMemo,
  useState,
  type MouseEvent,
  type ReactNode,
} from "react";
import {
  Alert,
  Autocomplete,
  Box,
  Button,
  Chip,
  Divider,
  Form,
  FormControlLabel,
  IconButton,
  InputAdornment,
  MenuItem,
  Radio,
  RadioGroup,
  Skeleton,
  Stack,
  TextField,
  ToggleButton,
  ToggleButtonGroup,
  Tooltip,
  Typography,
} from "@wso2/oxygen-ui";
import {
  ArrowLeft,
  ArrowRight,
  Lock,
  Plus,
  SearchX,
  ServerCrash,
  X,
} from "@wso2/oxygen-ui-icons-react";
import { useNavigate, useSearchParams } from "react-router-dom";
import {
  useCreateBudget,
  useGetBudget,
  useListBudgets,
  useListEventDestinations,
  useListLLMProviders,
  useListLLMProvidersAPIKeys,
  useListOrgAgents,
  useListProjects,
  useUpdateBudget,
} from "@agent-management-platform/api-client";
import { useUnsavedChangesGuard } from "@agent-management-platform/shared-component";
import {
  PageLayout,
  StateNotice,
  TextInput,
} from "@agent-management-platform/views";
import { INPUT_LIMITS } from "@agent-management-platform/types";
import {
  LABEL_PATTERN,
  MAX_LABELS,
  budgetFromForm,
  EMPTY_FORM,
  exemptionKinds,
  expressionPlaceholder,
  MAX_EXPRESSION_LENGTH,
  formFor,
  formFromBudget,
  handleFor,
  ANY,
  identityExpression,
  isCelKind,
  newExemption,
  newFilter,
  startFromQuery,
  duplicateNameError,
  isBudgetNotFound,
  sentFilterRows,
  serverFieldError,
  validateBudgetForm,
  type BudgetFormErrors,
  type BudgetFormState,
  type FilterRow,
  type ExemptionRow,
  type ScopeKind,
} from "../budgetForm";
import { PERIOD_LABELS, inSentence } from "./budgetLabels";

interface BudgetWizardProps {
  orgName?: string;
  /** Set to edit an existing budget. */
  budgetName?: string;
  /** The page's back link: the Budgets list, or the budget. */
  backHref: string;
  backLabel: string;
  /** After a save: the budget's name. */
  onSaved: (budgetName: string) => void;
}

/** The form's pages, in order. */
/**
 * What "Separate limit per" offers: the types of the Applies to filters set to "All"
 * (and Team, User, custom attribute), once each, in their order. A filter that picks one
 * value cannot be split, so its type is not offered.
 */
function splitChoices(filters: FilterRow[]): ScopeKind[] {
  const out: ScopeKind[] = [];
  for (const f of filters) {
    if (!out.includes(f.type) && (isCelKind(f.type) || f.id === ANY)) {
      out.push(f.type);
    }
  }
  return out;
}

const STEPS = [
  { key: "details", label: "Details" },
  { key: "scope", label: "Applies to and limit" },
  { key: "extras", label: "Exemptions, alerts and labels (optional)" },
] as const;

/** The page a field's error belongs to: the name on Details; exemptions and alerts on
 * the last; the rest (filters, limit) on Applies to and limit. */
function stepOfField(field: string): number {
  if (field === "displayName" || field === "description") return 0;
  if (
    field.startsWith("exemptions") ||
    field === "thresholds" ||
    field === "channels"
  )
    return 2;
  return 1;
}

const KIND_LABEL: Record<ScopeKind, string> = {
  llmProvider: "LLM provider",
  project: "Project",
  agent: "Agent",
  apiKey: "API key",
  team: "Team",
  user: "User",
  attribute: "Custom attribute",
};
/** What a filter or a split can be, in menu order. */
const FILTER_KINDS: ScopeKind[] = [
  "llmProvider",
  "project",
  "agent",
  "apiKey",
  "team",
  "user",
  "attribute",
];
type Option = { value: string; label: string };

/** A list the form loads (providers, projects, agents, channels), for its menus' states. */
export interface ListState {
  noun: string;
  loading?: boolean;
  failed?: boolean;
  retry?: () => void;
}

/** "Couldn't load …" with a Retry, shown where the list's options would be. */
function LoadFailed({ list }: { list: ListState }) {
  return (
    <Stack direction="row" spacing={1} alignItems="center">
      <Typography variant="body2" color="error.main">
        Couldn&apos;t load {list.noun}.
      </Typography>
      {list.retry && (
        <Button
          size="small"
          variant="text"
          // onMouseDown, so the click lands before the menu closes on blur.
          onMouseDown={(e) => {
            e.preventDefault();
            list.retry?.();
          }}
        >
          Retry
        </Button>
      )}
    </Stack>
  );
}

/** A titled section of a step; optional ones say so, locked ones show a lock. */
function Section({
  title,
  description,
  optional,
  locked,
  children,
}: {
  title: string;
  /** One line under the title, only where a choice needs it. */
  description?: string;
  optional?: boolean;
  locked?: boolean;
  children: ReactNode;
}) {
  return (
    <Box component="section">
      <Stack direction="row" spacing={1} alignItems="center">
        <Typography variant="subtitle1">
          {title}
          {optional && (
            <Typography component="span" variant="body2" color="text.secondary">
              {" "}
              (optional)
            </Typography>
          )}
        </Typography>
        {locked && (
          <Tooltip title="Fixed once the budget is created.">
            <Box sx={{ display: "flex", color: "text.secondary" }}>
              <Lock size={16} />
            </Box>
          </Tooltip>
        )}
      </Stack>
      {description && (
        <Typography variant="body2" color="text.secondary" sx={{ mt: 0.25 }}>
          {description}
        </Typography>
      )}
      <Box sx={{ mt: 2 }}>{children}</Box>
    </Box>
  );
}

/** A pill of choices; one is on. */
export function Segmented<T extends string>({
  value,
  options,
  onChange,
  disabled,
  label,
}: {
  value: T;
  options: { value: T; label: string }[];
  onChange: (v: T) => void;
  disabled?: boolean;
  label: string;
}) {
  return (
    <ToggleButtonGroup
      exclusive
      fullWidth
      size="small"
      value={value}
      onChange={(_, v: T | null) => v && onChange(v)}
      disabled={disabled}
      aria-label={label}
      sx={{ "& .MuiToggleButton-root": { textTransform: "none", px: 2 } }}
    >
      {options.map((o) => (
        <ToggleButton key={o.value} value={o.value}>
          {o.label}
        </ToggleButton>
      ))}
    </ToggleButtonGroup>
  );
}

/** Two or more choices as radio buttons in a row. */
function ChoiceField<T extends string>({
  label,
  value,
  options,
  onChange,
  disabled,
}: {
  label: string;
  value: T;
  options: { value: T; label: string; description?: string }[];
  onChange: (v: T) => void;
  disabled?: boolean;
}) {
  return (
    <Form.ElementWrapper label={label} name={label}>
      <RadioGroup
        value={value}
        onChange={(_, v) => onChange(v as T)}
        aria-label={label}
        // Options sit side by side as boxes; stacked on narrow screens.
        sx={{
          display: "grid",
          gridTemplateColumns: {
            xs: "1fr",
            md: `repeat(${options.length}, 1fr)`,
          },
          gap: 2,
        }}
      >
        {options.map((o) => (
          <FormControlLabel
            key={o.value}
            value={o.value}
            control={<Radio size="small" />}
            disabled={disabled}
            sx={{
              alignItems: o.description ? "flex-start" : "center",
              m: 0,
              p: 1.5,
              pl: 1,
              border: 1,
              borderRadius: 2,
              borderColor: o.value === value ? "primary.main" : "divider",
              bgcolor: "background.paper",
            }}
            label={
              o.description ? (
                <Box sx={{ pt: 0.75 }}>
                  <Typography variant="body2" fontWeight={500}>
                    {o.label}
                  </Typography>
                  <Typography variant="caption" color="text.secondary">
                    {o.description}
                  </Typography>
                </Box>
              ) : (
                <Typography variant="body2" fontWeight={500}>
                  {o.label}
                </Typography>
              )
            }
          />
        ))}
      </RadioGroup>
    </Form.ElementWrapper>
  );
}

/** Picks a project, an agent or an LLM provider from a list, or names an API key. */
function ValuePicker({
  kind,
  value,
  onChange,
  options,
  disabled,
  error,
  placeholder,
  list,
}: {
  kind: ScopeKind;
  value: string;
  onChange: (v: string) => void;
  options: Option[];
  disabled?: boolean;
  error?: string;
  placeholder: string;
  list?: ListState;
}) {
  const empty =
    kind === "apiKey"
      ? "No API keys yet."
      : kind === "agent"
        ? "No agents yet."
        : kind === "llmProvider"
          ? "No LLM providers yet."
          : "No projects yet. Add one from Projects.";
  // A saved key no longer listed (deleted, or an agent's own key) stays selectable.
  if (kind === "apiKey" && value && !options.some((o) => o.value === value)) {
    options = [...options, { value, label: value }];
  }
  const selected = options.find((o) => o.value === value) ?? null;
  return (
    <Autocomplete
      size="small"
      fullWidth
      options={options}
      value={selected}
      onChange={(_, o) => onChange(o?.value ?? "")}
      getOptionLabel={(o) => o.label}
      isOptionEqualToValue={(a, b) => a.value === b.value}
      disabled={disabled}
      loading={list?.loading}
      loadingText={`Loading ${list?.noun ?? "options"}…`}
      noOptionsText={list?.failed ? <LoadFailed list={list} /> : empty}
      renderInput={(params) => (
        <TextField
          {...params}
          placeholder={placeholder}
          error={!!error}
          helperText={error}
        />
      )}
    />
  );
}
/** A rounded "+ Kind" button that adds a condition, a warning or a channel. */
export function AddChip({
  label,
  onClick,
  disabled,
}: {
  label: string;
  onClick: (e: MouseEvent<HTMLElement>) => void;
  disabled?: boolean;
}) {
  return (
    <Button
      variant="text"
      size="small"
      startIcon={<Plus size={14} />}
      onClick={onClick}
      disabled={disabled}
      sx={{ textTransform: "none", px: 1, ml: -1, typography: "body2" }}
    >
      {label}
    </Button>
  );
}

/** Warning thresholds: a labelled box of percentages; type one and press Enter. */
export function Thresholds({
  value,
  onChange,
  error,
}: {
  value: number[];
  onChange: (v: number[]) => void;
  error?: string;
}) {
  const [draft, setDraft] = useState("");
  const [draftError, setDraftError] = useState("");
  // Each entry is checked as it is added; the reason shows under the box.
  const accept = (entries: string[]) => {
    const next: number[] = [];
    for (const e of entries) {
      const n = Number.parseInt(String(e).replace(/[^\d]/g, ""), 10);
      if (!Number.isFinite(n) || n < 1 || n > 1000) {
        setDraftError("Enter a percentage from 1 to 1000.");
        return;
      }
      next.push(n);
    }
    setDraftError("");
    setDraft("");
    onChange([...new Set(next)].sort((x, y) => x - y));
  };
  return (
    <Form.ElementWrapper label="Warn at" name="Warn at">
      <Autocomplete
        multiple
        freeSolo
        size="small"
        autoHighlight
        // No preset list (any whole percentage is allowed): while typing, the typed
        // value is offered as "Add 122%".
        options={[] as string[]}
        filterOptions={() =>
          draft && !value.includes(Number(draft)) ? [draft] : []
        }
        getOptionLabel={(o) => `${o}%`}
        renderOption={(props, o) => {
          const { key, ...rest } = props as typeof props & { key: string };
          return (
            <li key={key} {...rest}>
              {`Add ${o}%`}
            </li>
          );
        }}
        value={value.map(String)}
        inputValue={draft}
        onInputChange={(_, v, reason) => {
          if (reason === "input") {
            setDraft(v.replace(/[^\d]/g, ""));
            setDraftError("");
          }
        }}
        onChange={(_, v) => accept(v as string[])}
        renderTags={(tags, getTagProps) =>
          tags.map((t, i) => {
            const { key, ...props } = getTagProps({ index: i });
            return <Chip key={key} size="small" label={`${t}%`} {...props} />;
          })
        }
        renderInput={(params) => (
          <TextField
            {...params}
            placeholder={value.length ? "" : "e.g. 80"}
            error={!!(draftError || error)}
            helperText={
              draftError || error || "Type a percentage and press Enter."
            }
            slotProps={{
              htmlInput: { ...params.inputProps, "aria-label": "Warn at" },
            }}
          />
        )}
      />
    </Form.ElementWrapper>
  );
}

/** Budget labels as chips: type a word and press Enter, as the warning percentages are. */
export function Labels({
  value,
  onChange,
  error,
}: {
  value: string[];
  onChange: (v: string[]) => void;
  error?: string;
}) {
  const [draft, setDraft] = useState("");
  const [draftError, setDraftError] = useState("");
  // Each label is checked as it is added; the reason shows under the box.
  const accept = (entries: string[]) => {
    const next: string[] = [];
    for (const e of entries) {
      const word = String(e).trim();
      if (!LABEL_PATTERN.test(word)) {
        setDraftError("Use one word: letters, digits, '.', '_' or '-'.");
        return;
      }
      if (!next.includes(word)) next.push(word);
    }
    if (next.length > MAX_LABELS) {
      setDraftError(`Add at most ${MAX_LABELS} labels.`);
      return;
    }
    setDraftError("");
    setDraft("");
    onChange(next);
  };
  return (
    <Autocomplete
      multiple
      freeSolo
      size="small"
      autoHighlight
      // Any word is allowed: while typing, the typed word is offered as "Add finance".
      options={[] as string[]}
      filterOptions={() => (draft && !value.includes(draft) ? [draft] : [])}
      renderOption={(props, o) => {
        const { key, ...rest } = props as typeof props & { key: string };
        return (
          <li key={key} {...rest}>
            {`Add ${o}`}
          </li>
        );
      }}
      value={value}
      inputValue={draft}
      onInputChange={(_, v, reason) => {
        if (reason === "input") {
          setDraft(v.replace(/\s/g, ""));
          setDraftError("");
        }
      }}
      onChange={(_, v) => accept(v as string[])}
      renderTags={(tags, getTagProps) =>
        tags.map((t, i) => {
          const { key, ...props } = getTagProps({ index: i });
          return <Chip key={key} size="small" label={t} {...props} />;
        })
      }
      renderInput={(params) => (
        <TextField
          {...params}
          placeholder={value.length ? "" : "e.g. finance"}
          error={!!(draftError || error)}
          helperText={draftError || error || "Type a label and press Enter."}
          slotProps={{
            htmlInput: { ...params.inputProps, "aria-label": "Labels" },
          }}
        />
      )}
    />
  );
}

/** The organization's event destinations, picked in a labelled box. */
export function Channels({
  value,
  onChange,
  options,
  error,
  list,
}: {
  value: string[];
  onChange: (v: string[]) => void;
  options: string[];
  error?: string;
  list?: ListState;
}) {
  return (
    <Form.ElementWrapper
      label="Notification channels"
      name="Notification channels"
    >
      <Autocomplete
        multiple
        size="small"
        options={options}
        value={value}
        onChange={(_, v) => onChange(v)}
        loading={list?.loading}
        loadingText="Loading notification channels…"
        noOptionsText={
          list?.failed ? (
            <LoadFailed list={list} />
          ) : options.length ? (
            "Every channel is added"
          ) : (
            "No event destinations yet"
          )
        }
        renderTags={(tags, getTagProps) =>
          tags.map((t, i) => {
            const { key, ...props } = getTagProps({ index: i });
            return <Chip key={key} size="small" label={t} {...props} />;
          })
        }
        renderInput={(params) => (
          <TextField
            {...params}
            placeholder={value.length ? "" : "Choose channels"}
            error={!!error}
            helperText={error}
            slotProps={{
              htmlInput: {
                ...params.inputProps,
                "aria-label": "Notification channels",
              },
            }}
          />
        )}
      />
    </Form.ElementWrapper>
  );
}

/** Create or edit a budget: three parts, six numbered cards (UX 3-4). */
/**
 * Create or edit a budget, one step at a time, as the console's other wizards do (the
 * evaluation Monitor wizard): a page with a back link, Previous and Next under the
 * content.
 */
export function BudgetWizard({
  orgName,
  budgetName,
  backHref,
  backLabel,
  onSaved,
}: BudgetWizardProps) {
  const navigate = useNavigate();
  const editing = !!budgetName;
  const existing = useGetBudget({ orgName, budgetName });
  // A budget created on a gateway: only its name, description and warnings change here.
  const fromGateway = existing.data?.status?.origin === "gatewayApi";
  const budgets = useListBudgets({ orgName }, { limit: 100 });
  const providers = useListLLMProviders({ orgName });
  const projects = useListProjects({ orgName }, { limit: 100 });
  const orgAgents = useListOrgAgents({ orgName });
  const destinations = useListEventDestinations(orgName);
  const create = useCreateBudget();
  const update = useUpdateBudget();

  // "Advanced options" on an entity's page opens this form with the entity filled in.
  // Those values are where the form starts, not changes, so they don't make it dirty.
  const [searchParams] = useSearchParams();
  const [start] = useState(() =>
    editing ? EMPTY_FORM : formFor(startFromQuery(searchParams)),
  );
  const [form, setForm] = useState<BudgetFormState>(start);
  const [initial, setInitial] = useState<BudgetFormState>(start);
  useEffect(() => {
    if (existing.data) {
      const loaded = formFromBudget(existing.data);
      setForm(loaded);
      setInitial(loaded);
    }
  }, [existing.data]);

  // API keys of the providers the budget covers: the one filtered on, or all of them.
  const providerFilter = form.filters.find((f) => f.type === "llmProvider")?.id;
  const scopedProviders = (providers.data?.providers ?? []).filter(
    (p) => !providerFilter || p.id === providerFilter,
  );
  const providerKeys = useListLLMProvidersAPIKeys(
    orgName,
    scopedProviders.map((p) => p.uuid),
  );

  // The pages: an edit can open any of them; a new budget goes forward with Next.
  const [step, setStep] = useState(0);
  // Pages whose errors are shown: those Next or Save has checked.
  const [checked, setChecked] = useState<Set<number>>(new Set());

  const isDirty = JSON.stringify(form) !== JSON.stringify(initial);
  const { allowNavigation } = useUnsavedChangesGuard(isDirty);
  // A check that failed brings the first field in error into view and focuses it.
  const [failedChecks, setFailedChecks] = useState(0);
  useEffect(() => {
    if (!failedChecks) return;
    const first = document.querySelector<HTMLElement>('[aria-invalid="true"]');
    first?.scrollIntoView({ behavior: "smooth", block: "center" });
    first?.focus({ preventScroll: true });
  }, [failedChecks, step]);
  // A failed save's message sits next to the field the server named, until the next change.
  const [serverErrors, setServerErrors] = useState<BudgetFormErrors>({});
  const duplicateName = duplicateNameError(
    form.displayName,
    budgets.data?.budgets ?? [],
    budgetName,
  );
  const allErrors: BudgetFormErrors = {
    ...validateBudgetForm(form),
    ...(duplicateName ? { displayName: duplicateName } : {}),
  };
  const errorsOn = (n: number) =>
    Object.keys(allErrors).filter((k) => stepOfField(k) === n);
  const errors: BudgetFormErrors = {
    ...Object.fromEntries(
      Object.entries(allErrors).filter(([k]) => checked.has(stepOfField(k))),
    ),
    ...serverErrors,
  };
  const patch = (changes: Partial<BudgetFormState>) => {
    setServerErrors({});
    setForm((f) => ({ ...f, ...changes }));
  };
  const set = <K extends keyof BudgetFormState>(
    key: K,
    value: BudgetFormState[K],
  ) => patch({ [key]: value } as Partial<BudgetFormState>);
  // Changing what the budget counts clears the exemptions: an exemption must stay
  // narrower than the filters and the split, and re-adding it is simpler than ruling
  // types out.
  // The limit is split by one of the filters; it goes back to one shared limit when
  // that filter is removed or picks one value.
  const setFilters = (filters: FilterRow[]) => {
    const keepsSplit =
      !!form.splitBy && splitChoices(filters).includes(form.splitBy);
    patch({
      filters,
      exemptions: [],
      ...(keepsSplit ? {} : { splitBy: "", splitExpression: "" }),
    });
  };
  const setSplit = (splitBy: ScopeKind | "") =>
    patch({
      splitBy,
      splitExpression: splitBy ? identityExpression(splitBy) : "",
      exemptions: [],
    });

  const providerOptions: Option[] = (providers.data?.providers ?? []).map(
    (p) => ({ value: p.id, label: p.name }),
  );
  // API keys by the name gateways see, shown as "Mock OpenAI / team-alpha-key"
  // (owner first, as agents are shown "Project / Agent").
  const apiKeyOptions: Option[] = providerKeys.keys.map(
    ({ providerId, key }) => ({
      value: key.name,
      label: `${
        scopedProviders.find((p) => p.uuid === providerId)?.name ??
        "LLM provider"
      } / ${key.displayName || key.name}`,
    }),
  );
  const apiKeyList: ListState = {
    noun: "API keys",
    loading: providerKeys.isLoading,
  };
  const projectOptions: Option[] = (projects.data?.projects ?? []).map((p) => ({
    value: p.name,
    label: p.displayName || p.name,
  }));
  // Agents keyed project/agent: names are unique only within a project.
  const agentOptions: Option[] = (orgAgents.data?.agents ?? []).map((a) => ({
    value: `${a.projectName}/${a.name}`,
    label: `${a.projectDisplayName || a.projectName} / ${a.displayName || a.name}`,
  }));
  const channelOptions = (destinations.data?.destinations ?? []).map(
    (d) => d.name,
  );
  const providerList: ListState = {
    noun: "LLM providers",
    loading: providers.isLoading,
    failed: !!providers.error,
    retry: () => void providers.refetch(),
  };
  const projectList: ListState = {
    noun: "projects",
    loading: projects.isLoading,
    failed: !!projects.error,
    retry: () => void projects.refetch(),
  };
  const agentList: ListState = {
    noun: "agents",
    loading: orgAgents.isLoading,
    failed: !!orgAgents.error,
    retry: () => void orgAgents.refetch(),
  };
  const channelList: ListState = {
    noun: "notification channels",
    loading: destinations.isLoading,
    failed: !!destinations.error,
    retry: () => void destinations.refetch(),
  };
  const allowedExemptionKinds = exemptionKinds(form);
  const taken = useMemo(
    () => (budgets.data?.budgets ?? []).map((b) => b.metadata.name),
    [budgets.data],
  );
  const handle = handleFor(form.displayName, taken);

  const updateFilter = (i: number, change: Partial<FilterRow>) =>
    setFilters(form.filters.map((f, j) => (j === i ? { ...f, ...change } : f)));
  // A type another filter already has (Team, User and custom attributes may repeat).
  const filterTypeTaken = (i: number, k: ScopeKind) =>
    !isCelKind(k) && form.filters.some((f, j) => j !== i && f.type === k);
  // The type a new filter starts with: the first one still free.
  const firstFreeKind = FILTER_KINDS.find((k) => !filterTypeTaken(-1, k));
  const updateExemption = (i: number, change: Partial<ExemptionRow>) =>
    set(
      "exemptions",
      form.exemptions.map((e, j) => (j === i ? { ...e, ...change } : e)),
    );
  const agentValue = (r: { id: string; project: string }) =>
    r.id === ANY ? ANY : r.id ? `${r.project}/${r.id}` : "";
  const splitAgent = (v: string) => {
    if (v === ANY) return { project: "", id: ANY };
    const [project = "", id = ""] = v.split("/");
    return { project, id };
  };
  // "All LLM providers", "All projects", ...: the type without one value, first in the
  // list (Kong's "is set").
  const anyOption = (kind: ScopeKind): Option[] =>
    isCelKind(kind)
      ? []
      : [
          {
            value: ANY,
            label:
              kind === "llmProvider"
                ? "All LLM providers"
                : kind === "apiKey"
                  ? "All API keys"
                  : `All ${inSentence(KIND_LABEL[kind])}s`,
          },
        ];
  const pickerPlaceholder = (kind: ScopeKind) =>
    kind === "apiKey"
      ? "Choose an API key"
      : kind === "llmProvider"
        ? "Choose an LLM provider"
        : kind === "agent"
          ? "Choose an agent"
          : kind === "project"
            ? "Choose a project"
            : "Choose one";

  const check = (pages: number[]) => {
    setChecked((c) => new Set([...c, ...pages]));
    const failing = pages.filter((n) => errorsOn(n).length > 0);
    if (failing.length) {
      setStep(failing[0]);
      setFailedChecks((n) => n + 1);
    }
    return failing.length === 0;
  };
  const goTo = (n: number) => {
    setStep(n);
    window.scrollTo({ top: 0, behavior: "smooth" });
  };
  const next = () => {
    if (check([step])) goTo(step + 1);
  };

  const onSaveError = (err: unknown) => {
    const { field, message } = serverFieldError(
      err,
      sentFilterRows(form.filters),
    );
    setServerErrors({ [field]: message });
    setStep(stepOfField(field));
    setFailedChecks((n) => n + 1);
  };

  const save = () => {
    if (!check(STEPS.map((_, i) => i))) return;
    const body = budgetFromForm(form, {
      name: handle,
      existing: existing.data,
    });
    const done = (name: string) => allowNavigation(() => onSaved(name));
    if (editing) {
      update.mutate(
        { params: { orgName, budgetName }, body },
        {
          onSuccess: () => done(body.metadata.name),
          onError: onSaveError,
        },
      );
    } else {
      create.mutate(
        { params: { orgName }, body },
        {
          onSuccess: (b) => done(b.metadata.name),
          onError: onSaveError,
        },
      );
    }
  };

  // The add-condition menu's anchor.

  const title = editing
    ? `Edit ${initial.displayName || budgetName}`
    : "Create budget";
  const saving = create.isPending || update.isPending;
  // The page, as the console's other wizards: a back link, the step in the description.
  const shell = (body: ReactNode, actions?: ReactNode) => (
    <PageLayout
      title={title}
      description={
        actions && !editing
          ? `Step ${step + 1} of ${STEPS.length} · ${STEPS[step].label}`
          : undefined
      }
      disableIcon
      backHref={backHref}
      backLabel={backLabel}
    >
      <Stack spacing={3} sx={{ maxWidth: 960 }}>
        {body}
        {actions}
      </Stack>
    </PageLayout>
  );
  if (editing && existing.error) {
    const notFound = isBudgetNotFound(existing.error);
    return shell(
      notFound ? (
        <StateNotice
          icon={SearchX}
          title={`There is no budget named ${budgetName}`}
          description="It may have been deleted, or the link is wrong."
        />
      ) : (
        <StateNotice
          icon={ServerCrash}
          tone="error"
          title="Couldn't load the budget"
          description="The server didn't answer. Try again in a moment."
          actions={
            <Button variant="outlined" onClick={() => void existing.refetch()}>
              Retry
            </Button>
          }
        />
      ),
    );
  }
  if (editing && existing.isLoading) {
    return shell(<Skeleton variant="rounded" height={320} />);
  }

  // One row per filter: its type, then the one it matches (or a CEL expression).
  const filterRows = form.filters.map((f, i) => (
    <Stack
      key={i}
      direction={{ xs: "column", md: "row" }}
      spacing={1.5}
      alignItems={{ md: "flex-start" }}
    >
      <Box sx={{ width: { md: 220 }, flexShrink: 0 }}>
        <TextInput
          select
          size="small"
          fullWidth
          value={f.type}
          disabled={editing}
          aria-label={`Filter ${i + 1} type`}
          onChange={(ev) =>
            setFilters(
              form.filters.map((x, j) =>
                j === i ? newFilter(ev.target.value as ScopeKind) : x,
              ),
            )
          }
        >
          {FILTER_KINDS.map((k) => (
            <MenuItem
              key={k}
              value={k}
              disabled={k !== f.type && filterTypeTaken(i, k)}
            >
              {KIND_LABEL[k]}
            </MenuItem>
          ))}
        </TextInput>
      </Box>
      <Box sx={{ flex: 1, minWidth: 0 }}>
        {isCelKind(f.type) ? (
          <TextInput
            size="small"
            fullWidth
            value={f.expression}
            disabled={editing}
            onChange={(e) => updateFilter(i, { expression: e.target.value })}
            placeholder={expressionPlaceholder(f.type)}
            error={!!errors[`filters.${i}`]}
            helperText={
              errors[`filters.${i}`] ??
              (form.splitBy === f.type
                ? `Optional. Empty counts all ${inSentence(KIND_LABEL[f.type])}s.`
                : "CEL expression for requests to count.")
            }
            maxLength={MAX_EXPRESSION_LENGTH}
            hideCharacterCount
            sx={{ "& input": { fontFamily: "monospace" } }}
          />
        ) : (
          <ValuePicker
            kind={f.type}
            value={f.type === "agent" ? agentValue(f) : f.id}
            onChange={(v) =>
              updateFilter(i, f.type === "agent" ? splitAgent(v) : { id: v })
            }
            options={[
              ...anyOption(f.type),
              ...(f.type === "llmProvider"
                ? providerOptions
                : f.type === "agent"
                  ? agentOptions
                  : f.type === "apiKey"
                    ? apiKeyOptions
                    : projectOptions),
            ]}
            list={
              f.type === "llmProvider"
                ? providerList
                : f.type === "agent"
                  ? agentList
                  : f.type === "apiKey"
                    ? apiKeyList
                    : projectList
            }
            disabled={editing}
            error={errors[`filters.${i}`]}
            placeholder={pickerPlaceholder(f.type)}
          />
        )}
      </Box>
      {!editing && (
        <IconButton
          aria-label={`Remove filter ${i + 1}`}
          onClick={() => setFilters(form.filters.filter((_, j) => j !== i))}
          sx={{ alignSelf: { xs: "flex-end", md: "flex-start" } }}
        >
          <X size={16} />
        </IconButton>
      )}
    </Stack>
  ));

  const fieldWidth = {};

  const details = (
    <Section title="General details">
      <Stack spacing={3} sx={fieldWidth}>
        <TextInput
          label="Name"
          size="small"
          fullWidth
          value={form.displayName}
          onChange={(e) => set("displayName", e.target.value)}
          placeholder="e.g. Payments on OpenAI"
          error={!!errors.displayName}
          helperText={errors.displayName}
          maxLength={INPUT_LIMITS.NAME}
          hideCharacterCount
        />
        <TextInput
          label="Description (optional)"
          fullWidth
          multiline
          minRows={3}
          value={form.description}
          onChange={(e) => set("description", e.target.value)}
          placeholder="e.g. Payments team's OpenAI spend"
          error={!!errors.description}
          helperText={errors.description}
          maxLength={INPUT_LIMITS.DESCRIPTION}
          hideCharacterCount
        />
      </Stack>
    </Section>
  );

  const appliesTo = (
    <Section
      title="Applies to"
      description="What this budget counts."
      locked={editing}
    >
      {errors.filters && (
        <Alert severity="error" sx={{ mb: 2 }}>
          {errors.filters}
        </Alert>
      )}
      <Stack spacing={1.5}>
        {filterRows.length ? (
          filterRows.map((row, i) => (
            <Box key={i}>
              {i > 0 && (
                <Typography
                  variant="caption"
                  fontWeight={600}
                  color="primary.main"
                  component="div"
                  sx={{ mb: 1.5 }}
                >
                  AND
                </Typography>
              )}
              {row}
            </Box>
          ))
        ) : (
          <Typography variant="body2" color="text.secondary">
            All LLM usage in the organization.
          </Typography>
        )}
      </Stack>
      {!editing && (
        <Box sx={{ mt: 2 }}>
          <AddChip
            label="Add filter"
            disabled={!firstFreeKind}
            onClick={() =>
              firstFreeKind &&
              setFilters([...form.filters, newFilter(firstFreeKind)])
            }
          />
        </Box>
      )}
    </Section>
  );

  const exemptions = (
    <Section
      title="Exemptions"
      description="Traffic this budget skips."
      optional
      locked={fromGateway}
    >
      {errors.exemptions && (
        <Alert severity="error" sx={{ mb: 2 }}>
          {errors.exemptions}
        </Alert>
      )}
      <Stack spacing={1.5}>
        {/* One line per exemption, as the Applies to filters: type, value, remove. */}
        {form.exemptions.map((e, i) => (
          <Stack
            key={i}
            direction={{ xs: "column", md: "row" }}
            spacing={1.5}
            alignItems={{ md: "flex-start" }}
          >
            <Box sx={{ width: { md: 220 }, flexShrink: 0 }}>
              <TextInput
                select
                size="small"
                fullWidth
                value={e.type}
                disabled={fromGateway}
                aria-label={`Exemption ${i + 1} type`}
                onChange={(ev) =>
                  updateExemption(
                    i,
                    newExemption(ev.target.value as ExemptionRow["type"]),
                  )
                }
              >
                {[...new Set([e.type, ...allowedExemptionKinds])].map((k) => (
                  <MenuItem key={k} value={k}>
                    {KIND_LABEL[k]}
                  </MenuItem>
                ))}
              </TextInput>
            </Box>
            <Box sx={{ flex: 1, minWidth: 0 }}>
              {isCelKind(e.type) ? (
                <TextInput
                  size="small"
                  fullWidth
                  value={e.expression}
                  disabled={fromGateway}
                  onChange={(ev) =>
                    updateExemption(i, { expression: ev.target.value })
                  }
                  placeholder={expressionPlaceholder(e.type)}
                  error={!!errors[`exemptions.${i}`]}
                  helperText={
                    errors[`exemptions.${i}`] ??
                    "CEL expression for requests to skip."
                  }
                  maxLength={MAX_EXPRESSION_LENGTH}
                  hideCharacterCount
                  sx={{ "& input": { fontFamily: "monospace" } }}
                />
              ) : (
                <ValuePicker
                  kind={e.type}
                  value={e.type === "agent" ? agentValue(e) : e.id}
                  onChange={(v) =>
                    updateExemption(
                      i,
                      e.type === "agent" ? splitAgent(v) : { id: v },
                    )
                  }
                  options={e.type === "agent" ? agentOptions : projectOptions}
                  list={e.type === "agent" ? agentList : projectList}
                  error={errors[`exemptions.${i}`]}
                  placeholder={pickerPlaceholder(e.type)}
                  disabled={fromGateway}
                />
              )}
            </Box>
            {!fromGateway && (
              <IconButton
                aria-label={`Remove exemption ${i + 1}`}
                onClick={() =>
                  set(
                    "exemptions",
                    form.exemptions.filter((_, j) => j !== i),
                  )
                }
                sx={{ alignSelf: { xs: "flex-end", md: "flex-start" } }}
              >
                <X size={16} />
              </IconButton>
            )}
          </Stack>
        ))}
        {!fromGateway && (
          <Box>
            <AddChip
              label="Add exemption"
              disabled={!allowedExemptionKinds.length}
              onClick={() =>
                set("exemptions", [
                  ...form.exemptions,
                  newExemption(allowedExemptionKinds[0]),
                ])
              }
            />
          </Box>
        )}
      </Stack>
    </Section>
  );

  // The split's own error; Team, User and custom attribute show theirs on the expression.
  const splitPickError =
    form.splitBy && !isCelKind(form.splitBy) ? errors.splitBy : undefined;
  const spendingLimit = (
    <Section
      title="Limit"
      description="How the amount is shared."
      locked={fromGateway}
    >
      <Stack spacing={3} sx={fieldWidth}>
        {/* The split and, for Team, User or a custom attribute, its expression on one line. */}
        <Stack
          direction={{ xs: "column", md: "row" }}
          spacing={2}
          alignItems={{ md: "flex-start" }}
        >
          <Box sx={{ flex: 1, minWidth: 0 }}>
            <TextInput
              label="Separate limit per"
              select
              size="small"
              fullWidth
              value={form.splitBy || "none"}
              disabled={editing}
              onChange={(e) =>
                setSplit(
                  e.target.value === "none"
                    ? ""
                    : (e.target.value as ScopeKind),
                )
              }
              error={!!splitPickError}
              helperText={
                splitPickError ||
                (form.splitBy
                  ? `Each ${inSentence(KIND_LABEL[form.splitBy])} gets its own ${Number(form.amount) > 0 ? `$${Number(form.amount).toFixed(2)} ` : ""}limit.`
                  : "All matching usage shares one limit.")
              }
            >
              <MenuItem value="none">None (one shared limit)</MenuItem>
              {splitChoices(form.filters).map((k) => (
                <MenuItem key={k} value={k}>
                  {KIND_LABEL[k]}
                </MenuItem>
              ))}
            </TextInput>
          </Box>
          {form.splitBy && isCelKind(form.splitBy) && (
            <Box sx={{ flex: 1, minWidth: 0 }}>
              <TextInput
                label="Identity expression"
                size="small"
                fullWidth
                value={form.splitExpression}
                disabled={editing}
                onChange={(e) => set("splitExpression", e.target.value)}
                placeholder='e.g. request.Headers["x-cost-center"]'
                error={!!errors.splitBy}
                helperText={
                  errors.splitBy ??
                  `CEL value that identifies each ${inSentence(KIND_LABEL[form.splitBy])}.`
                }
                maxLength={MAX_EXPRESSION_LENGTH}
                hideCharacterCount
                sx={{ "& input": { fontFamily: "monospace" } }}
              />
            </Box>
          )}
        </Stack>
        {/* Amount and its reset period on one line. */}
        <Stack
          direction={{ xs: "column", md: "row" }}
          spacing={2}
          alignItems={{ md: "flex-start" }}
        >
          <Box sx={{ flex: 1 }}>
            <TextInput
              label="Amount"
              size="small"
              fullWidth
              value={form.amount}
              disabled={fromGateway}
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
          </Box>
          <Box sx={{ flex: 1 }}>
            <Form.ElementWrapper label="Reset period" name="Reset period">
              <Segmented
                label="Reset period"
                value={form.period}
                disabled={editing}
                onChange={(v) => set("period", v)}
                options={(["day", "week", "month"] as const).map((p) => ({
                  value: p,
                  label: PERIOD_LABELS[p],
                }))}
              />
              <Typography
                variant="caption"
                color="text.secondary"
                component="div"
                sx={{ mt: 1 }}
              >
                {editing ? "Fixed once created." : "Periods use UTC."}
              </Typography>
            </Form.ElementWrapper>
          </Box>
        </Stack>
        <ChoiceField
          label="Enforcement"
          value={form.action}
          disabled={fromGateway}
          onChange={(v) => set("action", v)}
          options={[
            {
              value: "block",
              label: "Block requests",
              description: "Requests are refused at the limit.",
            },
            {
              value: "allow",
              label: "Allow requests",
              description: "Only warns.",
            },
          ]}
        />
        {form.action === "block" && (
          <TextInput
            label="Block at"
            size="small"
            fullWidth
            value={form.blockAt}
            disabled={fromGateway}
            onChange={(e) => set("blockAt", e.target.value)}
            error={!!errors.blockAt}
            helperText={
              errors.blockAt ?? "Refuse requests at this % of the amount."
            }
            inputMode="numeric"
            slotProps={{
              input: {
                endAdornment: <InputAdornment position="end">%</InputAdornment>,
              },
            }}
          />
        )}
      </Stack>
    </Section>
  );

  const alerts = (
    <Section
      title="Alerts"
      description="Notify at these % of the limit."
      optional
    >
      <Stack spacing={2} sx={fieldWidth}>
        <Thresholds
          value={form.thresholds}
          onChange={(v) => set("thresholds", v)}
          error={errors.thresholds}
        />
        <Channels
          value={form.channels}
          onChange={(v) => set("channels", v)}
          options={channelOptions}
          list={channelList}
          error={errors.channels}
        />
      </Stack>
    </Section>
  );

  // Labels as chips, added like the warning percentages: type a word, press Enter.
  const labelsSection = (
    <Section title="Labels" description="Labels to organize budgets" optional>
      <Labels
        value={form.labels}
        onChange={(labels) => set("labels", labels)}
        error={errors.labels}
      />
    </Section>
  );

  // Details, then what the budget covers and its limit; exemptions, alerts and labels are
  // optional.
  const pages = [
    details,
    <Stack key="scope" spacing={4} divider={<Divider flexItem />}>
      {appliesTo}
      {spendingLimit}
    </Stack>,
    <Stack key="extras" spacing={4} divider={<Divider flexItem />}>
      {exemptions}
      {alerts}
      {labelsSection}
    </Stack>,
  ];
  const last = step === STEPS.length - 1;

  // Edit shows every section on one page; create walks through them one step at a time.
  return shell(
    <Stack spacing={editing ? 5 : 3}>
      {fromGateway && (
        <Alert severity="info">
          Gateway budget: only name, description and warnings change here.
        </Alert>
      )}
      {editing
        ? pages.map((page, i) => <Box key={i}>{page}</Box>)
        : pages[step]}
      {errors.form && <Alert severity="error">{errors.form}</Alert>}
    </Stack>,
    editing ? (
      <Stack direction="row" gap={2}>
        <Button
          variant="contained"
          color="primary"
          onClick={save}
          disabled={saving}
        >
          Save changes
        </Button>
        <Button
          variant="outlined"
          color="primary"
          onClick={() => allowNavigation(() => navigate(backHref))}
          disabled={saving}
        >
          Cancel
        </Button>
      </Stack>
    ) : (
      <Stack direction="row" gap={2}>
        {step > 0 && (
          <Button
            variant="outlined"
            color="primary"
            startIcon={<ArrowLeft size={20} />}
            onClick={() => goTo(step - 1)}
            disabled={saving}
          >
            Previous
          </Button>
        )}
        {!last && (
          <Button
            variant="contained"
            color="primary"
            endIcon={<ArrowRight size={20} />}
            onClick={next}
            disabled={saving}
          >
            Next
          </Button>
        )}
        {last && (
          <Button
            variant="contained"
            color="primary"
            onClick={save}
            disabled={saving}
          >
            Create budget
          </Button>
        )}
      </Stack>
    ),
  );
}

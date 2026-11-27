/*
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

import { useId, useState, type ReactNode } from "react";
import {
  Box,
  Button,
  ButtonBase,
  Chip,
  Collapse,
  Form,
  FormControl,
  FormHelperText,
  FormLabel,
  IconButton,
  ListItemIcon,
  ListItemText,
  Menu,
  MenuItem,
  OutlinedInput,
  Select,
  Stack,
  Switch,
  Tooltip,
  Typography,
  type Theme,
} from "@wso2/oxygen-ui";
import {
  Activity,
  ChevronRight,
  Coins,
  Info,
  Infinity as InfinityIcon,
  Plus,
  Puzzle,
  Trash2,
  Wrench,
  X,
} from "@wso2/oxygen-ui-icons-react";

/** A one-pixel border in the divider colour. */
const hairline = (theme: Theme) => `1px solid ${theme.palette.divider}`;

/** A small square tile behind an icon. */
const iconTileSx = (theme: Theme, size: number) => ({
  alignItems: "center",
  bgcolor: "action.hover",
  border: hairline(theme),
  borderRadius: "8px",
  color: "text.secondary",
  display: "flex",
  flexShrink: 0,
  height: size,
  justifyContent: "center",
  width: size,
});
import {
  BUILT_IN_TYPES,
  fieldKeys,
  MAX_CONDITIONS,
  MAX_CUSTOM_ID_CHARS,
  MAX_WINDOWS,
  newCondition,
  newPair,
  newWindow,
  TIME_UNITS,
  TOKEN_TYPES,
  type ConditionDraft,
  type ConditionType,
  type ConditionsCheck,
  type FieldError,
  type OnExceed,
  type TimeUnit,
  type TokenType,
  type WindowDraft,
} from "./planConditions";

/** The editor's text, in the console's language (it has no translations). */
const messages = {
  typeAdded: "Added",
  addCondition: "Add condition",
  addLimit: "Add limit",
  addValue: "Add value",
  collapse: (name: string) => `Hide ${name} settings`,
  conditionsHelper:
    "A plan with no conditions is unlimited. A condition takes effect on an MCP server only where the policy that enforces it is attached.",
  conditionsLabel: "Conditions",
  conditionTypes: "Condition types",
  countLabel: "Count",
  countPlaceholder: "count",
  customIdHint: "Lowercase letters, digits and hyphens, for example region-limit",
  customIdLabel: "Condition name",
  customPolicyLabel: "Custom policy (optional)",
  customPolicyInfo:
    "The policy that enforces this condition, for example region-guard v1. It must be attached to the MCP server to take effect.",
  customPolicyName: "Policy name",
  customPolicyNamePlaceholder: "e.g. region-guard",
  customPolicyVersion: "Policy version",
  customIdPlaceholder: "e.g. region-limit",
  empty: "No conditions: this plan is unlimited.",
  expand: (name: string) => `Show ${name} settings`,
  keyLabel: "Key",
  keyPlaceholder: "key",
  noLimitYet: "No limit yet",
  noValuesYet: "No values yet",
  onExceedLabel: "Block requests when quota is reached",
  onExceedReject: "Reject requests that exceed the limit.",
  overageAllowed: "overage allowed",
  pairEquals: "=",
  per: "per",
  removeCondition: (name: string) => `Remove ${name} condition`,
  removeLimit: "Remove limit",
  removeValue: "Remove value",
  tokenTypeLabel: "Tokens counted",
  unitLabel: "Window unit",
  valueLabel: "Value",
  valuePlaceholder: "value",
  windowLabel: "Window length",
};

const TYPE_NAMES: Record<string, string> = {
  custom: "Custom",
  "request-count": "Request count",
  "token-count": "Token count",
  "tool-call-count": "Tool-call count",
};

const TYPE_DESCRIPTIONS: Record<string, string> = {
  custom: "Your own condition, read by a policy you write.",
  "request-count": "Limits how many requests a subscriber makes.",
  "token-count": "Limits how many LLM tokens a subscriber uses.",
  "tool-call-count": "Limits how many MCP tools a subscriber calls.",
};

const TOKEN_TYPE_NAMES: Record<string, string> = {
  completion: "Completion tokens",
  prompt: "Prompt tokens",
  total: "Total tokens",
};

const UNIT_NAMES: Record<string, string> = {
  DAY: "Day",
  HOUR: "Hour",
  MINUTE: "Minute",
  MONTH: "Month",
};

const WHAT_NAMES: Record<string, string> = {
  completion: "completion tokens",
  prompt: "prompt tokens",
  requests: "requests",
  toolCalls: "tool calls",
  total: "tokens",
};

const UNIT_SINGULAR: Record<string, string> = {
  DAY: "day",
  HOUR: "hour",
  MINUTE: "minute",
  MONTH: "month",
};

/** One limit of a condition, e.g. "1,000 requests per minute" or "5,000 tokens per 2 hours". */
const windowSummary = (count: number, what: string, amount: number, unit: string) =>
  `${count.toLocaleString()} ${WHAT_NAMES[what] ?? "requests"} per ${
    amount === 1 ? UNIT_SINGULAR[unit] : `${amount} ${UNIT_SINGULAR[unit]}s`
  }`;

const TYPE_ICONS: Record<ConditionType, ReactNode> = {
  custom: <Puzzle size={16} />,
  "request-count": <Activity size={16} />,
  "token-count": <Coins size={16} />,
  "tool-call-count": <Wrench size={16} />,
};

/** A condition type's name, as the editor's menu and headers show it ("Request count"). */
export const conditionTypeName = (type: ConditionType): string => TYPE_NAMES[type] ?? "Custom";

const WHAT: Record<string, string> = {
  "request-count": "requests",
  "tool-call-count": "toolCalls",
};

/** The one line a condition's header shows. */
const summaryOf = (draft: ConditionDraft): string => {
  if (draft.type === "custom") {
    const pairs = draft.pairs
      .filter((pair) => pair.name.trim())
      .map((pair) => `${pair.name.trim()} = ${pair.value.trim()}`);
    return pairs.length > 0 ? pairs.join(" · ") : messages.noValuesYet;
  }
  const what = draft.type === "token-count" ? draft.tokenType : WHAT[draft.type];
  const parts = draft.windows
    .filter((window) => /^\d+$/.test(window.count.trim()) && Number(window.count) >= 1)
    .map((window) =>
      windowSummary(
        Number(window.count),
        what,
        Number(window.timeAmount) >= 1 ? Number(window.timeAmount) : 1,
        window.timeUnit,
      ),
    );
  if (parts.length === 0) return messages.noLimitYet;
  if (draft.onExceed === "allow") parts.push(messages.overageAllowed);
  return parts.join(" · ");
};

const ErrorText = ({ error }: { error: FieldError | undefined }) =>
  error ? (
    <Typography color="error" variant="caption">
      {error.message}
    </Typography>
  ) : null;

export type PlanConditionsEditorProps = {
  value: ConditionDraft[];
  onChange: (next: ConditionDraft[]) => void;
  /** The result of `checkConditions(value)`. */
  check: ConditionsCheck;
  /** Shows every problem, not only those of fields the user has left. */
  showAllErrors: boolean;
  /** A plan in use keeps its conditions: they are shown, not edited. */
  readOnly: boolean;
  disabled?: boolean;
};

/** The conditions of a plan: one card per condition, and a menu to add the built-in types not yet
 * used, or a custom condition. */
export function PlanConditionsEditor({
  value,
  onChange,
  check,
  showAllErrors,
  readOnly,
  disabled = false,
}: PlanConditionsEditorProps) {
  const menuId = useId();
  const [menuAnchor, setMenuAnchor] = useState<HTMLElement | null>(null);
  // The type picked from the menu, added once the menu has closed: while it is open it holds the
  // focus, which the new condition's first field takes.
  const [picked, setPicked] = useState<ConditionType | null>(null);
  // Which conditions are expanded; a saved plan's conditions open collapsed, new ones expanded.
  const [open, setOpen] = useState<Set<number>>(() => new Set());
  // Fields the user has left, whose problems now show.
  const [touched, setTouched] = useState<Set<string>>(() => new Set());

  const touch = (key: string) =>
    setTouched((current) => (current.has(key) ? current : new Set(current).add(key)));
  /** The problem of a field, once the user has left it or any other field of its row (`row`). */
  const errorOf = (key: string, row: string[] = [key]): FieldError | undefined =>
    showAllErrors || row.some((other) => touched.has(other)) ? check.errors.get(key) : undefined;

  const used = new Set(value.map((draft) => draft.type));
  // Every type stays listed; a built-in type the plan already has is shown disabled, marked Added.
  const menuTypes: ConditionType[] = [...BUILT_IN_TYPES, "custom"];
  const atMax = value.length >= MAX_CONDITIONS;
  const fieldsDisabled = disabled || readOnly;
  // A plan without conditions offers the condition types as cards; once it has one, Add condition
  // opens the menu.
  const showChooser = !readOnly && value.length === 0;

  const update = (key: number, patch: Partial<ConditionDraft>) =>
    onChange(value.map((draft) => (draft.key === key ? { ...draft, ...patch } : draft)));

  const add = (type: ConditionType) => {
    const draft = newCondition(type);
    setOpen((current) => new Set(current).add(draft.key));
    onChange([...value, draft]);
  };

  return (
    <Stack spacing={1.5}>
      <Stack alignItems="flex-start" direction="row" justifyContent="space-between" spacing={2}>
        <Box>
          <Stack alignItems="center" direction="row" spacing={1}>
            <Form.Header sx={{ fontWeight: 600, typography: "body2" }}>
              {messages.conditionsLabel}
            </Form.Header>
            {value.length > 0 && <Chip label={value.length} size="small" />}
          </Stack>
          <Typography
            color="text.secondary"
            sx={{ display: "block", opacity: 0.7 }}
            variant="caption"
          >
            {messages.conditionsHelper}
          </Typography>
        </Box>
        {!readOnly && !showChooser && (
          <>
            <Button
              aria-controls={menuAnchor ? menuId : undefined}
              aria-expanded={menuAnchor ? "true" : undefined}
              aria-haspopup="menu"
              color="secondary"
              disabled={disabled || atMax}
              onClick={(event) => setMenuAnchor(event.currentTarget)}
              size="small"
              startIcon={<Plus size={16} />}
              sx={{ flexShrink: 0 }}
              variant="outlined"
            >
              {messages.addCondition}
            </Button>
            <Menu
              anchorEl={menuAnchor}
              anchorOrigin={{ horizontal: "right", vertical: "bottom" }}
              // Focus goes to the new condition's first field, not back to this button.
              disableRestoreFocus={picked !== null}
              id={menuId}
              onClose={() => setMenuAnchor(null)}
              open={menuAnchor !== null}
              slotProps={{
                list: { "aria-label": messages.conditionTypes },
                transition: {
                  onExited: () => {
                    if (picked) add(picked);
                    setPicked(null);
                  },
                },
              }}
              transformOrigin={{ horizontal: "right", vertical: "top" }}
            >
              {menuTypes.map((type) => {
                const added = type !== "custom" && used.has(type);
                return (
                  <MenuItem
                    disabled={added}
                    key={type}
                    onClick={() => {
                      setPicked(type);
                      setMenuAnchor(null);
                    }}
                    sx={{ maxWidth: 380, whiteSpace: "normal" }}
                  >
                    <ListItemIcon>{TYPE_ICONS[type]}</ListItemIcon>
                    <ListItemText
                      primary={
                        <Stack alignItems="center" component="span" direction="row" spacing={1}>
                          <span>{conditionTypeName(type)}</span>
                          {added && (
                            <Chip component="span" label={messages.typeAdded} size="small" />
                          )}
                        </Stack>
                      }
                      secondary={TYPE_DESCRIPTIONS[type]}
                    />
                  </MenuItem>
                );
              })}
            </Menu>
          </>
        )}
      </Stack>

      {showChooser && (
        <ConditionTypeChooser disabled={disabled} onPick={add} />
      )}

      {value.length === 0 && !showChooser && (
        <Stack
          alignItems="center"
          direction="row"
          spacing={1.5}
          sx={{
            border: "1px dashed",
            borderColor: "divider",
            borderRadius: "10px",
            color: "text.secondary",
            px: 2,
            py: 2,
          }}
        >
          <InfinityIcon size={16} />
          <Typography variant="body2">{messages.empty}</Typography>
        </Stack>
      )}

      {value.map((draft) => (
        <ConditionCard
          check={check}
          disabled={fieldsDisabled}
          draft={draft}
          errorOf={errorOf}
          key={draft.key}
          onChange={(patch) => update(draft.key, patch)}
          onRemove={() => onChange(value.filter((other) => other.key !== draft.key))}
          onToggle={() =>
            setOpen((current) => {
              const next = new Set(current);
              if (next.has(draft.key)) next.delete(draft.key);
              else next.add(draft.key);
              return next;
            })
          }
          // A condition with a problem showing stays open, so the problem can be seen.
          open={open.has(draft.key) || (showAllErrors && check.conditionsWithErrors.has(draft.key))}
          readOnly={readOnly}
          touch={touch}
        />
      ))}

      {check.errors.get(fieldKeys.conditions()) && (
        <ErrorText error={check.errors.get(fieldKeys.conditions())} />
      )}
    </Stack>
  );
}

type ConditionTypeChooserProps = {
  disabled: boolean;
  onPick: (type: ConditionType) => void;
};

/** The condition types as outlined cards in one row, offered while a plan has no conditions. */
function ConditionTypeChooser({ disabled, onPick }: ConditionTypeChooserProps) {
  const types: ConditionType[] = [...BUILT_IN_TYPES, "custom"];
  return (
    <Box
      aria-label={messages.conditionTypes}
      role="group"
      sx={{
        display: "grid",
        gap: 1.5,
        gridTemplateColumns: { md: "repeat(4, minmax(0, 1fr))", xs: "repeat(2, minmax(0, 1fr))" },
      }}
    >
      {types.map((type) => (
        <ButtonBase
          disabled={disabled}
          key={type}
          onClick={() => onPick(type)}
          sx={(theme) => ({
            alignItems: "flex-start",
            border: "1px solid",
            borderColor: "divider",
            borderRadius: `${theme.shape.borderRadius}px`,
            display: "flex",
            flexDirection: "column",
            gap: 0.75,
            justifyContent: "flex-start",
            px: 1.75,
            py: 1.5,
            textAlign: "left",
            transition: theme.transitions.create(["border-color", "background-color"], {
              duration: theme.transitions.duration.shorter,
            }),
            "&:hover, &.Mui-focusVisible": { bgcolor: "action.hover", borderColor: "primary.main" },
            "&.Mui-disabled": { opacity: 0.5 },
          })}
        >
          <Stack alignItems="center" component="span" direction="row" spacing={1}>
            <Box component="span" sx={{ color: "text.secondary", display: "flex" }}>
              {TYPE_ICONS[type]}
            </Box>
            <Typography component="span" sx={{ fontWeight: 500 }} variant="body2">
              {conditionTypeName(type)}
            </Typography>
          </Stack>
          <Typography color="text.secondary" component="span" variant="caption">
            {TYPE_DESCRIPTIONS[type]}
          </Typography>
        </ButtonBase>
      ))}
    </Box>
  );
}

type ConditionCardProps = {
  draft: ConditionDraft;
  check: ConditionsCheck;
  open: boolean;
  readOnly: boolean;
  disabled: boolean;
  errorOf: (key: string, row?: string[]) => FieldError | undefined;
  touch: (key: string) => void;
  onChange: (patch: Partial<ConditionDraft>) => void;
  onRemove: () => void;
  onToggle: () => void;
};

function ConditionCard({
  draft,
  check,
  open,
  readOnly,
  disabled,
  errorOf,
  touch,
  onChange,
  onRemove,
  onToggle,
}: ConditionCardProps) {
  const id = useId();
  const name = conditionTypeName(draft.type);
  const hasError = check.conditionsWithErrors.has(draft.key);

  return (
    <Box
      aria-labelledby={`${id}-title`}
      role="group"
      sx={(theme) => ({
        border: hairline(theme),
        borderColor: hasError && !open ? "error.main" : "divider",
        borderRadius: "10px",
      })}
    >
      <Stack alignItems="center" direction="row" spacing={1} sx={{ pr: 1 }}>
        <ButtonBase
          aria-controls={`${id}-body`}
          aria-expanded={open}
          aria-label={(open ? messages.collapse : messages.expand)(name)}
          onClick={onToggle}
          sx={{
            borderRadius: "10px",
            flex: 1,
            gap: 1.25,
            justifyContent: "flex-start",
            minWidth: 0,
            px: 1.5,
            py: 1.25,
            textAlign: "left",
          }}
        >
          <Box
            component="span"
            sx={{
              color: "text.secondary",
              display: "flex",
              transform: open ? "rotate(90deg)" : "none",
              transition: "transform 150ms",
            }}
          >
            <ChevronRight size={16} />
          </Box>
          <Box component="span" sx={(theme) => iconTileSx(theme, 30)}>
            {TYPE_ICONS[draft.type]}
          </Box>
          <Box component="span" sx={{ display: "flex", flexDirection: "column", minWidth: 0 }}>
            <Typography
              component="span"
              id={`${id}-title`}
              sx={{ fontWeight: 600 }}
              variant="body2"
            >
              {name}
            </Typography>
            <Typography
              color={hasError && !open ? "error" : "text.secondary"}
              component="span"
              noWrap
              variant="caption"
            >
              {summaryOf(draft)}
            </Typography>
          </Box>
        </ButtonBase>
        {draft.type === "custom" && draft.id.trim() && (
          <Chip
            label={draft.id.trim()}
            size="small"
            sx={{ fontFamily: "monospace", maxWidth: 200 }}
            variant="outlined"
          />
        )}
        {!readOnly && (
          <IconButton
            aria-label={messages.removeCondition(name)}
            color="error"
            disabled={disabled}
            onClick={onRemove}
            size="small"
          >
            <Trash2 size={16} />
          </IconButton>
        )}
      </Stack>
      <Collapse in={open} unmountOnExit>
        <Box id={`${id}-body`} sx={{ px: 2, pb: 2, pt: 0.5 }}>
          {draft.type === "custom" ? (
            <CustomFields
              disabled={disabled}
              draft={draft}
              errorOf={errorOf}
              onChange={onChange}
              readOnly={readOnly}
              touch={touch}
            />
          ) : (
            <LimitFields
              disabled={disabled}
              draft={draft}
              errorOf={errorOf}
              onChange={onChange}
              readOnly={readOnly}
              touch={touch}
            />
          )}
        </Box>
      </Collapse>
    </Box>
  );
}

type FieldsProps = {
  draft: ConditionDraft;
  readOnly: boolean;
  disabled: boolean;
  errorOf: (key: string, row?: string[]) => FieldError | undefined;
  touch: (key: string) => void;
  onChange: (patch: Partial<ConditionDraft>) => void;
};

/** A built-in condition: what it counts (tokens), its windows, then what happens once a limit is
 * used up. */
function LimitFields({ draft, readOnly, disabled, errorOf, touch, onChange }: FieldsProps) {
  const id = useId();
  const setWindow = (key: number, patch: Partial<WindowDraft>) =>
    onChange({
      windows: draft.windows.map((window) =>
        window.key === key ? { ...window, ...patch } : window,
      ),
    });
  const windowsError = errorOf(fieldKeys.windows(draft.key));

  return (
    <Stack spacing={1.5}>
      {draft.type === "token-count" && (
        <FormControl sx={{ maxWidth: 320 }}>
          <FormLabel id={`${id}-token-type`}>{messages.tokenTypeLabel}</FormLabel>
          <Select
            disabled={disabled}
            labelId={`${id}-token-type`}
            onChange={(event) => onChange({ tokenType: event.target.value as TokenType })}
            size="small"
            value={draft.tokenType}
          >
            {TOKEN_TYPES.map((tokenType) => (
              <MenuItem key={tokenType} value={tokenType}>
                {TOKEN_TYPE_NAMES[tokenType]}
              </MenuItem>
            ))}
          </Select>
        </FormControl>
      )}

      <Stack spacing={1}>
        {draft.windows.map((window) => {
          const countKey = fieldKeys.count(draft.key, window.key);
          const amountKey = fieldKeys.amount(draft.key, window.key);
          const unitKey = fieldKeys.unit(draft.key, window.key);
          const row = [countKey, amountKey, unitKey];
          const countError = errorOf(countKey, row);
          const amountError = errorOf(amountKey, row);
          const unitError = errorOf(unitKey, row);
          const rowError = countError ?? amountError ?? unitError;
          return (
            <Box key={window.key}>
              <Box
                sx={{
                  alignItems: "center",
                  display: "grid",
                  gap: 1,
                  gridTemplateColumns: "minmax(0, 1.4fr) auto minmax(0, 0.7fr) minmax(0, 1fr) auto",
                }}
              >
                <OutlinedInput
                  autoFocus={!readOnly && window.count === ""}
                  disabled={disabled}
                  error={Boolean(countError)}
                  inputProps={{
                    "aria-label": messages.countLabel,
                    inputMode: "numeric",
                    min: 1,
                  }}
                  onBlur={() => touch(countKey)}
                  onChange={(event) => setWindow(window.key, { count: event.target.value })}
                  placeholder={messages.countPlaceholder}
                  size="small"
                  sx={{ fontFamily: "monospace" }}
                  type="number"
                  value={window.count}
                />
                <Typography color="text.secondary" variant="body2">
                  {messages.per}
                </Typography>
                <OutlinedInput
                  disabled={disabled}
                  error={Boolean(amountError)}
                  inputProps={{
                    "aria-label": messages.windowLabel,
                    inputMode: "numeric",
                    min: 1,
                  }}
                  onBlur={() => touch(amountKey)}
                  onChange={(event) => setWindow(window.key, { timeAmount: event.target.value })}
                  placeholder="1"
                  size="small"
                  sx={{ fontFamily: "monospace" }}
                  type="number"
                  value={window.timeAmount}
                />
                <Select
                  disabled={disabled}
                  error={Boolean(unitError)}
                  inputProps={{ "aria-label": messages.unitLabel }}
                  onChange={(event) => {
                    setWindow(window.key, { timeUnit: event.target.value as TimeUnit });
                    touch(unitKey);
                  }}
                  size="small"
                  value={window.timeUnit}
                >
                  {TIME_UNITS.map((unit) => (
                    <MenuItem key={unit} value={unit}>
                      {UNIT_NAMES[unit]}
                    </MenuItem>
                  ))}
                </Select>
                {readOnly ? (
                  <Box sx={{ width: 34 }} />
                ) : (
                  <IconButton
                    aria-label={messages.removeLimit}
                    color="error"
                    // A built-in condition keeps at least one limit; remove the condition instead.
                    disabled={disabled || draft.windows.length <= 1}
                    onClick={() =>
                      onChange({
                        windows: draft.windows.filter((other) => other.key !== window.key),
                      })
                    }
                    size="small"
                  >
                    <X size={16} />
                  </IconButton>
                )}
              </Box>
              {rowError && <FormHelperText error>{rowError.message}</FormHelperText>}
            </Box>
          );
        })}
      </Stack>
      <ErrorText error={windowsError} />
      {!readOnly && (
        <Box>
          <Button
            disabled={disabled || draft.windows.length >= MAX_WINDOWS}
            onClick={() => onChange({ windows: [...draft.windows, newWindow()] })}
            size="small"
            startIcon={<Plus size={16} />}
            variant="text"
          >
            {messages.addLimit}
          </Button>
        </Box>
      )}
      {/* What happens once a limit is used up, after the limits: main's bordered switch row. */}
      <Stack
        alignItems="center"
        direction="row"
        justifyContent="space-between"
        spacing={2}
        sx={{ border: "1px solid", borderColor: "divider", borderRadius: "10px", px: 2, py: 1.5 }}
      >
        <Box>
          <Typography id={`${id}-on-exceed-label`} sx={{ fontWeight: 600 }} variant="body1">
            {messages.onExceedLabel}
          </Typography>
          <Typography color="text.secondary" sx={{ opacity: 0.7 }} variant="body2">
            {messages.onExceedReject}
          </Typography>
        </Box>
        <Switch
          checked={draft.onExceed === "reject"}
          disabled={disabled}
          onChange={(event) =>
            onChange({ onExceed: (event.target.checked ? "reject" : "allow") as OnExceed })
          }
          slotProps={{ input: { "aria-labelledby": `${id}-on-exceed-label`, role: "switch" } }}
        />
      </Stack>
    </Stack>
  );
}

/** A custom condition: its name, the policy that enforces it (optional), and the key-value pairs
 * that policy reads. */
function CustomFields({ draft, readOnly, disabled, errorOf, touch, onChange }: FieldsProps) {
  const id = useId();
  const idKey = fieldKeys.customId(draft.key);
  const idError = errorOf(idKey);
  const policyNameKey = fieldKeys.policyName(draft.key);
  const policyVersionKey = fieldKeys.policyVersion(draft.key);
  const policyNameError = errorOf(policyNameKey);
  const policyVersionError = errorOf(policyVersionKey);
  // Too many bytes in all: shown once any value has been left.
  const paramsError = errorOf(
    fieldKeys.params(draft.key),
    draft.pairs.map((pair) => `${fieldKeys.pairName(draft.key, pair.key)}.value`),
  );

  return (
    <Stack spacing={1.5}>
      <FormControl error={Boolean(idError)} fullWidth required>
        <FormLabel htmlFor={`${id}-id`}>{messages.customIdLabel}</FormLabel>
        <OutlinedInput
          autoFocus={!readOnly && draft.id === ""}
          disabled={disabled}
          id={`${id}-id`}
          inputProps={{ "aria-describedby": `${id}-id-helper`, maxLength: MAX_CUSTOM_ID_CHARS }}
          onBlur={() => touch(idKey)}
          onChange={(event) => onChange({ id: event.target.value })}
          placeholder={messages.customIdPlaceholder}
          size="small"
          sx={{ fontFamily: "monospace" }}
          value={draft.id}
        />
        <FormHelperText id={`${id}-id-helper`}>
          {idError ? idError.message : messages.customIdHint}
        </FormHelperText>
      </FormControl>

      <FormControl fullWidth>
        <Stack alignItems="center" direction="row" spacing={0.5}>
          <FormLabel id={`${id}-policy`}>{messages.customPolicyLabel}</FormLabel>
          <Tooltip title={messages.customPolicyInfo}>
            <Box
              aria-label={messages.customPolicyInfo}
              component="span"
              role="img"
              sx={{ color: "text.secondary", display: "inline-flex" }}
            >
              <Info size={14} />
            </Box>
          </Tooltip>
        </Stack>
        <Box
          sx={{
            display: "grid",
            gap: 1,
            gridTemplateColumns: "minmax(0, 1.4fr) minmax(0, 0.6fr)",
          }}
        >
          <OutlinedInput
            disabled={disabled}
            error={Boolean(policyNameError)}
            inputProps={{ "aria-label": messages.customPolicyName }}
            onBlur={() => touch(policyNameKey)}
            onChange={(event) => onChange({ policyName: event.target.value })}
            placeholder={messages.customPolicyNamePlaceholder}
            size="small"
            sx={{ fontFamily: "monospace" }}
            value={draft.policyName}
          />
          <OutlinedInput
            disabled={disabled}
            error={Boolean(policyVersionError)}
            inputProps={{ "aria-label": messages.customPolicyVersion }}
            onBlur={() => touch(policyVersionKey)}
            onChange={(event) => onChange({ policyVersion: event.target.value })}
            placeholder="v1"
            size="small"
            sx={{ fontFamily: "monospace" }}
            value={draft.policyVersion}
          />
        </Box>
        {(policyNameError ?? policyVersionError) && (
          <FormHelperText error>{(policyNameError ?? policyVersionError)!.message}</FormHelperText>
        )}
      </FormControl>

      <Stack spacing={1}>
        {draft.pairs.map((pair) => {
          const nameKey = fieldKeys.pairName(draft.key, pair.key);
          const valueKey = `${nameKey}.value`;
          const nameError = errorOf(nameKey, [nameKey, valueKey]);
          const setPair = (patch: Partial<typeof pair>) =>
            onChange({
              pairs: draft.pairs.map((other) =>
                other.key === pair.key ? { ...other, ...patch } : other,
              ),
            });
          return (
            <Box key={pair.key}>
              <Box
                sx={{
                  alignItems: "center",
                  display: "grid",
                  gap: 1,
                  gridTemplateColumns: "minmax(0, 1fr) auto minmax(0, 1.4fr) auto",
                }}
              >
                <OutlinedInput
                  disabled={disabled}
                  error={Boolean(nameError)}
                  inputProps={{ "aria-label": messages.keyLabel }}
                  onBlur={() => touch(nameKey)}
                  onChange={(event) => setPair({ name: event.target.value })}
                  placeholder={messages.keyPlaceholder}
                  size="small"
                  sx={{ fontFamily: "monospace" }}
                  value={pair.name}
                />
                <Typography color="text.secondary" variant="body2">
                  {messages.pairEquals}
                </Typography>
                <OutlinedInput
                  disabled={disabled}
                  inputProps={{ "aria-label": messages.valueLabel }}
                  onBlur={() => touch(valueKey)}
                  onChange={(event) => setPair({ value: event.target.value })}
                  placeholder={messages.valuePlaceholder}
                  size="small"
                  sx={{ fontFamily: "monospace" }}
                  value={pair.value}
                />
                {readOnly ? (
                  <Box sx={{ width: 34 }} />
                ) : (
                  <IconButton
                    aria-label={messages.removeValue}
                    color="error"
                    disabled={disabled}
                    onClick={() =>
                      onChange({ pairs: draft.pairs.filter((other) => other.key !== pair.key) })
                    }
                    size="small"
                  >
                    <X size={16} />
                  </IconButton>
                )}
              </Box>
              {nameError && <FormHelperText error>{nameError.message}</FormHelperText>}
            </Box>
          );
        })}
      </Stack>
      <ErrorText error={paramsError} />
      {!readOnly && (
        <Box>
          <Button
            disabled={disabled}
            onClick={() => onChange({ pairs: [...draft.pairs, newPair()] })}
            size="small"
            startIcon={<Plus size={16} />}
            variant="text"
          >
            {messages.addValue}
          </Button>
        </Box>
      )}
    </Stack>
  );
}

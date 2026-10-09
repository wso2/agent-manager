"use client";

import { useState, type ChangeEvent } from "react";

type ChatComposerProps = {
  onSend: (message: string) => void;
  onStop?: () => void;
  disabled?: boolean;
  loading?: boolean;
};

export default function ChatComposer({
  onSend,
  onStop,
  disabled,
  loading,
}: ChatComposerProps) {
  const [value, setValue] = useState("");

  const submit = () => {
    if (disabled) {
      return;
    }
    const trimmed = value.trim();
    if (!trimmed) {
      return;
    }

    onSend(trimmed);
    setValue("");
  };

  const handleKeyDown = (event: React.KeyboardEvent<HTMLTextAreaElement>) => {
    if (event.key === "Enter" && !event.shiftKey) {
      event.preventDefault();
      submit();
    }
  };

  return (
    <div className="composer">
      <form
        className="composer-card"
        onSubmit={(event) => {
          event.preventDefault();
          submit();
        }}
      >
        <textarea
          className="composer-input"
          value={value}
          onChange={(e: ChangeEvent<HTMLTextAreaElement>) =>
            setValue(e.target.value)
          }
          onKeyDown={handleKeyDown}
          placeholder="Describe the report you want (company, macro, or market topic)..."
          disabled={disabled}
          rows={2}
        />
        <div className="composer-actions">
          {loading && onStop ? (
            <button
              className="composer-stop"
              type="button"
              onClick={onStop}
              aria-label="Stop response"
            >
              <span />
            </button>
          ) : (
            <button
              className="composer-send"
              type="submit"
              disabled={disabled || !value.trim()}
              aria-label="Send message"
            >
              <svg
                width="20"
                height="20"
                viewBox="0 0 24 24"
                fill="none"
                stroke="currentColor"
                strokeWidth="2"
              >
                <path d="M22 2L11 13M22 2l-7 20-4-9-9-4 20-7z" />
              </svg>
            </button>
          )}
        </div>
      </form>
    </div>
  );
}

import { useId, useImperativeHandle, useRef, useState, type ClipboardEvent, type KeyboardEvent, type Ref } from "react";
import { X } from "lucide-react";
import { addKeywords, normalizeKeyword, removeKeyword } from "../lib/keywords";
import { FieldError } from "./AuthCard";

/** Imperative handle: lets a form fold the text still being typed into the list when it saves. */
export interface TagInputHandle {
  /** Adds the pending text as chips and returns the resulting list (the current one when nothing is pending). */
  commit: () => string[];
}

interface Props {
  label: string;
  value: string[];
  onChange: (value: string[]) => void;
  hint?: string;
  /** Validation message under the field. */
  error?: string | null;
  /** A keyword to mark as rejected (for example one the server says belongs to another category). */
  invalidKeyword?: string | null;
  /** Message shown next to the rejected chip. */
  invalidMessage?: string;
  placeholder?: string;
  handleRef?: Ref<TagInputHandle>;
}

const boxClass =
  "focus-within:ring-2 focus-within:ring-ring flex min-h-11 w-full flex-wrap items-center gap-1.5 rounded-lg border border-input bg-surface px-2 py-1.5 text-foreground transition-colors duration-200 aria-[invalid=true]:border-destructive";

/**
 * Chip input for a list of short words. Enter or comma adds the typed word, a pasted
 * "a, b, c" becomes three chips, Backspace on an empty field removes the last chip and a
 * word already in the list (ignoring case and accents) is not added, with a short hint.
 * The word still being typed is added on blur, so nothing is lost when the user goes straight to Save.
 */
export function TagInput({ label, value, onChange, hint, error, invalidKeyword, invalidMessage, placeholder, handleRef }: Props) {
  const id = useId();
  const hintId = `${id}-hint`;
  const errorId = `${id}-error`;
  const noteId = `${id}-note`;
  const inputRef = useRef<HTMLInputElement>(null);
  const [text, setText] = useState("");
  const [duplicates, setDuplicates] = useState<string[]>([]);
  // The latest list and text, so commit() is right even before React re-renders.
  const latest = useRef({ value, text });
  latest.current = { value, text };

  function commit(raw: string): string[] {
    const { keywords, duplicates: dup } = addKeywords(latest.current.value, raw);
    setDuplicates(dup);
    setText("");
    latest.current = { value: keywords, text: "" };
    if (keywords.length !== value.length) onChange(keywords);
    return keywords;
  }

  useImperativeHandle(handleRef, () => ({ commit: () => (latest.current.text.trim() ? commit(latest.current.text) : latest.current.value) }));

  function onKeyDown(e: KeyboardEvent<HTMLInputElement>) {
    if (e.key === "Enter" || e.key === ",") {
      e.preventDefault();
      if (text.trim()) commit(text);
    } else if (e.key === "Backspace" && text === "" && value.length > 0) {
      setDuplicates([]);
      onChange(value.slice(0, -1));
    }
  }

  function onPaste(e: ClipboardEvent<HTMLInputElement>) {
    const pasted = e.clipboardData.getData("text");
    if (!/[,\n]/.test(pasted)) return;
    e.preventDefault();
    commit(`${text}${pasted}`);
  }

  const noteText = duplicates.length > 0 ? `Ya estaba: ${duplicates.join(", ")}.` : "";
  const describedBy = [error ? errorId : null, noteText ? noteId : null, hint ? hintId : null].filter(Boolean).join(" ") || undefined;
  const invalidKey = invalidKeyword ? normalizeKeyword(invalidKeyword) : null;

  return (
    <div className="min-w-0">
      <label htmlFor={id} className="mb-1.5 block text-sm font-medium">
        {label}
      </label>
      {/* The box only forwards clicks to the input; the input is the single tab stop. */}
      <div role="presentation" aria-invalid={error ? true : undefined} className={boxClass} onClick={() => inputRef.current?.focus()}>
        <ul className="contents" aria-label={`Lista de ${label.toLowerCase()}`}>
          {value.map((kw) => {
            const bad = invalidKey !== null && normalizeKeyword(kw) === invalidKey;
            return (
              <li
                key={kw}
                data-invalid={bad ? "true" : undefined}
                className={`inline-flex min-h-8 max-w-full items-center gap-1 rounded-full border py-0.5 pl-3 pr-1 text-sm ${
                  bad ? "border-destructive bg-destructive/10 text-destructive" : "border-border bg-primary/10"
                }`}
              >
                <span className="min-w-0 break-words">{kw}</span>
                <button
                  type="button"
                  aria-label={`Quitar ${kw}`}
                  onClick={(e) => {
                    e.stopPropagation();
                    setDuplicates([]);
                    onChange(removeKeyword(value, kw));
                    inputRef.current?.focus();
                  }}
                  className="focus-ring flex h-8 w-8 shrink-0 items-center justify-center rounded-full text-muted transition-colors duration-200 hover:bg-primary/20 hover:text-foreground"
                >
                  <X className="h-4 w-4" aria-hidden="true" />
                </button>
              </li>
            );
          })}
        </ul>
        <input
          ref={inputRef}
          id={id}
          value={text}
          onChange={(e) => {
            setText(e.target.value);
            if (duplicates.length > 0) setDuplicates([]);
          }}
          onKeyDown={onKeyDown}
          onPaste={onPaste}
          onBlur={() => {
            if (text.trim()) commit(text);
          }}
          placeholder={value.length === 0 ? placeholder : undefined}
          autoComplete="off"
          autoCapitalize="none"
          enterKeyHint="done"
          aria-describedby={describedBy}
          aria-invalid={error ? true : undefined}
          className="min-h-8 min-w-[8ch] flex-1 bg-transparent px-1 py-1 outline-none"
        />
      </div>
      {invalidKey !== null && invalidMessage && (
        <p role="alert" className="mt-1.5 text-sm text-destructive">
          {invalidMessage}
        </p>
      )}
      {noteText && (
        <p id={noteId} role="status" className="mt-1.5 text-sm text-muted">
          {noteText}
        </p>
      )}
      {hint && (
        <p id={hintId} className="mt-1.5 text-sm text-muted">
          {hint}
        </p>
      )}
      {error && <FieldError id={errorId}>{error}</FieldError>}
    </div>
  );
}

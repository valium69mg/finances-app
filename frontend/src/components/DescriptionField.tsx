import { useRef, useState, type KeyboardEvent } from "react";
import { descriptionAfterPick, matchKeywords, type KeywordOwner } from "../lib/keywords";
import { TextField } from "./AuthCard";

interface Props {
  label?: string;
  value: string;
  onChange: (value: string) => void;
  /** Known keywords of the Gasto categories, from the settings the page already loaded. */
  owners: KeywordOwner[];
  /** Called with the picked suggestion; the description is already updated through onChange when it should change. */
  onPick: (owner: KeywordOwner) => void;
  maxLength?: number;
}

/**
 * Description input with quick-pick chips under it: the known keywords matching what is typed,
 * each showing its category ("heb · Súper"). Picking one selects its category and completes the
 * description only when it is empty or just the start of the keyword. ArrowDown moves from the
 * input into the chips, Left/Right move between them, Enter or a tap picks, Escape closes the list
 * (it comes back as soon as the text changes). Matching runs on the loaded settings: no request.
 */
export function DescriptionField({ label = "Descripción", value, onChange, owners, onPick, maxLength }: Props) {
  const [dismissed, setDismissed] = useState(false);
  const inputRef = useRef<HTMLInputElement>(null);
  const group = useRef<HTMLDivElement>(null);
  const suggestions = dismissed ? [] : matchKeywords(value, owners);

  const chips = () => Array.from(group.current?.querySelectorAll<HTMLButtonElement>("button") ?? []);

  function pick(owner: KeywordOwner) {
    onChange(descriptionAfterPick(value, owner.keyword));
    onPick(owner);
    setDismissed(true);
    inputRef.current?.focus();
  }

  function onInputKeyDown(e: KeyboardEvent<HTMLInputElement>) {
    if (e.key === "ArrowDown" && suggestions.length > 0) {
      e.preventDefault();
      chips()[0]?.focus();
    } else if (e.key === "Escape" && suggestions.length > 0) {
      // Close the suggestions first; a second Escape reaches the dialog.
      e.stopPropagation();
      setDismissed(true);
    }
  }

  function onChipKeyDown(e: KeyboardEvent<HTMLDivElement>) {
    const list = chips();
    const at = list.indexOf(document.activeElement as HTMLButtonElement);
    if (at < 0) return;
    if (e.key === "ArrowRight" || e.key === "ArrowDown") {
      e.preventDefault();
      list[(at + 1) % list.length]?.focus();
    } else if (e.key === "ArrowLeft" || e.key === "ArrowUp") {
      e.preventDefault();
      if (at === 0) inputRef.current?.focus();
      else list[at - 1]?.focus();
    } else if (e.key === "Escape") {
      e.preventDefault();
      e.stopPropagation();
      setDismissed(true);
      inputRef.current?.focus();
    }
  }

  return (
    <div className="min-w-0">
      <TextField
        label={label}
        value={value}
        inputRef={inputRef}
        onChange={(e) => {
          setDismissed(false);
          onChange(e.target.value);
        }}
        onKeyDown={onInputKeyDown}
        maxLength={maxLength}
        autoComplete="off"
      />
      {suggestions.length > 0 && (
        <div ref={group} role="group" aria-label="Palabras clave sugeridas" onKeyDown={onChipKeyDown} className="mt-2 flex flex-wrap gap-2">
          {suggestions.map((s) => (
            <button
              key={`${s.category}:${s.keyword}`}
              type="button"
              onClick={() => pick(s)}
              className="focus-ring inline-flex min-h-11 items-center rounded-full border border-border bg-primary/10 px-4 text-sm transition-colors duration-200 hover:bg-primary/20"
            >
              <span className="font-medium">{s.keyword}</span>
              <span className="text-muted">&nbsp;·&nbsp;{s.category}</span>
            </button>
          ))}
        </div>
      )}
    </div>
  );
}

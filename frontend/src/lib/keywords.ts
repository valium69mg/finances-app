/**
 * Keyword helpers shared by the category tag input and the expense quick-pick.
 * Matching ignores case and accents, like the backend (ledger.Normalize), so a
 * keyword the user sees as "the same" is the same for both sides.
 */

/** Lowercase, trimmed, accents stripped: the comparison key of a keyword. */
export function normalizeKeyword(text: string): string {
  return text
    .trim()
    .normalize("NFKD")
    .replace(/\p{Mn}/gu, "")
    .toLowerCase();
}

/** Splits typed or pasted text on commas and newlines into trimmed, non-empty keywords. */
export function splitKeywords(text: string): string[] {
  return text
    .split(/[,\n]/)
    .map((k) => k.trim())
    .filter(Boolean);
}

export interface AddResult {
  keywords: string[];
  /** Typed keywords ignored because the list already had them (as typed, deduplicated). */
  duplicates: string[];
}

/**
 * Appends the keywords found in `text` to `list`, ignoring the ones already there
 * or repeated inside the text (normalized). The first spelling wins.
 */
export function addKeywords(list: string[], text: string): AddResult {
  const seen = new Set(list.map(normalizeKeyword));
  const keywords = [...list];
  const duplicates: string[] = [];
  for (const kw of splitKeywords(text)) {
    const key = normalizeKeyword(kw);
    if (seen.has(key)) {
      if (!duplicates.some((d) => normalizeKeyword(d) === key)) duplicates.push(kw);
      continue;
    }
    seen.add(key);
    keywords.push(kw);
  }
  return { keywords, duplicates };
}

/** Removes one keyword (compared normalized). */
export function removeKeyword(list: string[], keyword: string): string[] {
  const key = normalizeKeyword(keyword);
  return list.filter((k) => normalizeKeyword(k) !== key);
}

export interface KeywordOwner {
  keyword: string;
  category: string;
}

export interface KeywordSuggestion extends KeywordOwner {
  /** Hint shown with the chip: the category the keyword belongs to. */
  label: string;
}

const MAX_SUGGESTIONS = 6;

/**
 * Known keywords of the categories of one kind that match what is being typed:
 * the keyword starts with a word of the text or the text contains the keyword
 * as a prefix of a word or whole, and every candidate is compared normalized.
 * Prefix matches rank above substring matches; ties keep configuration order.
 * An empty text suggests nothing. Duplicated keywords (legacy data) show once,
 * for the first category that owns them.
 */
export function matchKeywords(text: string, owners: KeywordOwner[], limit = MAX_SUGGESTIONS): KeywordSuggestion[] {
  const query = normalizeKeyword(text);
  if (query.length === 0) return [];
  const words = query.split(/\s+/);
  const seen = new Set<string>();
  const ranked: { owner: KeywordOwner; rank: number; order: number }[] = [];
  owners.forEach((owner, order) => {
    const key = normalizeKeyword(owner.keyword);
    if (!key || seen.has(key)) return;
    let rank = -1;
    if (key.startsWith(query) || words.some((w) => w.length > 0 && key.startsWith(w))) rank = 0;
    else if (key.includes(query) || words.some((w) => w.length >= 2 && key.includes(w))) rank = 1;
    if (rank < 0) return;
    seen.add(key);
    ranked.push({ owner, rank, order });
  });
  ranked.sort((a, b) => a.rank - b.rank || a.order - b.order);
  return ranked.slice(0, limit).map(({ owner }) => ({ ...owner, label: `${owner.keyword} · ${owner.category}` }));
}

/**
 * What the description becomes when a suggestion is picked: the keyword when the
 * description is empty or only the beginning of it (so "heb" completes to
 * "heb"), otherwise the description untouched.
 */
export function descriptionAfterPick(description: string, keyword: string): string {
  const typed = normalizeKeyword(description);
  if (typed === "" || normalizeKeyword(keyword).startsWith(typed)) return keyword;
  return description;
}

/** Flattens the Gasto categories into keyword owners, in configuration order. */
export function keywordOwners(categories: { name: string; kind: string; keywords: string[] }[], kind: string): KeywordOwner[] {
  return categories.filter((c) => c.kind === kind).flatMap((c) => c.keywords.map((keyword) => ({ keyword, category: c.name })));
}

package domain

import (
	"fmt"
	"strings"

	ledger "github.com/valium69mg/finances-app/backend/internal/ledger/domain"
)

// KeywordInUseError reports that a keyword already belongs to another category
// of the same kind. Matching is case- and accent-insensitive (ledger.Normalize),
// the same comparison Config.InferCategory uses, so one keyword maps to exactly
// one category.
type KeywordInUseError struct {
	Keyword  string // as the caller typed it
	Category string // the category that already owns it
}

func (e *KeywordInUseError) Error() string {
	return fmt.Sprintf("keyword %q already belongs to the category %q", e.Keyword, e.Category)
}

// CleanKeywords trims the keywords, drops the empty ones and drops duplicates
// by ledger.Normalize, keeping the first spelling. It never returns nil.
func CleanKeywords(keywords []string) []string {
	out := make([]string, 0, len(keywords))
	seen := map[string]bool{}
	for _, kw := range keywords {
		kw = strings.TrimSpace(kw)
		if kw == "" {
			continue
		}
		n := ledger.Normalize(kw)
		if seen[n] {
			continue
		}
		seen[n] = true
		out = append(out, kw)
	}
	return out
}

// CleanCategories returns a copy of cats with every category's keywords
// cleaned by CleanKeywords. The input is not modified.
func CleanCategories(cats []Category) []Category {
	out := make([]Category, len(cats))
	for i, c := range cats {
		c.Keywords = CleanKeywords(c.Keywords)
		out[i] = c
	}
	return out
}

// CheckKeywordOwnership enforces "a keyword belongs to one category per kind"
// for what is being saved, not for the whole list: only the keywords a category
// gained compared with the stored list (a new category gains all of its own)
// are checked, against the keywords of the other categories of its kind. A
// conflict that already exists in the stored data and is left untouched never
// blocks the save of an unrelated category. The caller passes cleaned
// categories (CleanCategories). The error is a *KeywordInUseError.
func CheckKeywordOwnership(stored, updated []Category) error {
	before := map[string]map[string]bool{}
	for _, c := range stored {
		set := map[string]bool{}
		for _, kw := range c.Keywords {
			set[ledger.Normalize(kw)] = true
		}
		before[c.Name] = set
	}
	for _, c := range updated {
		for _, kw := range c.Keywords {
			n := ledger.Normalize(kw)
			if before[c.Name][n] {
				continue
			}
			for _, other := range updated {
				if other.Name == c.Name || other.Kind != c.Kind {
					continue
				}
				for _, okw := range other.Keywords {
					if ledger.Normalize(okw) == n {
						return &KeywordInUseError{Keyword: kw, Category: other.Name}
					}
				}
			}
		}
	}
	return nil
}

// KeywordConflict is a keyword shared by several categories of one kind.
type KeywordConflict struct {
	Keyword    string // normalized
	Kind       ledger.Kind
	Categories []string
}

// FindKeywordConflicts lists the keywords owned by more than one category of
// the same kind, in first-seen order. It is read-only: legacy data is reported,
// never rewritten.
func FindKeywordConflicts(cats []Category) []KeywordConflict {
	type key struct {
		kind ledger.Kind
		kw   string
	}
	index := map[key]int{}
	var out []KeywordConflict
	for _, c := range cats {
		for _, kw := range CleanKeywords(c.Keywords) {
			k := key{c.Kind, ledger.Normalize(kw)}
			i, ok := index[k]
			if !ok {
				index[k] = len(out)
				out = append(out, KeywordConflict{Keyword: k.kw, Kind: c.Kind, Categories: []string{c.Name}})
				continue
			}
			out[i].Categories = append(out[i].Categories, c.Name)
		}
	}
	conflicts := out[:0]
	for _, c := range out {
		if len(c.Categories) > 1 {
			conflicts = append(conflicts, c)
		}
	}
	return conflicts
}

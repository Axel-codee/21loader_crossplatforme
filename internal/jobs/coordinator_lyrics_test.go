package jobs

import "testing"

func TestExtractLyricsFoundSummary(t *testing.T) {
	chunk := "[lyrics] Recherche 10/11: 10. All I Ask\n" +
		"[lyrics] Sous-titres synchronises generes.\n" +
		"[lyrics] Termine: 10 genere(s), 1 deja present(s), 0 erreur(s).\n"

	found, total, ok := extractLyricsFoundSummary(chunk)
	if !ok {
		t.Fatalf("expected extractLyricsFoundSummary to match")
	}
	if found != 11 || total != 11 {
		t.Fatalf("unexpected summary values: found=%d total=%d", found, total)
	}
}

func TestExtractLyricsFoundSummaryNoMatch(t *testing.T) {
	found, total, ok := extractLyricsFoundSummary("[lyrics] Recherche 1/11: 01. Hello\n")
	if ok {
		t.Fatalf("expected no match, got found=%d total=%d", found, total)
	}
}

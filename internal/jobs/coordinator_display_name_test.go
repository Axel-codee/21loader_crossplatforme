package jobs

import (
	"strings"
	"testing"
	"time"

	"persodl-cross/internal/core"
	"persodl-cross/internal/xuuid"
)

func TestSetRuntimeDisplayNameSetsYouTubeWhenMissing(t *testing.T) {
	id := xuuid.New()
	c := &Coordinator{
		jobs: []core.JobRecord{
			core.NewJobRecord(core.JobRequest{
				ID:         id,
				CreatedAt:  time.Now().UTC(),
				SourceKind: core.SourceYouTube,
			}),
		},
		displayNameByJobID: map[xuuid.UUID]string{},
	}

	c.setRuntimeDisplayName(id, "Spencer Maro - Starfire")

	got := strings.TrimSpace(c.displayNameByJobID[id])
	if got != "Spencer Maro - Starfire" {
		t.Fatalf("unexpected display name: %q", got)
	}
}

func TestSetRuntimeDisplayNameDoesNotOverrideExistingName(t *testing.T) {
	id := xuuid.New()
	c := &Coordinator{
		jobs: []core.JobRecord{
			core.NewJobRecord(core.JobRequest{
				ID:         id,
				CreatedAt:  time.Now().UTC(),
				SourceKind: core.SourceYouTube,
			}),
		},
		displayNameByJobID: map[xuuid.UUID]string{
			id: "Nom deja defini",
		},
	}

	c.setRuntimeDisplayName(id, "Nouveau nom")

	if got := strings.TrimSpace(c.displayNameByJobID[id]); got != "Nom deja defini" {
		t.Fatalf("existing display name should stay unchanged, got=%q", got)
	}
}

func TestSetRuntimeDisplayNameSkipsNonYouTubeAndCustomName(t *testing.T) {
	idQobuz := xuuid.New()
	idCustom := xuuid.New()
	c := &Coordinator{
		jobs: []core.JobRecord{
			core.NewJobRecord(core.JobRequest{
				ID:         idQobuz,
				CreatedAt:  time.Now().UTC(),
				SourceKind: core.SourceQobuz,
			}),
			core.NewJobRecord(core.JobRequest{
				ID:         idCustom,
				CreatedAt:  time.Now().UTC(),
				SourceKind: core.SourceYouTube,
				CustomName: "Mon nom perso",
			}),
		},
		displayNameByJobID: map[xuuid.UUID]string{},
	}

	c.setRuntimeDisplayName(idQobuz, "Album X")
	c.setRuntimeDisplayName(idCustom, "Titre YouTube")

	if got := strings.TrimSpace(c.displayNameByJobID[idQobuz]); got != "" {
		t.Fatalf("qobuz job should not receive runtime youtube display name, got=%q", got)
	}
	if got := strings.TrimSpace(c.displayNameByJobID[idCustom]); got != "" {
		t.Fatalf("job with custom name should not receive runtime display name, got=%q", got)
	}
}

func TestResolveEnqueueDisplayNameKeepsProvidedDisplayName(t *testing.T) {
	c := &Coordinator{}
	built := builtJob{
		Request: core.JobRequest{
			ID:         xuuid.New(),
			CreatedAt:  time.Now().UTC(),
			SourceKind: core.SourceYouTube,
			InputURL:   "https://www.youtube.com/watch?v=86aHZNYEUjw",
		},
		DisplayName: "Nom deja fourni",
	}
	got := c.resolveEnqueueDisplayName(nil, built)
	if got != "Nom deja fourni" {
		t.Fatalf("provided displayName should be kept, got=%q", got)
	}
}

func TestResolveEnqueueDisplayNameSkipsWhenServiceUnavailable(t *testing.T) {
	c := &Coordinator{}
	built := builtJob{
		Request: core.JobRequest{
			ID:         xuuid.New(),
			CreatedAt:  time.Now().UTC(),
			SourceKind: core.SourceYouTube,
			InputURL:   "https://www.youtube.com/watch?v=86aHZNYEUjw",
		},
	}
	got := c.resolveEnqueueDisplayName(nil, built)
	if got != "" {
		t.Fatalf("expected empty displayName without youtube service, got=%q", got)
	}
}

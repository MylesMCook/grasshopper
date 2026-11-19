package service

import (
	"net/http"
	"strings"

	"github.com/MylesMCook/grasshopper/internal/memory"
)

// Owner endpoints for areas (docs/adr/0003-faceted-classification.md). The
// owner creates areas, chooses the areas each project draws on, and sets a
// memory's area; agents only propose areas when they save.

// areaRequestError reports validation failures in the owner's words and
// hides storage errors.
func areaRequestError(w http.ResponseWriter, err error) {
	message := err.Error()
	switch {
	case strings.HasPrefix(message, "unknown_area"), strings.HasPrefix(message, "invalid"), strings.HasPrefix(message, "area required"):
		http.Error(w, message, http.StatusBadRequest)
	case message == "memory_not_found":
		http.Error(w, "memory not found", http.StatusNotFound)
	default:
		http.Error(w, "areas unavailable", http.StatusServiceUnavailable)
	}
}

// visualizerAreas lists the owner's areas and, for one project, the areas it
// draws on.
func visualizerAreas(store *memory.Writer) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var input struct {
			Project *string `json:"project"`
		}
		if !decodeVisualizerRead(w, r, &input) {
			return
		}
		areas, err := store.Areas(r.Context())
		projectAreas := []string{}
		if err == nil && input.Project != nil {
			projectAreas, err = store.ProjectAreas(r.Context(), *input.Project)
		}
		if err != nil {
			areaRequestError(w, err)
			return
		}
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		writeOwnerJSON(w, r, store, struct {
			Areas        []string `json:"areas"`
			ProjectAreas []string `json:"project_areas"`
		}{areas, projectAreas})
	}
}

func visualizerAreaCreate(store *memory.Writer, audit *auditLog) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var input struct {
			Name string `json:"name"`
		}
		if !decodeVisualizerRead(w, r, &input) {
			return
		}
		if err := store.CreateArea(r.Context(), input.Name); err != nil {
			areaRequestError(w, err)
			return
		}
		audit.event("owner_write", r, auditFields{action: "create_area"})
		w.WriteHeader(http.StatusNoContent)
	}
}

func visualizerProjectAreas(store *memory.Writer, audit *auditLog) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var input struct {
			Project string   `json:"project"`
			Areas   []string `json:"areas"`
		}
		if !decodeVisualizerRead(w, r, &input) {
			return
		}
		if err := store.SetProjectAreas(r.Context(), input.Project, input.Areas); err != nil {
			areaRequestError(w, err)
			return
		}
		audit.event("owner_write", r, auditFields{action: "project_areas"})
		w.WriteHeader(http.StatusNoContent)
	}
}

func visualizerMemoryArea(store *memory.Writer, audit *auditLog) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var input struct {
			ID         int64   `json:"id"`
			Area       *string `json:"area"`
			Everywhere *bool   `json:"everywhere"`
		}
		if !decodeVisualizerRead(w, r, &input) {
			return
		}
		if input.ID < 1 || (input.Area == nil && input.Everywhere == nil) {
			http.Error(w, "invalid area request", http.StatusBadRequest)
			return
		}
		if err := store.SetMemoryFacet(r.Context(), input.ID, input.Area, input.Everywhere); err != nil {
			areaRequestError(w, err)
			return
		}
		audit.event("owner_write", r, auditFields{action: "memory_area", id: input.ID})
		w.WriteHeader(http.StatusNoContent)
	}
}

package api

import (
	"math"
	"net/http"
	"strconv"

	"github.com/ogredforman/waypoint/internal/api/apitypes"
	"github.com/ogredforman/waypoint/internal/domain"
	"github.com/ogredforman/waypoint/internal/store"
)

const (
	defaultEntryPage = 20
	maxEntryPage     = 100
)

// listEntries pages through entries, newest first. With project_id= it lists
// the entries about that project: those scoped to it, and unscoped ones with
// an observation linked to it or its tasks or blockers. before= continues
// from an earlier page's next_before; limit= sets the page size.
func (h *handlers) listEntries(w http.ResponseWriter, r *http.Request) {
	projectID, err := queryID(r, "project_id")
	if err != nil {
		h.fail(w, r, err)
		return
	}
	before, err := queryID(r, "before")
	if err != nil {
		h.fail(w, r, err)
		return
	}
	limit := defaultEntryPage
	if s := r.URL.Query().Get("limit"); s != "" {
		n, err := strconv.Atoi(s)
		if err != nil || n < 1 || n > maxEntryPage {
			h.fail(w, r, badRequestf("query parameter limit must be between 1 and %d", maxEntryPage))
			return
		}
		limit = n
	}
	beforeID := int64(math.MaxInt64)
	if before != nil {
		beforeID = *before
	}

	var entries []domain.Entry
	if projectID == nil {
		entries, err = h.store.ListEntriesBefore(r.Context(), beforeID, limit)
	} else {
		if _, err := h.store.GetProject(r.Context(), *projectID); err != nil {
			h.fail(w, r, err)
			return
		}
		entries, err = h.store.ListEntriesBeforeForProject(r.Context(), *projectID, beforeID, limit)
	}
	if err != nil {
		h.fail(w, r, err)
		return
	}
	page := apitypes.EntryPage{Entries: mapSlice(entries, entryDTO)}
	if len(entries) == limit {
		next := entries[len(entries)-1].ID
		page.NextBefore = &next
	}
	writeJSON(w, http.StatusOK, page)
}

// createEntry takes an apitypes.CreateEntryRequest, stores the entry, and
// queues its extraction. It answers 202: the outcome arrives over SSE.
func (h *handlers) createEntry(w http.ResponseWriter, r *http.Request) {
	var req apitypes.CreateEntryRequest
	if err := readJSON(w, r, &req); err != nil {
		h.fail(w, r, err)
		return
	}
	e, run, err := h.worker.SaveEntry(r.Context(), domain.Entry{
		Body:              req.Body,
		ProjectID:         req.ProjectID,
		ReplyToQuestionID: req.ReplyToQuestionID,
	})
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusAccepted, apitypes.CreateEntryResponse{Entry: entryDTO(e), Run: runDTO(run)})
}

func (h *handlers) getEntry(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	if err != nil {
		h.fail(w, r, err)
		return
	}
	ctx := r.Context()
	var d apitypes.EntryDetail
	err = h.store.InReadTx(ctx, func(tx *store.ReadTx) error {
		e, err := tx.GetEntry(ctx, id)
		if err != nil {
			return err
		}
		d.Entry = entryDTO(e)

		runs, err := tx.ListExtractionRunsByEntry(ctx, id)
		if err != nil {
			return err
		}
		if len(runs) > 0 {
			run := runDTO(runs[len(runs)-1])
			d.Run = &run
		}

		all, err := tx.ListObservationsByEntry(ctx, id)
		if err != nil {
			return err
		}
		d.Observations = []apitypes.Observation{}
		for _, o := range domain.LatestRun(all) {
			links, err := tx.ListLinksByObservation(ctx, o.ID)
			if err != nil {
				return err
			}
			d.Observations = append(d.Observations, apitypes.Observation{
				ID:        o.ID,
				Kind:      o.Kind,
				Summary:   o.Summary,
				SpanStart: utf16Offset(e.Body, o.SpanStart),
				SpanEnd:   utf16Offset(e.Body, o.SpanEnd),
				Links: mapSlice(links, func(l domain.ObservationLink) apitypes.ObservationLink {
					return apitypes.ObservationLink{TargetType: l.TargetType, TargetID: l.TargetID, Confidence: l.Confidence}
				}),
			})
		}

		questions, err := tx.ListQuestionsByEntry(ctx, id)
		if err != nil {
			return err
		}
		d.Questions = mapSlice(questions, questionDTO)

		changesets, err := tx.ListChangesetsByEntry(ctx, id)
		if err != nil {
			return err
		}
		d.Changesets = make([]apitypes.Changeset, len(changesets))
		for i, cs := range changesets {
			if d.Changesets[i], err = changesetDTO(ctx, tx, cs); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, d)
}

// reprocessEntry queues a new extraction run for the entry. The entry's
// pending changesets are rejected and its open questions dismissed; the UI
// warns about that first.
func (h *handlers) reprocessEntry(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	if err != nil {
		h.fail(w, r, err)
		return
	}
	run, err := h.worker.Reprocess(r.Context(), id)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusAccepted, runDTO(run))
}

func (h *handlers) dismissQuestion(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	if err != nil {
		h.fail(w, r, err)
		return
	}
	var q domain.FollowUpQuestion
	err = h.store.InTx(r.Context(), func(tx *store.Tx) error {
		var err error
		q, err = tx.DismissQuestion(r.Context(), id)
		return err
	})
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, questionDTO(q))
}

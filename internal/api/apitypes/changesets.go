package apitypes

import (
	"encoding/json"
	"time"

	"github.com/ogredforman/waypoint/internal/domain"
)

// Changeset is a group of proposed ops, with its ops in position order.
type Changeset struct {
	ID     int64                  `json:"id"`
	Source domain.ChangesetSource `json:"source"`
	// EntryID is the entry an extraction changeset came from; null for a
	// manual one.
	EntryID           *int64                 `json:"entry_id" tstype:"number | null,required"`
	ParentChangesetID *int64                 `json:"parent_changeset_id" tstype:"number | null,required"`
	Status            domain.ChangesetStatus `json:"status"`
	Ops               []ChangeOp             `json:"ops"`
	CreatedAt         time.Time              `json:"created_at"`
	ResolvedAt        *time.Time             `json:"resolved_at" tstype:"string | null,required"`
}

// ChangeOp is one proposed mutation. Payload's shape depends on OpType (SPEC
// §6.2). AppliedPayload is what was applied instead, if the user edited it.
// Refs names the existing rows the payload refers to, keyed "kind:id" (kind
// is project, task, blocker, or idea), so the review UI can say what an op
// touches; rows that no longer exist are left out.
type ChangeOp struct {
	ID             int64                 `json:"id"`
	Position       int64                 `json:"position"`
	OpType         domain.OpType         `json:"op_type"`
	Payload        json.RawMessage       `json:"payload" tstype:"{ [key: string]: unknown }"`
	Rationale      *string               `json:"rationale" tstype:"string | null,required"`
	Confidence     *float64              `json:"confidence" tstype:"number | null,required"`
	Status         domain.ChangeOpStatus `json:"status"`
	AppliedPayload json.RawMessage       `json:"applied_payload" tstype:"{ [key: string]: unknown } | null"`
	Refs           map[string]string     `json:"refs" tstype:"{ [key: string]: string }"`
}

// Decision is the user's call on one op.
type Decision string

const (
	DecisionAccept Decision = "accept"
	DecisionReject Decision = "reject"
	DecisionEdit   Decision = "edit"
)

// ApplyRequest is the body of POST /api/changesets/{id}/apply. Ops left out
// stay pending in a changeset split off this one.
type ApplyRequest struct {
	Ops []OpDecision `json:"ops"`
}

// OpDecision decides op ID. Payload is the edited payload, required for
// edit and not allowed otherwise.
type OpDecision struct {
	ID       int64           `json:"id"`
	Decision Decision        `json:"decision"`
	Payload  json.RawMessage `json:"payload,omitempty" tstype:"{ [key: string]: unknown }"`
}

// ApplyResponse is what applying did.
type ApplyResponse struct {
	Changeset Changeset `json:"changeset"`
	// SplitOff is the new pending changeset holding the undecided ops and
	// those held back by the active-project cap, or null if there were none.
	SplitOff *Changeset `json:"split_off" tstype:"Changeset | null,required"`
	// HeldByCap lists the IDs of the ops in SplitOff that were accepted but
	// would have gone over the active-project cap.
	HeldByCap []int64 `json:"held_by_cap"`
}

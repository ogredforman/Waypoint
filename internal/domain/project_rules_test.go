package domain

import (
	"errors"
	"slices"
	"testing"
)

func TestCheckProjectStatusChange(t *testing.T) {
	const capN = 3
	for _, from := range ProjectStatuses() {
		for _, to := range ProjectStatuses() {
			// Under the cap every change is allowed.
			if err := CheckProjectStatusChange(from, to, capN-1, capN); err != nil {
				t.Errorf("%s -> %s under cap: %v", from, to, err)
			}

			// At and over the cap, only becoming active is refused.
			for _, active := range []int{capN, capN + 2} {
				err := CheckProjectStatusChange(from, to, active, capN)
				refused := from != ProjectActive && to == ProjectActive
				if !refused {
					if err != nil {
						t.Errorf("%s -> %s with %d active: %v", from, to, active, err)
					}
					continue
				}
				var capErr *ActiveCapError
				if !errors.As(err, &capErr) {
					t.Fatalf("%s -> %s with %d active: got %v, want *ActiveCapError", from, to, active, err)
				}
				if capErr.Cap != capN || capErr.Active != active {
					t.Errorf("ActiveCapError = %+v, want cap %d active %d", capErr, capN, active)
				}
			}
		}
	}
}

func TestCheckProjectStatusChangeInvalid(t *testing.T) {
	err := CheckProjectStatusChange("archived", "gone", 0, 3)
	if got := fields(t, err); !slices.Equal(got, []string{"from", "to"}) {
		t.Errorf("problem fields %v, want [from to]", got)
	}
	if err := CheckProjectStatusChange(ProjectParked, ProjectActive, 0, 0); err == nil {
		t.Error("cap 0: want error, got nil")
	}
}

func TestCheckNewProjectStatus(t *testing.T) {
	tests := []struct {
		status  ProjectStatus
		active  int
		wantErr string // "", "cap", or "invalid"
	}{
		{ProjectParked, 0, ""},
		{ProjectParked, 5, ""},
		{ProjectActive, 2, ""},
		{ProjectActive, 3, "cap"},
		{ProjectActive, 4, "cap"},
		{ProjectPaused, 0, "invalid"},
		{ProjectDone, 0, "invalid"},
		{"bogus", 0, "invalid"},
	}
	for _, tt := range tests {
		err := CheckNewProjectStatus(tt.status, tt.active, 3)
		var capErr *ActiveCapError
		var ve *ValidationError
		switch tt.wantErr {
		case "":
			if err != nil {
				t.Errorf("%s with %d active: %v", tt.status, tt.active, err)
			}
		case "cap":
			if !errors.As(err, &capErr) {
				t.Errorf("%s with %d active: got %v, want *ActiveCapError", tt.status, tt.active, err)
			}
		case "invalid":
			if !errors.As(err, &ve) {
				t.Errorf("%s: got %v, want *ValidationError", tt.status, err)
			}
		}
	}
}

func TestActiveCapErrorMessage(t *testing.T) {
	err := &ActiveCapError{Cap: 3, Active: 3}
	if got, want := err.Error(), "3 of 3 active projects; pause or park one to make room"; got != want {
		t.Errorf("Error() = %q, want %q", got, want)
	}
}

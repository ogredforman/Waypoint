package domain

import "fmt"

// Project status rules (SPEC §5, §8). Any status may change to any other; the
// only limit is the active-project cap, which applies when a project becomes
// active. Projects already active stay active if the cap is lowered below
// their count, and edits that keep a project active always pass.

// DefaultActiveProjectCap is the active-project cap when none is configured.
const DefaultActiveProjectCap = 3

// ActiveCapError means a project can't become active because the cap is
// reached. Callers turn it into a gentle prompt to pause or park a project.
type ActiveCapError struct {
	Cap    int // the configured cap
	Active int // projects active right now
}

func (e *ActiveCapError) Error() string {
	return fmt.Sprintf("%d of %d active projects; pause or park one to make room", e.Active, e.Cap)
}

// CheckProjectStatusChange reports whether a project may move from one status
// to another, given how many projects are active now (activeCount) and the cap (limit).
func CheckProjectStatusChange(from, to ProjectStatus, activeCount, limit int) error {
	var c checker
	c.enum("from", from.Valid())
	c.enum("to", to.Valid())
	if err := c.err("project status change"); err != nil {
		return err
	}
	if from == ProjectActive {
		return nil
	}
	return checkRoomForActive(to, activeCount, limit)
}

// CheckNewProjectStatus reports whether a new project may start with status s.
// A manually created project may start active (if there's room) or parked;
// starting paused or done isn't meaningful. Projects proposed by extraction
// (create_project) are always parked (SPEC §6.2); the op payload enforces that.
func CheckNewProjectStatus(s ProjectStatus, activeCount, limit int) error {
	if s != ProjectActive && s != ProjectParked {
		var c checker
		c.add("status", "a new project starts active or parked")
		return c.err("project")
	}
	return checkRoomForActive(s, activeCount, limit)
}

func checkRoomForActive(to ProjectStatus, activeCount, limit int) error {
	if to != ProjectActive {
		return nil
	}
	if limit < 1 {
		return fmt.Errorf("active project cap must be at least 1, got %d", limit)
	}
	if activeCount >= limit {
		return &ActiveCapError{Cap: limit, Active: activeCount}
	}
	return nil
}

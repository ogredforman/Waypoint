package domain

import "slices"

// Each enum is a typed string whose values match a CHECK (... IN ...) list in
// the migrations. The plural function returns every value in the SQL order; a
// test in internal/db fails if the two drift apart.

// ProjectStatus is the lifecycle state of a project. Only active projects count
// toward the active-project cap.
type ProjectStatus string

const (
	ProjectActive ProjectStatus = "active"
	ProjectPaused ProjectStatus = "paused"
	ProjectParked ProjectStatus = "parked"
	ProjectDone   ProjectStatus = "done"
)

func ProjectStatuses() []ProjectStatus {
	return []ProjectStatus{ProjectActive, ProjectPaused, ProjectParked, ProjectDone}
}

func (s ProjectStatus) Valid() bool { return slices.Contains(ProjectStatuses(), s) }

// MilestoneStatus is the lifecycle state of a milestone (post-MVP).
type MilestoneStatus string

const (
	MilestoneOpen    MilestoneStatus = "open"
	MilestoneDone    MilestoneStatus = "done"
	MilestoneDropped MilestoneStatus = "dropped"
)

func MilestoneStatuses() []MilestoneStatus {
	return []MilestoneStatus{MilestoneOpen, MilestoneDone, MilestoneDropped}
}

func (s MilestoneStatus) Valid() bool { return slices.Contains(MilestoneStatuses(), s) }

// TaskStatus is the lifecycle state of a task.
type TaskStatus string

const (
	TaskTodo       TaskStatus = "todo"
	TaskInProgress TaskStatus = "in_progress"
	TaskDone       TaskStatus = "done"
	TaskDropped    TaskStatus = "dropped"
)

func TaskStatuses() []TaskStatus {
	return []TaskStatus{TaskTodo, TaskInProgress, TaskDone, TaskDropped}
}

func (s TaskStatus) Valid() bool { return slices.Contains(TaskStatuses(), s) }

// TaskSize is a rough effort estimate for a task.
type TaskSize string

const (
	SizeXS TaskSize = "xs"
	SizeS  TaskSize = "s"
	SizeM  TaskSize = "m"
	SizeL  TaskSize = "l"
)

func TaskSizes() []TaskSize {
	return []TaskSize{SizeXS, SizeS, SizeM, SizeL}
}

func (s TaskSize) Valid() bool { return slices.Contains(TaskSizes(), s) }

// Energy is how much focus a task needs.
type Energy string

const (
	EnergyLow    Energy = "low"
	EnergyMedium Energy = "medium"
	EnergyDeep   Energy = "deep"
)

func Energies() []Energy {
	return []Energy{EnergyLow, EnergyMedium, EnergyDeep}
}

func (e Energy) Valid() bool { return slices.Contains(Energies(), e) }

// ExtractionRunStatus is the state of one extraction pass. Running runs are
// reset to pending on restart.
type ExtractionRunStatus string

const (
	RunPending   ExtractionRunStatus = "pending"
	RunRunning   ExtractionRunStatus = "running"
	RunSucceeded ExtractionRunStatus = "succeeded"
	RunFailed    ExtractionRunStatus = "failed"
)

func ExtractionRunStatuses() []ExtractionRunStatus {
	return []ExtractionRunStatus{RunPending, RunRunning, RunSucceeded, RunFailed}
}

func (s ExtractionRunStatus) Valid() bool { return slices.Contains(ExtractionRunStatuses(), s) }

// QuestionStatus is the state of a follow-up question.
type QuestionStatus string

const (
	QuestionOpen      QuestionStatus = "open"
	QuestionAnswered  QuestionStatus = "answered"
	QuestionDismissed QuestionStatus = "dismissed"
)

func QuestionStatuses() []QuestionStatus {
	return []QuestionStatus{QuestionOpen, QuestionAnswered, QuestionDismissed}
}

func (s QuestionStatus) Valid() bool { return slices.Contains(QuestionStatuses(), s) }

// ObservationKind is the type of fragment extracted from an entry.
type ObservationKind string

const (
	ObservationProgress ObservationKind = "progress"
	ObservationPlan     ObservationKind = "plan"
	ObservationBlocker  ObservationKind = "blocker"
	ObservationIdea     ObservationKind = "idea"
	ObservationFeeling  ObservationKind = "feeling"
	ObservationDecision ObservationKind = "decision"
	ObservationQuestion ObservationKind = "question"
	ObservationNote     ObservationKind = "note"
)

func ObservationKinds() []ObservationKind {
	return []ObservationKind{
		ObservationProgress, ObservationPlan, ObservationBlocker, ObservationIdea, ObservationFeeling,
		ObservationDecision, ObservationQuestion, ObservationNote,
	}
}

func (k ObservationKind) Valid() bool { return slices.Contains(ObservationKinds(), k) }

// TargetType names the table an observation link points into.
type TargetType string

const (
	TargetProject   TargetType = "project"
	TargetMilestone TargetType = "milestone"
	TargetTask      TargetType = "task"
	TargetBlocker   TargetType = "blocker"
	TargetIdea      TargetType = "idea"
)

func TargetTypes() []TargetType {
	return []TargetType{TargetProject, TargetMilestone, TargetTask, TargetBlocker, TargetIdea}
}

func (t TargetType) Valid() bool { return slices.Contains(TargetTypes(), t) }

// BlockerStatus is the lifecycle state of a blocker.
type BlockerStatus string

const (
	BlockerOpen     BlockerStatus = "open"
	BlockerResolved BlockerStatus = "resolved"
)

func BlockerStatuses() []BlockerStatus {
	return []BlockerStatus{BlockerOpen, BlockerResolved}
}

func (s BlockerStatus) Valid() bool { return slices.Contains(BlockerStatuses(), s) }

// IdeaStatus is the lifecycle state of a parking-lot idea.
type IdeaStatus string

const (
	IdeaParked    IdeaStatus = "parked"
	IdeaPromoted  IdeaStatus = "promoted"
	IdeaDiscarded IdeaStatus = "discarded"
)

func IdeaStatuses() []IdeaStatus {
	return []IdeaStatus{IdeaParked, IdeaPromoted, IdeaDiscarded}
}

func (s IdeaStatus) Valid() bool { return slices.Contains(IdeaStatuses(), s) }

// ChangesetSource says where a changeset came from.
type ChangesetSource string

const (
	SourceExtraction ChangesetSource = "extraction"
	SourceManual     ChangesetSource = "manual"
)

func ChangesetSources() []ChangesetSource {
	return []ChangesetSource{SourceExtraction, SourceManual}
}

func (s ChangesetSource) Valid() bool { return slices.Contains(ChangesetSources(), s) }

// ChangesetStatus is the review state of a changeset.
type ChangesetStatus string

const (
	ChangesetPending          ChangesetStatus = "pending"
	ChangesetApplied          ChangesetStatus = "applied"
	ChangesetPartiallyApplied ChangesetStatus = "partially_applied"
	ChangesetRejected         ChangesetStatus = "rejected"
)

func ChangesetStatuses() []ChangesetStatus {
	return []ChangesetStatus{
		ChangesetPending, ChangesetApplied, ChangesetPartiallyApplied, ChangesetRejected,
	}
}

func (s ChangesetStatus) Valid() bool { return slices.Contains(ChangesetStatuses(), s) }

// OpType is the kind of mutation a change op proposes (SPEC §6.2).
type OpType string

const (
	OpCreateTask     OpType = "create_task"
	OpUpdateTask     OpType = "update_task"
	OpSetTaskStatus  OpType = "set_task_status"
	OpSplitTask      OpType = "split_task"
	OpAddDependency  OpType = "add_dependency"
	OpDeleteTask     OpType = "delete_task"
	OpCreateBlocker  OpType = "create_blocker"
	OpUpdateBlocker  OpType = "update_blocker"
	OpResolveBlocker OpType = "resolve_blocker"
	OpDeleteBlocker  OpType = "delete_blocker"
	OpParkIdea       OpType = "park_idea"
	OpUpdateIdea     OpType = "update_idea"
	OpDeleteIdea     OpType = "delete_idea"
	OpCreateProject  OpType = "create_project"
	OpUpdateProject  OpType = "update_project"
)

func OpTypes() []OpType {
	return []OpType{
		OpCreateTask, OpUpdateTask, OpSetTaskStatus, OpSplitTask, OpAddDependency, OpDeleteTask,
		OpCreateBlocker, OpUpdateBlocker, OpResolveBlocker, OpDeleteBlocker,
		OpParkIdea, OpUpdateIdea, OpDeleteIdea,
		OpCreateProject, OpUpdateProject,
	}
}

func (t OpType) Valid() bool { return slices.Contains(OpTypes(), t) }

// ChangeOpStatus is the review state of a single change op.
type ChangeOpStatus string

const (
	OpPending  ChangeOpStatus = "pending"
	OpApplied  ChangeOpStatus = "applied"
	OpRejected ChangeOpStatus = "rejected"
	OpEdited   ChangeOpStatus = "edited"
)

func ChangeOpStatuses() []ChangeOpStatus {
	return []ChangeOpStatus{OpPending, OpApplied, OpRejected, OpEdited}
}

func (s ChangeOpStatus) Valid() bool { return slices.Contains(ChangeOpStatuses(), s) }

// BriefingSource says which briefer produced a briefing.
type BriefingSource string

const (
	BriefingHeuristic BriefingSource = "heuristic"
	BriefingLLM       BriefingSource = "llm"
)

func BriefingSources() []BriefingSource {
	return []BriefingSource{BriefingHeuristic, BriefingLLM}
}

func (s BriefingSource) Valid() bool { return slices.Contains(BriefingSources(), s) }

// InterfaceFont is the font of the app's interface, one of the fonts bundled
// with the frontend (SPEC §10.5).
type InterfaceFont string

const (
	FontInter                    InterfaceFont = "inter"
	FontGeist                    InterfaceFont = "geist"
	FontManrope                  InterfaceFont = "manrope"
	FontIBMPlexSans              InterfaceFont = "ibm-plex-sans"
	FontAtkinsonHyperlegibleNext InterfaceFont = "atkinson-hyperlegible-next"
)

func InterfaceFonts() []InterfaceFont {
	return []InterfaceFont{FontInter, FontGeist, FontManrope, FontIBMPlexSans, FontAtkinsonHyperlegibleNext}
}

func (f InterfaceFont) Valid() bool { return slices.Contains(InterfaceFonts(), f) }

// DiaryFont is the font diary entries are written and read in: Literata, a
// serif, or any interface font.
type DiaryFont string

// DiaryLiterata is the default diary font; the others share their values with
// InterfaceFont.
const DiaryLiterata DiaryFont = "literata"

func DiaryFonts() []DiaryFont {
	out := []DiaryFont{DiaryLiterata}
	for _, f := range InterfaceFonts() {
		out = append(out, DiaryFont(f))
	}
	return out
}

func (f DiaryFont) Valid() bool { return slices.Contains(DiaryFonts(), f) }

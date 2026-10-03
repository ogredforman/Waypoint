# Architecture Decision Records

One file per significant decision: `NNNN-short-title.md`, numbered in order. Copy `0000-template.md`.

Write an ADR when a choice is hard to reverse, adds a significant dependency, or changes something
the SPEC states. If the decision changes the SPEC, update `docs/SPEC.md` in the same commit.

ADRs are never deleted. A reversed decision gets a new ADR that supersedes the old one, and the
old one's status is changed to `Superseded by NNNN`.

| # | Title | Status |
|---|---|---|
| [0001](./0001-api-types.md) | API type sharing between Go and the frontend | Accepted |
| [0002](./0002-deleting-tasks-blockers-ideas.md) | Deleting tasks, blockers, and ideas | Accepted |
| [0003](./0003-splitting-changesets.md) | Splitting a changeset when it is applied | Accepted |
| [0004](./0004-diary-panel-layout.md) | The diary as a panel beside every screen | Accepted |
| [0005](./0005-profiles.md) | Profiles: one database per profile | Accepted |
| [0006](./0006-archived-projects.md) | Archived projects: a fifth project status | Accepted |

---
status: proposed
date: 2026-10-13
decision-makers: Nathan Zender
---

# Record architecture decisions as MADR files in `docs/adr`

## Context and Problem Statement

pr-riskinator is a change-management control that gates pull requests. SOC 2 and ISO 27001 auditors will ask why the control works the way it does, and future contributors will need to know which constraints are deliberate. Where do we record design decisions so they are reviewable, versioned, and auditable?

## Decision Drivers

- Decisions must be reviewable through the same pull request process as code.
- Auditors need a stable, dated record of why the control is designed this way.
- Low ceremony: writing a decision record should take minutes, not hours.

## Considered Options

1. Markdown ADRs in the repository using the MADR 4 structure
2. A wiki or external document tool
3. No formal record; rely on PR descriptions and commit messages

## Decision Outcome

Chosen option: "Markdown ADRs in the repository using the MADR 4 structure", because decisions then live next to the code they constrain, change only through reviewed pull requests, and carry git history as an audit trail.

Conventions:

- Location: `docs/adr/NNNN-kebab-case-title.md`, numbered sequentially.
- Template: `docs/adr/template.md`.
- Status values: `proposed`, `accepted`, `rejected`, `deprecated`, `superseded by NNNN`.
- An accepted ADR is not rewritten. A changed decision gets a new ADR that supersedes the old one, and the old one's status is updated to point at it.
- `docs/adr/README.md` holds the index and the list of open decisions.

### Consequences

- Good, because the design rationale is versioned with the code and visible in review.
- Good, because auditors can be pointed at a single directory.
- Bad, because ADRs can drift from the implementation if nobody updates their status. The index review in each release mitigates this.

### Confirmation

Every pull request that changes a documented decision either references an existing ADR or adds a new one. Reviewers check this.

## More Information

- MADR: <https://adr.github.io/madr/>

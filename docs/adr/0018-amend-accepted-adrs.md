---
status: accepted
date: 2026-11-07
decision-makers: Nathan Zender
---

# Amend part of an accepted ADR with an `amended by NNNN` status

Amends ADR 0001 (status values and the no-rewrite rule).

## Context and Problem Statement

ADR 0001 allows the status values `proposed`, `accepted`, `rejected`, `deprecated`, and `superseded by NNNN`, and says an accepted ADR is not rewritten. Supersession replaces a whole ADR. Several changes touch only one decision or one section of a larger ADR: ADR 0017 replaced decision 4 of ADR 0005, and the per-field weakening rule (ADR 0019) replaces one bullet of ADR 0010. ADR 0005 was handled ad hoc: its status stayed `accepted`, a note was added to its body, and the index said "decision 4 superseded by 0017". How do we record that part of an accepted ADR has changed, so the record stays auditable and a reader can tell which text is in force?

## Decision Drivers

- Auditors and contributors must be able to tell, from the amended ADR alone, that part of it no longer applies and where the replacement lives.
- The original text stays readable for history (ADR 0001).
- Low ceremony: a one-section change should not force a full rewrite of a long ADR.
- One consistent mechanism instead of ad hoc notes.

## Considered Options

1. Supersede the whole ADR every time, restating the parts that still hold
2. Add an `amended by NNNN` status, with a pointer note on each amended section
3. Leave the status alone and add an "Amended by" line under More Information

## Decision Outcome

Chosen option: "Add an `amended by NNNN` status", because it marks partial change in the same place readers already look for full supersession (the status), without duplicating text that still holds.

Rules:

- **Amending ADR.** A new ADR that changes part of an accepted ADR says so in a line under its title, naming the ADR and the exact decisions or sections it replaces, for example "Amends ADR 0010 (Merge semantics, third bullet)." Its Decision Outcome states the replacement text in full.
- **Amended ADR status.** The amended ADR's status becomes `accepted; amended by NNNN`. Further amendments append to the list: `accepted; amended by NNNN, MMMM`.
- **Amended ADR body.** A one-line note is inserted at the start of each amended section: "Amended by ADR NNNN: <one-sentence summary>. The original text is kept below for history." This note and the status are the only edits allowed to an accepted ADR. The original text is not changed.
- **Precedence.** Where an amending ADR and the amended ADR disagree, the amending ADR applies to the sections it names. Everything else in the amended ADR stays in force.
- **When to supersede instead.** If an amendment would replace most of an ADR's decision, or a reader would need several amending ADRs to reconstruct the current decision, supersede the ADR in full instead.
- **Index.** `docs/adr/README.md` shows the combined status, for example `accepted; amended by 0019`.

The status values from ADR 0001 become: `proposed`, `accepted`, `accepted; amended by NNNN`, `rejected`, `deprecated`, `superseded by NNNN`.

ADR 0005 is brought in line: its status becomes `accepted; amended by 0017`. Its existing in-body note already satisfies the body rule.

### Consequences

- Good, because the status line shows at a glance that part of an ADR has changed and where to look.
- Good, because long ADRs are not duplicated to change one rule.
- Good, because the existing ADR 0005 case now follows a written rule.
- Bad, because the current decision can be spread across more than one file. The "when to supersede instead" rule limits this.
- Bad, because inserting notes is an edit to an accepted ADR, a narrow exception to ADR 0001's no-rewrite rule.

### Confirmation

- Reviewers check, on any PR that adds an amending ADR, that the amended ADR's status, section notes, and index row are updated in the same PR (CONTRIBUTING: "A change that alters a decision adds or supersedes an ADR in the same PR").
- The index review in each release (ADR 0001) checks that every `amended by` status points at an existing ADR that names the amended sections.

## Pros and Cons of the Options

### Supersede the whole ADR every time

- Good, because it needs no change to ADR 0001.
- Good, because the current decision is always in one file.
- Bad, because a one-line change to a 90-line ADR duplicates 90 lines, and the duplicate can drift from the original.
- Bad, because the history of which part changed is buried in a diff between two files.

### Add an `amended by NNNN` status

- Good, because partial change is visible in the status, where readers already look.
- Good, because only the changed part is restated.
- Bad, because the current decision can span files.

### "Amended by" line under More Information only

- Good, because it touches the least text.
- Bad, because a reader who stops at the status line or the amended section never sees it. That is the failure an auditor would flag.

## More Information

- ADR 0001: original status values and the no-rewrite rule.
- ADR 0017: the first partial supersession (decision 4 of ADR 0005).
- ADR 0019: the first amendment written under this rule.

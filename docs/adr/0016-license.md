---
status: accepted
date: 2026-10-13
decision-makers: Nathan Zender
---

# License: Apache-2.0

## Context and Problem Statement

priskinator is planned for use by other organizations, each running its own instance (ADR 0014). Its likely adopters are compliance-minded organizations whose legal teams review licenses before adoption, and contributions may come from several companies. Under which license is the project distributed?

## Decision Drivers

- Low friction for enterprise adoption and legal review.
- Clear licensing of incoming contributions without a contributor license agreement.
- Protection for users against patent claims from contributors.
- Compatibility with dependencies: palantir/go-githubapp (Apache-2.0) and ast-grep (MIT).
- No current goal of preventing third parties from offering the tool as a hosted service.

## Considered Options

1. MIT
2. Apache-2.0
3. AGPL-3.0
4. Functional Source License (FSL), converting to Apache-2.0 after two years

## Decision Outcome

Chosen option: "Apache-2.0", because it is permissive like MIT and adds an explicit patent grant with patent retaliation, plus default terms for contributions. Enterprise legal teams commonly prefer it for these reasons.

- The repository root carries the unmodified license text in `LICENSE`.
- A `NOTICE` file is added only if a bundled third-party component requires attribution there.
- Contributions are accepted under Apache-2.0 by default (section 5 of the license). No CLA.

### Consequences

- Good, because users receive a patent license from every contributor, and a party that sues over patents in the project loses its license.
- Good, because contribution licensing is clear without extra paperwork.
- Good, because it is compatible with the planned dependencies.
- Bad, because redistributors must carry notices and mark modified files, which is slightly more work than MIT.
- Bad, because nothing stops a third party from selling a hosted version. That is accepted. Revisit only together with a decision on a hosted offering (ADR 0014).

### Confirmation

- `LICENSE` matches the official text from <https://www.apache.org/licenses/LICENSE-2.0.txt>.
- Dependency license checks run in CI when code lands, rejecting licenses incompatible with Apache-2.0 distribution.

## Pros and Cons of the Options

### MIT

- Good, because it is short and simple, with minimal obligations.
- Bad, because it has no explicit patent grant and no contribution terms.

### Apache-2.0

- Good, because it adds a patent grant, patent retaliation, contribution terms, and a trademark clause.
- Bad, because the notice obligations are longer.

### AGPL-3.0

- Good, because anyone offering a modified version as a network service must publish their changes.
- Bad, because many enterprises prohibit AGPL dependencies, which conflicts with the adoption goal.

### FSL

- Good, because it blocks competing commercial use for two years, then becomes Apache-2.0.
- Bad, because it is not an OSI-approved open-source license during those two years, which narrows adoption.

# Audit Report -- NexusFlow
**Requirements Version Audited:** 1
**Date:** 2026-03-25
**Artifact Weight:** Blueprint
**Result:** ISSUES FOUND

## Summary
25 requirements audited (21 functional, 4 non-functional). 18 passed all five checks. 4 blocking issues found: 1 contradiction, 1 gap, 2 ambiguous. 3 deferred items tracked (non-blocking).

---

## Blocking Issues

### AUDIT-001: CONTRADICTION -- Manifest marks Designer "Skipped" but requirements demand web GUI
**Flag:** CONTRADICTION
**Requirements involved:** REQ-002, REQ-015, REQ-016, REQ-017, REQ-018
**Description:** The Methodology Manifest v1 marks the Designer agent as "Skipped" with the rationale "No user-facing UI -- this is a backend services system." However, the Brief declares the delivery channel as "Hybrid -- Web App + REST API," and five requirements specify web GUI functionality: task submission via GUI (REQ-002), Pipeline Builder with drag-and-drop (REQ-015), Worker Fleet Dashboard (REQ-016), Task Feed and Monitor (REQ-017), and Log Streamer (REQ-018). The Manifest and the requirements cannot both be correct. Either the GUI requirements are invalid, or the Designer agent must be re-activated. The Analyst flagged this in the Brief (Delivery Channel note); the Orchestrator flagged it as ESC-001. This audit formally confirms it as a blocking contradiction.
**Resolution needed:** Nexus decision -- the Manifest must be corrected to activate the Designer agent, or the GUI requirements must be removed. Given that the Nexus explicitly stated the web GUI in intake, the former is overwhelmingly likely to be the correct resolution.
**Nexus question:** The Methodology Manifest marks the Designer agent as "Skipped" because it assumed no user-facing UI, but your stated requirements include a web GUI with four views (Pipeline Builder, Worker Fleet Dashboard, Task Feed, Log Streamer). Should the Designer agent be re-activated to handle UX design for these views?

---

### AUDIT-002: GAP -- No requirement for pipeline CRUD operations
**Flag:** GAP
**Requirements involved:** (none -- that is the problem)
**Description:** The Brief's User Roles table states that Users can "Create/read/update own pipelines" and Admins can "manage pipelines." REQ-015 addresses visual pipeline construction in the GUI, but there is no requirement covering the full pipeline CRUD lifecycle: listing saved pipelines, editing an existing pipeline definition, deleting a pipeline, or managing pipelines via the REST API. REQ-001 references submitting a task with a "pipeline reference," implying pipelines are persistent named entities, but no requirement defines how they are created, stored, retrieved, updated, or deleted outside the drag-and-drop builder. The REST API surface for pipelines is entirely absent. This is not a deferrable need -- tasks cannot be submitted without pipelines existing, and the API channel has no way to manage them.
**Resolution needed:** Analyst must add a requirement (or requirements) covering pipeline CRUD operations for both the GUI and the REST API, including ownership and access control rules.

---

### AUDIT-003: AMBIGUOUS -- REQ-019/REQ-020 auth mechanism unspecified to the point of untestability
**Flag:** AMBIGUOUS
**Requirements involved:** REQ-019, REQ-020
**Description:** REQ-019 states "The system authenticates users" but does not specify the authentication mechanism. This is also listed as Open Context Question 5 in the Brief ("Is there an existing auth system to integrate with, or does NexusFlow manage its own credentials?"). The Definition of Done says "Unauthenticated requests are rejected" and "Role checks are enforced on all API endpoints and GUI views" -- these are testable at the authorization level, but the authentication path itself is not testable because the mechanism is unspecified. For a Blueprint-weight artifact, the Verifier needs to know whether to test against JWT tokens, session cookies, API keys, or an external SSO flow. Similarly, REQ-020 says admins can "create" user accounts but does not specify whether this means setting a password, sending an invite link, or provisioning through an external identity provider. The acceptance scenarios in REQ-019 test HTTP status codes (401, 403) but not the authentication flow itself. At Blueprint weight, this level of ambiguity blocks testability.
**Resolution needed:** The Nexus must answer Open Context Question 5 (authentication mechanism). Once answered, the Analyst should revise REQ-019 and REQ-020 to specify the auth mechanism and the user provisioning flow.
**Nexus question:** Does NexusFlow manage its own user credentials (username/password with sessions or tokens), or will it integrate with an external authentication system (OAuth2, SSO, LDAP)? This determines how REQ-019 and REQ-020 are specified and tested.

---

### AUDIT-004: AMBIGUOUS -- NFR-001 duplicates REQ-003 SLA target without clarifying scope
**Flag:** AMBIGUOUS
**Requirements involved:** REQ-003, NFR-001
**Description:** REQ-003 states "Queuing latency must be under 50ms at the 95th percentile" as part of its functional statement and Definition of Done. NFR-001 restates the same target: "Task queuing latency ... must be under 50ms at the 95th percentile." However, NFR-001 adds a load condition ("Under sustained load of 10,000 tasks/hour") that REQ-003 does not mention. This creates ambiguity: does the 50ms p95 target in REQ-003 apply at any load, or only under the sustained load specified in NFR-001? If these are the same requirement stated twice, one should reference the other. If they are different (functional correctness vs. performance under load), the distinction must be explicit. As written, a Verifier could reasonably test REQ-003 in isolation at low load and declare it passed, while NFR-001 fails under sustained load -- or vice versa.
**Resolution needed:** Analyst clarification -- REQ-003 should either reference NFR-001 for the latency SLA (removing the inline 50ms claim) or the two should be explicitly scoped to different conditions. This does not require Nexus input; the Analyst can resolve it.

---

## Deferred Items (non-blocking)

### AUDIT-005: DEFERRED -- Log retention policy
**Flag:** DEFERRED
**Brief reference:** Open Context Questions, item 1
**What is deferred:** The Brief asks "How long should task logs be retained? Is there a storage budget or TTL?" No requirement addresses log retention or cleanup.
**Why deferral is acceptable:** REQ-018 covers real-time log streaming, which is the core need. Log retention is a storage/operations concern that can be addressed when the Architect designs the logging subsystem or when operational experience reveals the storage impact. No downstream requirement depends on a retention policy being defined now.
**Resolve by:** Architecture Gate -- the Architect should include a log retention strategy (even if configurable) in the system design, or this escalates to a GAP.

---

### AUDIT-006: DEFERRED -- Pipeline template sharing between users
**Flag:** DEFERRED
**Brief reference:** Open Context Questions, item 3
**What is deferred:** The Brief asks "Can users share pipeline definitions with other users, or are pipelines strictly private?" No requirement addresses pipeline sharing.
**Why deferral is acceptable:** The current requirements treat pipelines as user-owned (REQ-015, User Roles table). Strict private ownership is a safe default for phase 1. Sharing is an additive feature that does not invalidate existing requirements if added later.
**Resolve by:** Before Cycle 2 planning -- if the Nexus wants sharing in phase 1, a requirement must be added. Otherwise, this is a future enhancement.

---

### AUDIT-007: DEFERRED -- Schema mapping validation timing
**Flag:** DEFERRED
**Brief reference:** Open Context Questions, item 6
**What is deferred:** The Brief asks whether schema mappings should be validated at pipeline definition time (design-time) or only at execution time. REQ-007 specifies runtime behavior (mapping applied at phase boundaries, error on missing field at execution) but is silent on design-time validation.
**Why deferral is acceptable:** REQ-007's acceptance scenarios test runtime behavior and are testable as written. Design-time validation is a UX enhancement that makes the Pipeline Builder more user-friendly but does not affect correctness -- a bad mapping will still fail at runtime with a clear error. The Architect can decide whether to include design-time validation as part of the Pipeline Builder design.
**Resolve by:** Architecture Gate -- the Architect should state whether design-time validation is included in the Pipeline Builder's scope. If included, the Analyst may need to add or revise REQ-007/REQ-015 acceptance scenarios.

---

## Observations (non-blocking, no flag)

**REQ-007 schema mapping depth (Analyst uncertainty flag):** The Analyst flagged uncertainty about how deep or complex schema mappings can be. REQ-007's acceptance scenarios test field renaming and missing-field error handling, which are testable and sufficient for phase 1. The depth of nesting (flat fields vs. nested objects vs. array transformations) is an architectural/implementation decision. The Architect can constrain this. No flag needed.

**REQ-008 atomic sink abstraction (Analyst uncertainty flag):** The Analyst flagged uncertainty about how atomicity is implemented across different sink types. REQ-008's Definition of Done and acceptance scenarios are testable: "after a forced Sink failure, the destination state is identical to its state before the Sink began." The mechanism (transactions, compensating writes, staging patterns) is an Architect decision. The requirement states the invariant; the Architect designs the mechanism. No flag needed.

**Heartbeat timeout threshold (Open Context Question 4):** No requirement hardcodes a specific timeout value. REQ-004 says "configured timeout" and the Brief suggests this may be deferred to the Architect as a configurable parameter. This is appropriate -- the Architect will set a default. No flag needed.

**Admin CRUD details (Open Context Question 2):** REQ-020 specifies "create, view, and deactivate" -- not full CRUD with delete. This is a reasonable conservative scope. The Brief asks whether admins can delete users who own tasks; REQ-020's acceptance scenario explicitly states "deactivation does not cancel in-flight tasks," which is a coherent position. The question of whether delete (vs. deactivate) is needed can be raised at demo. No flag needed.

**REQ-021 throughput and NFR-001 load condition:** REQ-021 (10,000 tasks/hour) and NFR-001 (50ms p95 under 10,000 tasks/hour) are consistent with each other. The issue noted in AUDIT-004 is about REQ-003's inline duplication, not about REQ-021/NFR-001 coherence.

---

## Passed Requirements

The following requirements cleared all five checks (consistency, completeness, coherence, traceability, testability):

REQ-001, REQ-002, REQ-004, REQ-005, REQ-006, REQ-007, REQ-008, REQ-009, REQ-010, REQ-011, REQ-012, REQ-013, REQ-014, REQ-016, REQ-017, REQ-018, REQ-021, NFR-002, NFR-003, NFR-004

**Note:** REQ-015 passes on four of five checks but is incomplete in the context of AUDIT-002 (pipeline CRUD gap). Its drag-and-drop pipeline construction is well-specified, but it does not cover the full pipeline lifecycle. Once AUDIT-002 is resolved, REQ-015 may need revised scope or a companion requirement.

REQ-003 passes on four of five checks but has a consistency issue with NFR-001 noted in AUDIT-004.

REQ-019 and REQ-020 are flagged in AUDIT-003 (ambiguous auth mechanism).

NFR-001 is flagged in AUDIT-004 (duplication/scope ambiguity with REQ-003).

---

## Recommendation

**RETURN TO ANALYST WITH NEXUS INPUT** -- two issues require Nexus decisions before the Analyst can revise:

1. **AUDIT-001** (Designer agent contradiction) -- requires Nexus confirmation to re-activate the Designer agent in the Manifest.
2. **AUDIT-003** (auth mechanism) -- requires Nexus answer to Open Context Question 5.

Two issues can be resolved by the Analyst without Nexus input:

3. **AUDIT-002** (pipeline CRUD gap) -- the Analyst can add the missing requirement(s).
4. **AUDIT-004** (REQ-003/NFR-001 duplication) -- the Analyst can clarify the relationship.

**Most critical Nexus question (surface first):** AUDIT-001 -- Designer agent re-activation. This is the highest-priority item because it affects the Manifest configuration and the entire downstream workflow for GUI-related requirements.

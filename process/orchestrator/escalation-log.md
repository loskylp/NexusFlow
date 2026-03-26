# Escalation Log

## ESC-001 -- 2026-03-25
**From:** Analyst | **Type:** Configuration contradiction
**Description:** Manifest v1 marks the Designer agent as "Skipped" with rationale "No user-facing UI -- this is a backend services system." However, the Nexus's stated requirements include a web GUI with four distinct views: Pipeline Builder (REQ-015), Worker Fleet Dashboard (REQ-016), Task Feed and Monitor (REQ-017), and Log Streamer (REQ-018). Additionally, REQ-002 requires task submission via web GUI. The Brief's Delivery Channel is "Hybrid -- Web App + REST API." The Designer agent must be re-activated.
**Decision:** Routed to Auditor -- expect the Auditor to formally flag this contradiction. After Auditor report, will surface to Nexus at the Requirements Gate along with a signal to the Methodologist to update the Manifest.
**Outcome:** Auditor confirmed as AUDIT-001. Nexus decided: re-activate the Designer agent. Methodologist dispatched to produce Manifest v2. Resolved.

## ESC-002 -- 2026-03-25
**From:** Auditor (AUDIT-003) | **Type:** Ambiguous requirement -- Nexus decision required
**Description:** REQ-019 and REQ-020 specify authentication and user management but do not specify the authentication mechanism. The Brief's Open Context Question 5 asks whether NexusFlow manages its own credentials or integrates with an external auth system. At Blueprint weight, this ambiguity blocks testability -- the Verifier cannot write auth tests without knowing the mechanism.
**Decision:** Surfaced to Nexus. Question: Does NexusFlow manage its own user credentials (username/password with sessions or tokens), or will it integrate with an external authentication system (OAuth2, SSO, LDAP)?
**Outcome:** Nexus decided: NexusFlow manages its own credentials (username/password with session tokens). No external auth. Analyst revised REQ-019 and REQ-020 in Requirements v3. Resolved.

## ESC-003 -- 2026-03-25
**From:** Nexus (at Requirements Gate) | **Type:** New requirements before gate approval
**Description:** Nexus requested four demo-infrastructure requirements be added before approving the Requirements Gate. These are explicitly not part of the core system but are needed to demonstrate NexusFlow to stakeholders in a Critical Profile demonstration without external cloud costs. Requirements: (1) Fake-S3 -- local S3-compatible storage (MinIO) pre-loaded with 100 sample files; (2) Mock-Postgres -- instance pre-populated with 10,000 rows of dirty data for ETL demo; (3) Sink-Inspector -- GUI tab to monitor output sinks and verify before-vs-after results; (4) Chaos Controller -- disturbance panel to kill workers or disconnect DB and demonstrate auto-recovery.
**Decision:** This is a mini Requirements Gate loop per Behavioral Principle 8. Routing to Analyst to draft demo requirements with proper IDs and acceptance scenarios, then to Auditor for validation, then re-present the gate to Nexus.
**Outcome:** Resolved. Analyst produced Requirements v4 (DEMO-001 through DEMO-004). Auditor audit v3 found one blocking issue (AUDIT-008: DEMO-003 traceability). Analyst corrected in Requirements v5. Auditor audit v4: PASS WITH DEFERRALS. All 31 requirements pass. Gate re-presented to Nexus.

## ESC-004 -- 2026-03-26
**From:** Nexus (at Architecture Gate) | **Type:** Architecture Gate revision -- foundational assumption changed
**Description:** Nexus directed two changes at the Architecture Gate: (1) Go replaces Node.js/TypeScript as the backend runtime, and (2) deployment targets nxlabs.cc infrastructure (187.124.233.130) with Traefik, Watchtower, and shared PostgreSQL. The Architect revised architecture to v2, updating ADR-004, ADR-005, ADR-006, ADR-007, ADR-008, and fitness functions v2 (FF-024, FF-025 added). The deployment model change is a foundational assumption change per the Orchestrator's backward impact check protocol. The Auditor is being dispatched with an explicit backward impact check instruction to verify no approved requirement acceptance scenarios are invalidated by the new deployment model.
**Decision:** Route to Auditor for architectural re-audit with backward impact check. If [INVALIDATED] flags found, route to Analyst before re-attempting the Architecture Gate.
**Outcome:** Resolved. Auditor re-audit v2 passed (no blocking issues, no invalidated requirements). Architecture Gate re-presented to Nexus and APPROVED (2026-03-26). AUDIT-006 (pipeline template sharing) closed as NOT APPLICABLE -- Nexus decided there will be no templates at all. Designer dispatched.

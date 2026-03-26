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

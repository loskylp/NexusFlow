# Routing Instruction

**To:** Analyst
**Phase:** INGESTION
**Task:** Conduct guided discovery with the Nexus to understand the NexusFlow task processing system -- ask questions to uncover requirements, constraints, and domain context before producing any artifacts.
**Return to:** Orchestrator when discovery is complete

---

## Required documents

| Document | Link | Why needed |
|---|---|---|
| Methodology Manifest v1 | [manifest-v1.md](../methodologist/manifest-v1.md) | Profile, artifact depth, documentation requirements, and Nexus intake note with the verbatim project description |

---

## Context

This is a greenfield project. The Nexus has described a microservices system for processing long-running asynchronous tasks with the following key elements:

- A Producer microservice that receives requests and deposits them into a message queue after validation
- Redis as a persistent message broker
- A fleet of Workers that monitor the queue and pull tasks based on capacity
- A monitoring layer with traceable task lifecycle per task
- An orchestrator component that detects downed workers and reassigns unfinished tasks
- Downstream teams depend on this system; SLA breaches occur if it fails

The Nexus intake note is preserved verbatim in the Manifest (Section: "Nexus Intake Note"). Use it as the starting point for discovery, but do not treat it as a complete requirements specification. Your job is to ask the Nexus 3-4 questions per turn to uncover what is missing, ambiguous, or assumed. Signal when discovery is complete.

Designer is skipped (no user-facing UI). The Architect will determine the implementation language and framework.

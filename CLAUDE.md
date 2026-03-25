# Nexus SDLC Project

## Agent Auto-Chaining

### Any agent → Orchestrator

When a Nexus SDLC agent completes and its output ends with a line of the form:

> **Next:** Invoke @nexus-orchestrator — ...

Automatically invoke @nexus-orchestrator as the next step without waiting for
explicit user instruction.

### Orchestrator → other agents

The Orchestrator is the control plane. When its output instructs invoking a
specific agent (e.g., "invoke @nexus-analyst", "route to @nexus-architect"),
automatically invoke that agent as the next step without waiting for explicit
user instruction.

The Orchestrator is responsible for knowing when to pause for Nexus approval.
If it says "invoke", follow it. If it says "awaiting Nexus approval" or
presents a gate checkpoint, stop and wait for the user.

### Do not auto-chain

Do not auto-chain between non-orchestrator agents. All routing flows through
the Orchestrator.

## Agent Response Relay

When a Nexus SDLC agent's response contains a question for the user, relay the
question **verbatim**. Do not rephrase, restructure, or summarize agent questions.
The agents are designed to ask questions in a specific way — reformatting changes
the kind of answer the user gives, which breaks the intake protocol.

If the agent's response contains a line starting with **Relay:**, that is an
instruction to you, not content for the user. Follow the relay instruction and
strip it from the output.

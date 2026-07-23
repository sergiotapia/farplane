# Farplane

Farplane is a control plane for AI agent computers. Members connect GitHub,
create Projects from repositories, and run Lanes as sandboxed computers.

## Language

### Tenancy and access

**Organization**:
The single install tenant. Members, secrets, GitHub App credentials, and the
Scratch Environment belong to one Organization.
_Avoid_: Tenant, workspace, team account

**Member**:
A person who belongs to an Organization.
_Avoid_: User (when you mean org membership), account

### GitHub and Projects

**Project**:
A Farplane binding to one GitHub repository inside an Organization. A Project
has zero or one Project Environment.
_Avoid_: Repo (when you mean the Farplane entity), app

**GitHub repository**:
The remote repository on GitHub that a Project binds to.
_Avoid_: Project (the Farplane entity is Project)

### Environments

**Project Environment**:
The Dockerfile for one Project. A Project has zero or one. Members can edit it
in the web app. When a Project has none, the Project Environment Generator
creates the first draft.
_Avoid_: Lane template, org template, Docker template (for a Project)

**Scratch Environment**:
The Organization-wide Dockerfile used by Scratch Lanes. There is one per
Organization. Members can edit it in the web app.
_Avoid_: Global template, org lane template, default template (prefer this name)

**Project Environment Generator**:
The AI discovery flow that clones a Project’s GitHub repository, explores its
requirements, and proposes a Project Environment Dockerfile.
_Avoid_: Setup agent, discovery job (when you mean this product flow)

**Discovery harness**:
The host-installed headless agent CLI chosen for Project Environment generation.
Selection requires a matching organization secret and a binary on the Farplane
host PATH (`omp`, `opencode`, `claude`, or `codex`).
_Avoid_: Lane agent (the harness inside a Lane computer), model

### Lanes

**Lane**:
A chat plus one runtime computer. A Lane is either a Project Lane or a Scratch
Lane.
_Avoid_: Session, agent, computer (the computer is the runtime behind the Lane)

**Project Lane**:
A Lane tied to one Project. It runs from that Project’s Project Environment
(snapshotted onto the Lane at create time).
_Avoid_: Repo lane

**Scratch Lane**:
A Lane with no Project. It runs from the Organization’s Scratch Environment
(snapshotted onto the Lane at create time). Used for open-ended exploration.
_Avoid_: Scratchpad lane, freeform lane

**Dockerfile snapshot**:
The Dockerfile text copied onto a Lane when the Lane is created. Later edits to
the Project Environment or Scratch Environment do not change existing Lanes.
_Avoid_: Image definition (unless you mean the built image)

### Runtime

**Runtime**:
The computer that runs a Lane (Docker host or Sprites.dev).
_Avoid_: Server, VM (unless you mean a specific backend)

**Agent harness**:
The coding-agent CLI inside the Lane computer (for example Claude Code or
Codex). Farplane runs harnesses with full sandbox permissions
(dangerously-skip-permissions or the harness equivalent).
_Avoid_: Agent (when you mean the CLI product), model

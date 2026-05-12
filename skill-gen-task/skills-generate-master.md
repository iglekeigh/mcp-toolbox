# Technical Reference: `skills-generate` Architecture & Implementation

The `skills-generate` command converts MCP Toolbox toolsets into standalone **Agent Skill** packages compatible with the [agentskills.io](https://agentskills.io) specification. It utilizes a registry-based architecture to introspect the codebase, establish verified connections, and generate self-documenting artifacts.

---

## 1. Architectural Pillars

### A. Discovery via Registry Pattern
The system avoids hardcoding data sources or tools by using a **Registry Pattern**.
*   **Self-Registration:** Every source and tool defines an `init()` function that registers a "Factory" (constructor) into a global map.
*   **Blank Imports:** `cmd/internal/imports.go` performs blank imports (e.g., `_ "internal/sources/postgres"`) to force the Go runtime to execute these `init()` blocks at startup, populating the registry automatically.

### B. Self-Documenting Manifests
Tools are intelligent and self-describing.
*   **The Manifest Interface:** Every tool implements a `Manifest()` method.
*   **ToolManifest:** This returns structured metadata (description, parameters, types, required fields) used to generate the final documentation without "guessing" tool behavior.

---

## 2. Command Interface

The command is registered in `cmd/root.go` and defined in `cmd/internal/skills/command.go`.

### Command Flags
| Flag | Purpose | Default |
|------|---------|---------|
| `--name` | Prefix for the generated skill/folder | (Required) |
| `--description` | Descriptive text for the skill frontmatter | (Required) |
| `--toolset` | Export a specific toolset | All toolsets |
| `--output-dir` | Target directory for artifacts | `skills` |
| `--invocation-mode` | How scripts run: `npx` (remote) or `binary` (local) | `npx` |
| `--toolbox-version` | Version of `@toolbox-sdk/server` for npx mode | Current version |

---

## 3. Detailed Execution Lifecycle

### Phase 1: Boot & Initialization
The command performs a "Full Boot" via `server.InitializeConfigs()`. It parses the YAML configuration and executes `sc.Initialize()` for every source.
*   **Live Verification:** For database sources, this attempts a network handshake (e.g., `Ping()`). This ensures the generated skill is backed by a working, accessible configuration.

### Phase 2: Tool Collection & Grouping
The `collectTools()` function groups tools by their toolset.
*   **Single Toolset:** If `--toolset` is used, only those tools are exported.
*   **Multi-Skill Generation:** If multiple toolsets exist in the config, the command generates multiple skills, one per toolset, using the naming pattern `<name>-<toolset>`.

### Phase 3: Directory Structuring
For each skill, a standardized folder is created:
```
<skill-name>/
├── SKILL.md          # Documentation & Frontmatter
├── assets/           # Local copies of YAML/JSON configs
└── scripts/          # Node.js wrapper scripts (one per tool)
```

### Phase 4: Asset Bundling
Original configuration files are copied to the `assets/` directory to ensure portability. The command converts absolute paths to relative paths so the skill can be moved across machines.

### Phase 5: Synthesis & Generation
*   **Script Generation:** Uses Go templates to create Node.js wrappers. These scripts detect host environments (Gemini CLI, Claude Code) and map platform-specific environment variables automatically.
*   **Markdown Generation:** Transforms `ToolManifest` objects into `SKILL.md`, generating formatted parameter tables and usage instructions.

---

## 4. Environment Intelligence (JS Wrappers)

The generated Node.js scripts in `scripts/` include logic to handle different AI platform contexts:
*   **Gemini CLI:** Detects `.env` files and loads variables.
*   **Claude Code:** Detects and maps `CLAUDE_PLUGIN_OPTION_` prefixes.
*   **Codex CI:** Sets specific user-agent metadata for telemetry.

---

## 5. Summary Table: Consistency Check
| Feature | Wiki Status | Architecture Status | Resolution |
|---------|-------------|---------------------|------------|
| **Registry** | Referenced in citations | Explained in depth | Pillar of Discovery |
| **Ping/Live Connect** | Implied via `Initialize` | Explicitly stated | Critical for verification |
| **Toolsets** | Mapping logic detailed | Grouping mentioned | Essential for grouping |
| **JS Environment** | Mapping mentioned | Logic explained | Key for portability |

---

## 6. Installation
Generated skills can be installed into the Gemini CLI directly:
```bash
gemini skills install ./skills/my-generated-skill
```
Alternatively, set the `--output-dir` to `~/.gemini/skills` to generate and install in one step.

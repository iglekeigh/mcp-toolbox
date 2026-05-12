# Implementation Guide: Skip DB Connections for Skills Generation

## Problem Statement

The `skills-generate` command currently requires database connections to function. It calls `server.InitializeConfigs()` in `collectTools()`, which initializes every `SourceConfig` by opening real DB connections and pinging them before initializing tools [1](#9-0) . However, skill generation only needs tool manifest data (name, description, parameters) — all derived purely from `ToolConfig` struct fields with no DB connection required [2](#9-1) .

## Solution: Context Flag Approach

Add a context flag `skipConnectionsKey` that sources check during `Initialize()` to skip DB operations. This approach:

- Requires no interface signature changes
- Follows existing patterns (similar to `instrumentationKey` in `util.go`) [3](#9-2)
- Only requires changes to 42 source implementations (no tool changes)
- Propagates the flag through the existing context chain

---

## Implementation Steps

### Step 1: Add Context Helpers to `internal/util/util.go`

Add the context key type and helper functions following the existing pattern:

```go
type skipConnectionsKey struct{}

// WithSkipConnections adds a flag to skip DB connections during initialization
func WithSkipConnections(ctx context.Context) context.Context {
    return context.WithValue(ctx, skipConnectionsKey{}, true)
}

// ShouldSkipConnections checks if DB connections should be skipped
func ShouldSkipConnections(ctx context.Context) bool {
    return ctx.Value(skipConnectionsKey{}) == true
}
```

### Step 2: Update `cmd/internal/skills/command.go`

Modify the `run()` function to wrap the context with the skip flag before calling `collectTools()` [4](#9-3) :

```go
func run(cmd *skillsCmd, opts *internal.ToolboxOptions) error {
    ctx, cancel := context.WithCancel(cmd.Context())
    defer cancel()

    ctx, shutdown, err := opts.Setup(ctx)
    if err != nil {
        return err
    }
    defer func() {
        _ = shutdown(ctx)
    }()

    // Add skipConnections flag for skills generation
    ctx = util.WithSkipConnections(ctx)

    parser := internal.ConfigParser{}
    _, err = opts.LoadConfig(ctx, &parser)
    // ... rest of function unchanged
```

### Step 3: Update Each Source's `Initialize()` Method

For each source package (42 total), wrap the DB connection logic:

```go
func (r Config) Initialize(ctx context.Context, tracer trace.Tracer) (sources.Source, error) {
    if !util.ShouldSkipConnections(ctx) {
        // Open DB connection, ping, verify datasets, etc.
        // ... existing connection logic
    }

    // Always build derived data structures from config fields
    return &Source{Config: r, ...}, nil
}
```

**Sources to update:** All packages under `internal/sources/` including postgres, mysql, sqlite, bigquery, alloydbadmin, cloudsql, http, etc.

### Step 4: Special Handling for BigQuery

BigQuery requires special handling because it builds an `AllowedDatasets` map during initialization. Skip the API verification but still build the map from the config slice [5](#9-4) :

```go
func (r Config) Initialize(ctx context.Context, tracer trace.Tracer) (sources.Source, error) {
    // ... config validation ...

    allowedDatasets := make(map[string]struct{})
    if len(r.AllowedDatasets) > 0 {
        for _, allowed := range r.AllowedDatasets {
            // ... normalization logic ...

            if !util.ShouldSkipConnections(ctx) {
                // Skip API verification when flag is set
                if s.Client != nil {
                    dataset := s.Client.DatasetInProject(projectID, datasetID)
                    _, err := dataset.Metadata(ctx)
                    // ... error handling ...
                }
            }
            allowedDatasets[allowedFullID] = struct{}{}
        }
    }

    // ... rest of initialization
}
```

---

## Testing Strategy

### 1. Unit Tests for Context Helpers (`internal/util/util_test.go`)

```go
func TestSkipConnectionsContext(t *testing.T) {
    ctx := context.Background()

    if util.ShouldSkipConnections(ctx) {
        t.Error("ShouldSkipConnections should return false by default")
    }

    ctx = util.WithSkipConnections(ctx)
    if !util.ShouldSkipConnections(ctx) {
        t.Error("ShouldSkipConnections should return true when flag is set")
    }
}
```

### 2. Integration Test in Skills Command (`cmd/internal/skills/command_test.go`)

Add a test with a source that normally requires a connection (postgres or bigquery) [6](#9-5) :

```go
func TestGenerateSkill_SkipConnections(t *testing.T) {
    tmpDir := t.TempDir()
    outputDir := filepath.Join(tmpDir, "skills")

    toolsFileContent := `
sources:
  my-postgres:
    kind: postgres
    connection_string: "postgres://user:pass@localhost:5432/db"
tools:
  list-tables:
    kind: postgres-list-tables
    source: my-postgres
    description: "List tables"
`

    toolsFilePath := filepath.Join(tmpDir, "tools.yaml")
    os.WriteFile(toolsFilePath, []byte(toolsFileContent), 0644)

    args := []string{
        "skills-generate",
        "--config", toolsFilePath,
        "--output-dir", outputDir,
        "--name", "postgres-skill",
        "--description", "Postgres skill",
    }

    // Should succeed even though postgres is not running
    got, err := invokeCommand(args)
    if err != nil {
        t.Fatalf("command failed (should skip connections): %v\nOutput: %s", err, got)
    }

    // Verify skill was generated
    skillPath := filepath.Join(outputDir, "postgres-skill")
    if _, err := os.Stat(skillPath); os.IsNotExist(err) {
        t.Fatalf("skill directory not created: %s", skillPath)
    }
}
```

### 3. Source Unit Tests

For each source, add a test case following the pattern from `cloudmonitoring_test.go` [7](#9-6) :

```go
func TestInitialize_SkipConnections(t *testing.T) {
    t.Parallel()

    cfg := postgres.Config{
        Name:             "test-source",
        Type:             "postgres",
        ConnectionString: "postgres://user:pass@localhost:5432/db",
    }

    ctx := util.WithSkipConnections(context.Background())
    source, err := cfg.Initialize(ctx, nil)
    if err != nil {
        t.Fatalf("Initialize with skip flag failed: %v", err)
    }

    if source == nil {
        t.Fatal("source should not be nil")
    }

    if source.SourceType() != "postgres" {
        t.Errorf("SourceType() = %q, want %q", source.SourceType(), "postgres")
    }
}
```

---

## Files Changed Summary

| File                                  | Change Type | Description                                     |
| ------------------------------------- | ----------- | ----------------------------------------------- |
| `internal/util/util.go`               | Add         | Context key and helper functions                |
| `internal/util/util_test.go`          | Add         | Unit tests for context helpers                  |
| `cmd/internal/skills/command.go`      | Modify      | Set skip flag in `run()` function               |
| `cmd/internal/skills/command_test.go` | Add         | Integration test with postgres/bigquery         |
| `internal/sources/*/`                 | Modify      | Update `Initialize()` in all 42 source packages |
| `internal/sources/*/*_test.go`        | Add         | Unit tests for skip flag behavior               |

---

## Alternative Approaches (For Reference)

| Approach         | Interface Changes      | Implementation Count | Notes                                                |
| ---------------- | ---------------------- | -------------------- | ---------------------------------------------------- |
| **Context flag** | None                   | 42 sources           | Recommended - follows existing patterns              |
| Stub() method    | New method             | 42 sources           | Explicit separation, but adds interface method       |
| ManifestOnly     | New method             | 50+ tools            | Requires tool changes, source metadata access issues |
| Lazy init        | New method + lifecycle | All sources          | Long-term architectural improvement                  |

---

## Notes

- The context flag propagates through `InitializeConfigs()` to each source's `Initialize()` method via the `childCtx` parameter [2](#9-1)
- Existing skills tests use SQLite (file-based) which doesn't require a running server, so they already pass without the skip flag. The new integration test with postgres/bigquery is needed to verify the flag actually prevents connection attempts.
- For BigQuery, verify that the `AllowedDatasets` map is still built from the config slice even when API verification is skipped.

Wiki pages you might want to explore:

- [Core Concepts & Configuration Reference (googleapis/mcp-toolbox)](/wiki/googleapis/mcp-toolbox#1.2)
- [Agent Skills Generation (googleapis/mcp-toolbox)](/wiki/googleapis/mcp-toolbox#7.1)

### Citations

**File:** cmd/internal/skills/command.go (L74-106)

```go
func run(cmd *skillsCmd, opts *internal.ToolboxOptions) error {
	ctx, cancel := context.WithCancel(cmd.Context())
	defer cancel()

	ctx, shutdown, err := opts.Setup(ctx)
	if err != nil {
		return err
	}
	defer func() {
		_ = shutdown(ctx)
	}()

	parser := internal.ConfigParser{}
	_, err = opts.LoadConfig(ctx, &parser)
	if err != nil {
		return err
	}

	if err := os.MkdirAll(cmd.outputDir, 0755); err != nil {
		errMsg := fmt.Errorf("error creating output directory: %w", err)
		opts.Logger.ErrorContext(ctx, errMsg.Error())
		return errMsg
	}

	opts.Logger.InfoContext(ctx, "Generating skillagent skills...")

	// Group the collected tools by toolset they belong to
	skillsToTools, err := cmd.collectTools(ctx, opts)
	if err != nil {
		errMsg := fmt.Errorf("error collecting skill tools: %w", err)
		opts.Logger.ErrorContext(ctx, errMsg.Error())
		return errMsg
	}
```

**File:** cmd/internal/skills/command.go (L229-236)

```go
func (c *skillsCmd) collectTools(ctx context.Context, opts *internal.ToolboxOptions) (map[string]map[string]tools.Tool, error) {
	// Initialize Resources
	sourcesMap, authServicesMap, embeddingModelsMap, toolsMap, toolsetsMap, promptsMap, promptsetsMap, err := server.InitializeConfigs(ctx, opts.Cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize resources: %w", err)
	}

	resourceMgr := resources.NewResourceManager(sourcesMap, authServicesMap, embeddingModelsMap, toolsMap, toolsetsMap, promptsMap, promptsetsMap)
```

**File:** cmd/internal/skills/command_test.go (L52-149)

```go
func TestGenerateSkill(t *testing.T) {
	// Create a temporary directory for tests
	tmpDir := t.TempDir()
	outputDir := filepath.Join(tmpDir, "skills")

	// Create a tools.yaml file with a sqlite tool
	toolsFileContent := `
sources:
  my-sqlite:
    kind: sqlite
    database: test.db
tools:
  hello-sqlite:
    kind: sqlite-sql
    source: my-sqlite
    description: "hello tool"
    statement: "SELECT 'hello' as greeting"
`

	toolsFilePath := filepath.Join(tmpDir, "tools.yaml")
	if err := os.WriteFile(toolsFilePath, []byte(toolsFileContent), 0644); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}

	args := []string{
		"skills-generate",
		"--config", toolsFilePath,
		"--output-dir", outputDir,
		"--name", "hello-sqlite",
		"--description", "hello tool",
	}

	got, err := invokeCommand(args)
	if err != nil {
		t.Fatalf("command failed: %v\nOutput: %s", err, got)
	}

	// Verify generated directory structure
	skillPath := filepath.Join(outputDir, "hello-sqlite")
	if _, err := os.Stat(skillPath); os.IsNotExist(err) {
		t.Fatalf("skill directory not created: %s", skillPath)
	}

	// Check SKILL.md
	skillMarkdown := filepath.Join(skillPath, "SKILL.md")
	content, err := os.ReadFile(skillMarkdown)
	if err != nil {
		t.Fatalf("failed to read SKILL.md: %v", err)
	}

	expectedFrontmatter := `---
name: hello-sqlite
description: hello tool
---`
	if !strings.HasPrefix(string(content), expectedFrontmatter) {
		t.Errorf("SKILL.md does not have expected frontmatter format.\nExpected prefix:\n%s\nGot:\n%s", expectedFrontmatter, string(content))
	}

	if !strings.Contains(string(content), "## Usage") {
		t.Errorf("SKILL.md does not contain '## Usage' section")
	}

	if !strings.Contains(string(content), "## Scripts") {
		t.Errorf("SKILL.md does not contain '## Scripts' section")
	}

	if !strings.Contains(string(content), "### hello-sqlite") {
		t.Errorf("SKILL.md does not contain '### hello-sqlite' tool header")
	}

	// Check script file
	scriptFilename := "hello-sqlite.js"
	scriptPath := filepath.Join(skillPath, "scripts", scriptFilename)
	if _, err := os.Stat(scriptPath); os.IsNotExist(err) {
		t.Fatalf("script file not created: %s", scriptPath)
	}

	scriptContent, err := os.ReadFile(scriptPath)
	if err != nil {
		t.Fatalf("failed to read script file: %v", err)
	}
	if !strings.Contains(string(scriptContent), "hello-sqlite") {
		t.Errorf("script file does not contain expected tool name")
	}

	// Check assets
	assetPath := filepath.Join(skillPath, "assets", "tools.yaml")
	if _, err := os.Stat(assetPath); os.IsNotExist(err) {
		t.Fatalf("asset file not created: %s", assetPath)
	}
	assetContent, err := os.ReadFile(assetPath)
	if err != nil {
		t.Fatalf("failed to read asset file: %v", err)
	}
	if !strings.Contains(string(assetContent), "hello-sqlite") {
		t.Errorf("asset file does not contain expected tool name")
	}
}
```

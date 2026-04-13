// Copyright 2026 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package describetool

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/googleapis/genai-toolbox/cmd/internal"
	_ "github.com/googleapis/genai-toolbox/internal/sources/sqlite"
	_ "github.com/googleapis/genai-toolbox/internal/tools/sqlite/sqlitesql"
	"github.com/spf13/cobra"
)

func describetoolCommand(args []string) (string, error) {
	parentCmd := &cobra.Command{Use: "toolbox"}

	buf := new(bytes.Buffer)
	opts := internal.NewToolboxOptions(internal.WithIOStreams(buf, buf))

	cmd := NewCommand(opts)
	parentCmd.AddCommand(cmd)
	parentCmd.SetArgs(args)

	err := parentCmd.Execute()
	return buf.String(), err
}

func TestDescribeTool(t *testing.T) {
	tmpDir := t.TempDir()
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
		t.Fatal(err)
	}

	output, err := describetoolCommand([]string{"describe-tool", "hello-sqlite", "--config", toolsFilePath})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(output, "Tool: hello-sqlite") {
		t.Errorf("expected output to contain 'Tool: hello-sqlite', got %q", output)
	}
	if !strings.Contains(output, "Description: hello tool") {
		t.Errorf("expected output to contain 'Description: hello tool', got %q", output)
	}
	if !strings.Contains(output, "Parameters:") {
		t.Errorf("expected output to contain 'Parameters:', got %q", output)
	}
}

func TestDescribeToolNotFound(t *testing.T) {
	tmpDir := t.TempDir()
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
		t.Fatal(err)
	}

	_, err := describetoolCommand([]string{"describe-tool", "non-existent-tool", "--config", toolsFilePath})
	if err == nil {
		t.Fatal("expected error for non-existent tool, got nil")
	}

	if !strings.Contains(err.Error(), `tool "non-existent-tool" not found`) {
		t.Errorf("expected error message to contain 'tool \"non-existent-tool\" not found', got %q", err.Error())
	}
}

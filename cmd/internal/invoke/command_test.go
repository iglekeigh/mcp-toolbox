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

package invoke

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/googleapis/genai-toolbox/cmd/internal"
	_ "github.com/googleapis/genai-toolbox/internal/sources/bigquery"
	_ "github.com/googleapis/genai-toolbox/internal/sources/sqlite"
	_ "github.com/googleapis/genai-toolbox/internal/tools/bigquery/bigquerysql"
	_ "github.com/googleapis/genai-toolbox/internal/tools/sqlite/sqlitesql"
	"github.com/spf13/cobra"
)

func invokeCommand(args []string) (string, error) {
	parentCmd := &cobra.Command{Use: "toolbox"}

	buf := new(bytes.Buffer)
	opts := internal.NewToolboxOptions(internal.WithIOStreams(buf, buf))
	
	internal.ConfigFileFlags(parentCmd.PersistentFlags(), opts)
	internal.PersistentFlags(parentCmd, opts)

	cmd := NewCommand(opts)
	parentCmd.AddCommand(cmd)
	parentCmd.SetArgs(args)

	err := parentCmd.Execute()
	return buf.String(), err
}

func TestInvokeTool(t *testing.T) {
	// Create a temporary config
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
  echo-tool:
    kind: sqlite-sql
    source: my-sqlite
    description: "echo tool"
    statement: "SELECT ? as msg"
    parameters:
      - name: message
        type: string
        description: message to echo
  int-tool:
    kind: sqlite-sql
    source: my-sqlite
    description: "int tool"
    statement: "SELECT ? as val"
    parameters:
      - name: value
        type: integer
        description: int value
  array-tool:
    kind: sqlite-sql
    source: my-sqlite
    description: "array tool"
    statement: "SELECT ? as val"
    parameters:
      - name: value
        type: array
        description: array value
        items:
          name: item
          type: string
          description: array item
  map-tool:
    kind: sqlite-sql
    source: my-sqlite
    description: "map tool"
    statement: "SELECT ? as val"
    parameters:
      - name: value
        type: map
        description: map value
`

	toolsFilePath := filepath.Join(tmpDir, "tools.yaml")
	if err := os.WriteFile(toolsFilePath, []byte(toolsFileContent), 0644); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}

	tcs := []struct {
		desc    string
		args    []string
		want    string
		wantErr bool
		errStr  string
	}{
		{
			desc: "success - basic tool call",
			args: []string{"--config", toolsFilePath, "invoke", "hello-sqlite"},
			want: `"greeting": "hello"`,
		},
		{
			desc: "success - tool call with parameters",
			args: []string{"--config", toolsFilePath, "invoke", "echo-tool", `{"message": "world"}`},
			want: `"msg": "world"`,
		},
		{
			desc: "success - tool call with integer parameters",
			args: []string{"--tools-file", toolsFilePath, "invoke", "int-tool", `{"value": 42}`},
			want: `"val": 42`,
		},
		{
			desc: "success - tool call with flags",
			args: []string{"--config", toolsFilePath, "invoke", "echo-tool", "--message", "value via flag"},
			want: `"msg": "value via flag"`,
		},
		{
			// Note: wantErr is true because SQLite doesn't support arrays as query arguments,
			// but failing here proves that flag parsing and parameter validation succeeded.
			desc: "success - tool call with array flags",
			args: []string{"--config", toolsFilePath, "invoke", "array-tool", "--value", "val1,val2"},
			wantErr: true,
			errStr:  `tool execution failed`,
		},
		{
			// Note: wantErr is true because SQLite doesn't support maps as query arguments,
			// but failing here proves that flag parsing and parameter validation succeeded.
			desc: "success - tool call with map flags",
			args: []string{"--config", toolsFilePath, "invoke", "map-tool", "--value", `{"key": "val"}`},
			wantErr: true,
			errStr:  `tool execution failed`,
		},
		{
			desc:    "error - tool not found",
			args:    []string{"--config", toolsFilePath, "invoke", "non-existent"},
			wantErr: true,
			errStr:  `tool "non-existent" not found`,
		},
		{
			desc:    "error - invalid JSON params",
			args:    []string{"--config", toolsFilePath, "invoke", "echo-tool", `{invalid-json}`},
			wantErr: true,
			errStr:  `params must be a valid JSON string`,
		},
	}

	for _, tc := range tcs {
		t.Run(tc.desc, func(t *testing.T) {
			got, err := invokeCommand(tc.args)
			if (err != nil) != tc.wantErr {
				t.Fatalf("got error %v, wantErr %v", err, tc.wantErr)
			}
			if tc.wantErr && !strings.Contains(err.Error(), tc.errStr) {
				t.Fatalf("got error %v, want error containing %q", err, tc.errStr)
			}
			if !tc.wantErr && !strings.Contains(got, tc.want) {
				t.Fatalf("got %q, want it to contain %q", got, tc.want)
			}
		})
	}
}

func TestInvokeTool_AuthUnsupported(t *testing.T) {
	tmpDir := t.TempDir()
	toolsFileContent := `
sources:
  my-bq:
    kind: bigquery
    project: my-project
    useClientOAuth: true
tools:
  bq-tool:
    kind: bigquery-sql
    source: my-bq
    description: "bq tool"
    statement: "SELECT 1"
`
	toolsFilePath := filepath.Join(tmpDir, "auth_tools.yaml")
	if err := os.WriteFile(toolsFilePath, []byte(toolsFileContent), 0644); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}

	args := []string{"--config", toolsFilePath, "invoke", "bq-tool"}
	_, err := invokeCommand(args)
	if err == nil {
		t.Fatal("expected error for tool requiring client auth, but got nil")
	}
	if !strings.Contains(err.Error(), "client authorization is not supported") {
		t.Fatalf("unexpected error message: %v", err)
	}
}

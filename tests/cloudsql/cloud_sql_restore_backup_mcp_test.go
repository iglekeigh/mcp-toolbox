// Copyright 2026 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package cloudsql

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/googleapis/genai-toolbox/internal/testutils"
	"github.com/googleapis/genai-toolbox/tests"
)

func TestRestoreBackupToolMCP(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	handler := &masterRestoreBackupHandler{t: t}
	server := httptest.NewServer(handler)
	defer server.Close()

	serverURL, err := url.Parse(server.URL)
	if err != nil {
		t.Fatalf("failed to parse server URL: %v", err)
	}

	originalTransport := http.DefaultClient.Transport
	if originalTransport == nil {
		originalTransport = http.DefaultTransport
	}
	http.DefaultClient.Transport = &restoreBackupTransport{
		transport: originalTransport,
		url:       serverURL,
	}
	t.Cleanup(func() {
		http.DefaultClient.Transport = originalTransport
	})

	toolsFile := getRestoreBackupToolsConfig()
	cmd, cleanup, err := tests.StartCmd(ctx, toolsFile)
	if err != nil {
		t.Fatalf("command initialization returned an error: %s", err)
	}
	defer cleanup()

	waitCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	out, err := testutils.WaitForString(waitCtx, regexp.MustCompile(`Server ready to serve`), cmd.Out)
	if err != nil {
		t.Logf("toolbox command logs: \n%s", out)
		t.Fatalf("toolbox didn't start successfully: %s", err)
	}

	tcs := []struct {
		name        string
		toolName    string
		args        map[string]any
		want        string
		expectError bool
		wantError   string
	}{
		{
			name:     "successful restore with standard backup",
			toolName: "restore-backup",
			args:     map[string]any{"target_project": "p1", "target_instance": "instance-standard", "backup_id": "12345", "source_project": "p1", "source_instance": "source"},
			want:     `{"name":"op1","status":"PENDING"}`,
		},
		{
			name:     "successful restore with project level backup",
			toolName: "restore-backup",
			args:     map[string]any{"target_project": "p1", "target_instance": "instance-project-level", "backup_id": "projects/p1/backups/test-uid"},
			want:     `{"name":"op1","status":"PENDING"}`,
		},
		{
			name:     "successful restore with BackupDR backup",
			toolName: "restore-backup",
			args:     map[string]any{"target_project": "p1", "target_instance": "instance-project-level", "backup_id": "projects/p1/locations/us-central1/backupVaults/test-vault/dataSources/test-ds/backups/test-uid"},
			want:     `{"name":"op1","status":"PENDING"}`,
		},
		{
			name:      "missing source instance info for standard backup",
			toolName:  "restore-backup",
			args:      map[string]any{"target_project": "p1", "target_instance": "instance-project-level", "backup_id": "12345"},
			expectError: true,
			wantError: `source project and instance are required when restoring via backup ID`,
		},
		{
			name:      "missing backup identifier",
			toolName:  "restore-backup",
			args:      map[string]any{"target_project": "p1", "target_instance": "instance-project-level"},
			expectError: true,
			wantError: `parameter "backup_id" is required`,
		},
		{
			name:      "missing target instance info",
			toolName:  "restore-backup",
			args:      map[string]any{"backup_id": "12345"},
			expectError: true,
			wantError: `parameter "target_project" is required`,
		},
	}

	for _, tc := range tcs {
		t.Run(tc.name, func(t *testing.T) {
			statusCode, mcpResp, err := tests.InvokeMCPTool(t, tc.toolName, tc.args, nil)
			if err != nil {
				t.Fatalf("native error executing %s: %s", tc.toolName, err)
			}
			if statusCode != http.StatusOK {
				t.Fatalf("expected status 200, got %d", statusCode)
			}

			if tc.expectError {
				tests.AssertMCPError(t, mcpResp, tc.wantError)
				return
			}

			if mcpResp.Result.IsError {
				t.Fatalf("%s returned error result: %v", tc.toolName, mcpResp.Result)
			}

			if len(mcpResp.Result.Content) == 0 {
				t.Fatalf("%s returned empty content field", tc.toolName)
			}

			// Gather all the text blocks
			var blocks []string
			for _, content := range mcpResp.Result.Content {
				if content.Type == "text" {
					blocks = append(blocks, strings.TrimSpace(content.Text))
				}
			}

			got := strings.Join(blocks, "")

			var gotMap, wantMap map[string]any
			if err := json.Unmarshal([]byte(got), &gotMap); err != nil {
				t.Fatalf("failed to unmarshal result: %v\nraw: %s", err, got)
			}
			if err := json.Unmarshal([]byte(tc.want), &wantMap); err != nil {
				t.Fatalf("failed to unmarshal want: %v", err)
			}

			if !reflect.DeepEqual(gotMap, wantMap) {
				t.Fatalf("unexpected result: got %+v, want %+v", gotMap, wantMap)
			}
		})
	}
}

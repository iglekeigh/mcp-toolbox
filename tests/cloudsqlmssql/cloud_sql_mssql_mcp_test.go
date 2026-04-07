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

package cloudsqlmssql

import (
	"context"
	"os"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/googleapis/genai-toolbox/internal/testutils"
	"github.com/googleapis/genai-toolbox/tests"
)

func TestCloudSQLMSSQLMCPToolEndpoints(t *testing.T) {
	if os.Getenv("CLOUD_SQL_MSSQL_PROJECT") == "" {
		t.Skip("Skipping Cloud SQL MSSQL MCP test because environment variables are not set")
	}

	sourceConfig := getCloudSQLMSSQLVars(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	db, err := initCloudSQLMSSQLConnection(CloudSQLMSSQLProject, CloudSQLMSSQLRegion, CloudSQLMSSQLInstance, "public", CloudSQLMSSQLUser, CloudSQLMSSQLPass, CloudSQLMSSQLDatabase)
	if err != nil {
		t.Fatalf("unable to create Cloud SQL connection pool: %s", err)
	}
	defer db.Close()

	// cleanup test environment
	tests.CleanupMSSQLTables(t, ctx, db)

	// create table name with UUID
	tableNameParam := "param_table_" + strings.ReplaceAll(uuid.New().String(), "-", "")
	tableNameAuth := "auth_table_" + strings.ReplaceAll(uuid.New().String(), "-", "")

	// set up data for param tool
	createParamTableStmt, insertParamTableStmt, paramToolStmt, idParamToolStmt, nameParamToolStmt, arrayToolStmt, paramTestParams := tests.GetMSSQLParamToolInfo(tableNameParam)
	teardownTable1 := tests.SetupMsSQLTable(t, ctx, db, createParamTableStmt, insertParamTableStmt, tableNameParam, paramTestParams)
	defer teardownTable1(t)

	// set up data for auth tool
	createAuthTableStmt, insertAuthTableStmt, authToolStmt, authTestParams := tests.GetMSSQLAuthToolInfo(tableNameAuth)
	teardownTable2 := tests.SetupMsSQLTable(t, ctx, db, createAuthTableStmt, insertAuthTableStmt, tableNameAuth, authTestParams)
	defer teardownTable2(t)

	// Write config into a file and pass it to command
	toolsConfig := tests.GetToolsConfig(sourceConfig, CloudSQLMSSQLToolType, paramToolStmt, idParamToolStmt, nameParamToolStmt, arrayToolStmt, authToolStmt)
	toolsConfig = tests.AddMSSQLExecuteSqlConfig(t, toolsConfig)
	tmplSelectCombined, tmplSelectFilterCombined := tests.GetMSSQLTmplToolStatement()
	toolsConfig = tests.AddTemplateParamConfig(t, toolsConfig, CloudSQLMSSQLToolType, tmplSelectCombined, tmplSelectFilterCombined, "")
	toolsConfig = tests.AddMSSQLPrebuiltToolConfig(t, toolsConfig)

	cmd, cleanup, err := tests.StartCmd(ctx, toolsConfig)
	if err != nil {
		t.Fatalf("command initialization returned an error: %s", err)
	}
	defer cleanup()

	waitCtx, cancelWait := context.WithTimeout(ctx, 10*time.Second)
	defer cancelWait()
	out, err := testutils.WaitForString(waitCtx, regexp.MustCompile(`Server ready to serve`), cmd.Out)
	if err != nil {
		t.Logf("toolbox command logs: \n%s", out)
		t.Fatalf("toolbox didn't start successfully: %s", err)
	}

	// Get configs for tests
	_, mcpMyFailToolWant, _, mcpSelect1Want := tests.GetMSSQLWants()

	// Verify the tools list manifest
	expectedTools := tests.GetBaseMCPExpectedTools()
	expectedTools = append(expectedTools, tests.GetExecuteSQLMCPExpectedTools()...)
	expectedTools = append(expectedTools, tests.GetTemplateParamMCPExpectedTools()...)
	expectedTools = append(expectedTools, tests.MCPToolManifest{
		Name:        "list_tables",
		Description: "Lists tables in the database.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"table_names": map[string]any{
					"default":     "",
					"description": "Optional: A comma-separated list of table names. If empty, details for all tables will be listed.",
					"type":        "string",
				},
				"output_format": map[string]any{
					"default":     "detailed",
					"description": "Optional: Use 'simple' for names only or 'detailed' for full info.",
					"type":        "string",
				},
			},
			"required": []any{},
		},
	})

	t.Run("verify tools/list registry returns complete manifest", func(t *testing.T) {
		tests.RunMCPToolsListMethod(t, expectedTools)
	})

	// Run tests via MCP
	tests.RunMCPToolCallMethod(t, mcpMyFailToolWant, mcpSelect1Want)

	// Run specific MSSQL tool tests via MCP
	tests.RunMSSQLListTablesTest(t, tableNameParam, tableNameAuth, tests.WithMCPExec())
}

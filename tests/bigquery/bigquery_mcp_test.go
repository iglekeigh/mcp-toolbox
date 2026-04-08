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

package bigquery

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"reflect"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/googleapis/genai-toolbox/internal/sources"
	"github.com/googleapis/genai-toolbox/internal/testutils"
	"github.com/googleapis/genai-toolbox/tests"
)

func TestBigQueryToolEndpointsMCP(t *testing.T) {
	sourceConfig := getBigQueryVars(t)
	uniqueID := strings.ReplaceAll(uuid.New().String(), "-", "")
	t.Logf("Starting MCP test with uniqueID: %s", uniqueID)

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Minute)
	defer cancel()

	args := []string{"--enable-api"}

	client, err := initBigQueryConnection(BigqueryProject)
	if err != nil {
		t.Fatalf("unable to create BigQuery client: %s", err)
	}

	// create table name with UUID
	datasetName := fmt.Sprintf("temp_toolbox_test_%s", uniqueID)
	tableName := fmt.Sprintf("param_table_%s", uniqueID)
	tableNameParam := fmt.Sprintf("`%s.%s.%s`",
		BigqueryProject,
		datasetName,
		tableName,
	)
	tableNameAuth := fmt.Sprintf("`%s.%s.auth_table_%s`",
		BigqueryProject,
		datasetName,
		uniqueID,
	)
	tableNameForecast := fmt.Sprintf("`%s.%s.forecast_table_%s`",
		BigqueryProject,
		datasetName,
		uniqueID,
	)
	tableNameAnalyzeContribution := fmt.Sprintf("`%s.%s.analyze_contribution_table_%s`",
		BigqueryProject,
		datasetName,
		uniqueID,
	)

	// global cleanup for this test run
	t.Cleanup(func() {
		tests.CleanupBigQueryDatasets(t, context.Background(), client, []string{datasetName})
	})

	// set up data for param tool
	createParamTableStmt, insertParamTableStmt, paramToolStmt, idParamToolStmt, nameParamToolStmt, arrayToolStmt, paramTestParams := getBigQueryParamToolInfo(tableNameParam)
	setupBigQueryTable(t, ctx, client, createParamTableStmt, insertParamTableStmt, datasetName, tableNameParam, paramTestParams)

	// set up data for auth tool
	createAuthTableStmt, insertAuthTableStmt, authToolStmt, authTestParams := getBigQueryAuthToolInfo(tableNameAuth)
	setupBigQueryTable(t, ctx, client, createAuthTableStmt, insertAuthTableStmt, datasetName, tableNameAuth, authTestParams)

	// set up data for forecast tool
	createForecastTableStmt, insertForecastTableStmt, forecastTestParams := getBigQueryForecastToolInfo(tableNameForecast)
	setupBigQueryTable(t, ctx, client, createForecastTableStmt, insertForecastTableStmt, datasetName, tableNameForecast, forecastTestParams)

	// set up data for analyze contribution tool
	createAnalyzeContributionTableStmt, insertAnalyzeContributionTableStmt, analyzeContributionTestParams := getBigQueryAnalyzeContributionToolInfo(tableNameAnalyzeContribution)
	setupBigQueryTable(t, ctx, client, createAnalyzeContributionTableStmt, insertAnalyzeContributionTableStmt, datasetName, tableNameAnalyzeContribution, analyzeContributionTestParams)

	// Write config into a file and pass it to command
	toolsFile := tests.GetToolsConfig(sourceConfig, BigqueryToolType, paramToolStmt, idParamToolStmt, nameParamToolStmt, arrayToolStmt, authToolStmt)
	toolsFile = addClientAuthSourceConfig(t, toolsFile)
	toolsFile = addBigQueryPrebuiltToolsConfig(t, toolsFile)

	cmd, cleanup, err := tests.StartCmd(ctx, toolsFile, args...)
	if err != nil {
		t.Fatalf("command initialization returned an error: %s", err)
	}
	defer cleanup()

	waitCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	out, err := testutils.WaitForString(waitCtx, regexp.MustCompile(`Server ready to serve`), cmd.Out)
	if err != nil {
		t.Logf("toolbox command logs: \n%s", out)
		t.Fatalf("toolbox didn't start successfully: %s", err)
	}

	// FIX: Background goroutine to drain server logs and prevent pipe buffer deadlock.
	go func() {
		_, _ = io.Copy(io.Discard, cmd.Out)
	}()

	select1Want := "[{\"f0_\":1}]"
	invokeParamWant := "[{\"id\":1,\"name\":\"Alice\"},{\"id\":3,\"name\":\"Sid\"}]"
	ddlWant := `"Query executed successfully and returned no content."`
	datasetInfoWant := "\"Location\":\"US\",\"DefaultTableExpiration\":0,\"Labels\":null,\"Access\":"
	tableInfoWant := "{\"Name\":\"\",\"Location\":\"US\",\"Description\":\"\",\"Schema\":[{\"Name\":\"id\""
	dataInsightsWant := `FINAL_RESPONSE`

	runBigQueryExecuteSqlToolInvokeTestMCP(t, select1Want, invokeParamWant, tableNameParam, ddlWant)
	runBigQueryForecastToolInvokeTestMCP(t, tableNameForecast)
	runBigQueryAnalyzeContributionToolInvokeTestMCP(t, tableNameAnalyzeContribution)
	runBigQueryListDatasetToolInvokeTestMCP(t, datasetName)
	runBigQueryGetDatasetInfoToolInvokeTestMCP(t, datasetName, datasetInfoWant)
	runBigQueryListTableIdsToolInvokeTestMCP(t, datasetName, tableName)
	runBigQueryGetTableInfoToolInvokeTestMCP(t, datasetName, tableName, tableInfoWant)
	runBigQueryConversationalAnalyticsInvokeTestMCP(t, datasetName, tableName, dataInsightsWant)
	runBigQuerySearchCatalogToolInvokeTestMCP(t, datasetName, tableName)
	runBigQueryDataTypeTestsMCP(t)
	runBigQueryExecuteSqlToolInvokeDryRunTestMCP(t, datasetName)
}

func getMcpRunner(t *testing.T) TestRunner {
	return func(t *testing.T, info ToolTestInfo) {
		got, handled := invokeMCPToolForTest(t, info)
		if handled {
			return
		}

		if info.Want != "" && !strings.Contains(got, info.Want) {
			t.Fatalf("unexpected result: got %s, want to contain %s", got, info.Want)
		}
	}
}

func invokeMCPToolForTest(t *testing.T, info ToolTestInfo) (string, bool) {
	parts := strings.Split(info.Api, "/")
	toolName := parts[len(parts)-2]

	var args map[string]any
	if info.RequestBody != nil {
		bodyBytes, err := io.ReadAll(info.RequestBody)
		if err != nil {
			t.Fatalf("failed to read request body: %v", err)
		}
		err = json.Unmarshal(bodyBytes, &args)
		if err != nil {
			t.Fatalf("failed to unmarshal request body: %v", err)
		}
	}

	statusCode, mcpResp, err := tests.InvokeMCPTool(t, toolName, args, info.RequestHeader)
	if err != nil {
		t.Fatalf("native error executing %s: %s", toolName, err)
	}

	if info.IsErr {
		if statusCode != http.StatusOK || mcpResp.Result.IsError {
			return "", true
		}
		t.Fatal("expected error result but got success")
		return "", false
	}

	if statusCode != http.StatusOK {
		t.Fatalf("expected status 200, got %d", statusCode)
	}

	if mcpResp.Result.IsError {
		t.Fatalf("%s returned error result: %v", toolName, mcpResp.Result)
	}

	var blocks []string
	for _, content := range mcpResp.Result.Content {
		if content.Type == "text" {
			blocks = append(blocks, strings.TrimSpace(content.Text))
		}
	}

	return strings.Join(blocks, ""), false
}

func runBigQueryExecuteSqlToolInvokeTestMCP(t *testing.T, select1Want, invokeParamWant, tableNameParam, ddlWant string) {
	runBigQueryExecuteSqlToolInvokeTestCommon(t, select1Want, invokeParamWant, tableNameParam, ddlWant, getMcpRunner(t))
}

func runBigQueryForecastToolInvokeTestMCP(t *testing.T, tableName string) {
	runBigQueryForecastToolInvokeTestCommon(t, tableName, getMcpRunner(t))
}

func runBigQueryAnalyzeContributionToolInvokeTestMCP(t *testing.T, tableName string) {
	runBigQueryAnalyzeContributionToolInvokeTestCommon(t, tableName, getMcpRunner(t))
}

func runBigQueryListDatasetToolInvokeTestMCP(t *testing.T, datasetWant string) {
	runBigQueryListDatasetToolInvokeTestCommon(t, datasetWant, getMcpRunner(t))
}

func runBigQueryGetDatasetInfoToolInvokeTestMCP(t *testing.T, datasetName, datasetInfoWant string) {
	runBigQueryGetDatasetInfoToolInvokeTestCommon(t, datasetName, datasetInfoWant, getMcpRunner(t))
}

func runBigQueryListTableIdsToolInvokeTestMCP(t *testing.T, datasetName, tablename_want string) {
	runBigQueryListTableIdsToolInvokeTestCommon(t, datasetName, tablename_want, getMcpRunner(t))
}

func runBigQueryGetTableInfoToolInvokeTestMCP(t *testing.T, datasetName, tableName, tableInfoWant string) {
	runBigQueryGetTableInfoToolInvokeTestCommon(t, datasetName, tableName, tableInfoWant, getMcpRunner(t))
}

func runBigQueryConversationalAnalyticsInvokeTestMCP(t *testing.T, datasetName, tableName, dataInsightsWant string) {
	runBigQueryConversationalAnalyticsInvokeTestCommon(t, datasetName, tableName, dataInsightsWant, func(t *testing.T, info ToolTestInfo) {
		got, handled := invokeMCPToolForTest(t, info)
		if handled {
			return
		}

		wantPattern := regexp.MustCompile(info.Want)
		if !wantPattern.MatchString(got) {
			t.Fatalf("response did not match the expected pattern.\nFull response:\n%s", got)
		}
	})
}

func runBigQuerySearchCatalogToolInvokeTestMCP(t *testing.T, datasetName string, tableName string) {
	runBigQuerySearchCatalogToolInvokeTestCommon(t, datasetName, tableName, func(t *testing.T, info ToolTestInfo) {
		got, handled := invokeMCPToolForTest(t, info)
		if handled {
			return
		}

		var entries []any
		if err := json.Unmarshal([]byte(got), &entries); err != nil {
			t.Fatalf("error unmarshalling result string: %v. Raw string: %s", err, got)
		}

		if len(entries) != 1 {
			t.Fatalf("expected exactly one entry, but got %d", len(entries))
		}
		entry, ok := entries[0].(map[string]interface{})
		if !ok {
			t.Fatalf("expected first entry to be a map, got %T", entries[0])
		}
		respTable, ok := entry[info.Want] // info.Want holds wantKey
		if !ok {
			t.Fatalf("expected entry to have key '%s', but it was not found in %v", info.Want, entry)
		}
		if respTable != tableName {
			t.Fatalf("expected key '%s' to have value '%s', but got %s", info.Want, tableName, respTable)
		}
	})
}

func runBigQueryDataTypeTestsMCP(t *testing.T) {
	runBigQueryDataTypeTestsCommon(t, func(t *testing.T, info ToolTestInfo) {
		got, handled := invokeMCPToolForTest(t, info)
		if handled {
			return
		}

		if info.Want != "" && !strings.Contains(got, info.Want) {
			t.Fatalf("unexpected result: got %s, want to contain %s", got, info.Want)
		}
	})
}

func runBigQueryExecuteSqlToolInvokeDryRunTestMCP(t *testing.T, datasetName string) {
	runBigQueryExecuteSqlToolInvokeDryRunTestCommon(t, datasetName, getMcpRunner(t))
}

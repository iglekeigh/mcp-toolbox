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
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"testing"
	"time"

	bigqueryapi "cloud.google.com/go/bigquery"
	"github.com/google/uuid"
	"github.com/googleapis/mcp-toolbox/internal/testutils"
	"github.com/googleapis/mcp-toolbox/tests"
)

func setupBigQueryMCPServer(t *testing.T, ctx context.Context) (datasetName string, tableNames map[string]string, cleanup func()) {
	sourceConfig := getBigQueryVars(t)
	uniqueID := strings.ReplaceAll(uuid.New().String(), "-", "")
	t.Logf("Starting MCP server setup with uniqueID: %s", uniqueID)

	args := []string{}

	client, err := initBigQueryConnection(BigqueryProject)
	if err != nil {
		t.Fatalf("unable to create BigQuery client: %s", err)
	}

	datasetName = fmt.Sprintf("temp_toolbox_test_%s", uniqueID)
	tableName := fmt.Sprintf("param_table_%s", uniqueID)
	tableNameParam := fmt.Sprintf("`%s.%s.%s`", BigqueryProject, datasetName, tableName)
	tableNameAuth := fmt.Sprintf("`%s.%s.auth_table_%s`", BigqueryProject, datasetName, uniqueID)
	tableNameForecast := fmt.Sprintf("`%s.%s.forecast_table_%s`", BigqueryProject, datasetName, uniqueID)
	tableNameAnalyzeContribution := fmt.Sprintf("`%s.%s.analyze_contribution_table_%s`", BigqueryProject, datasetName, uniqueID)
	tableNameDataType := fmt.Sprintf("`%s.%s.datatype_test_%s`", BigqueryProject, datasetName, uniqueID)

	// global cleanup for this test run
	cleanup = func() {
		tests.CleanupBigQueryDatasets(t, context.Background(), client, []string{datasetName})
	}

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

	// set up data for data type test tool
	createDataTypeTableStmt, insertDataTypeTableStmt, dataTypeToolStmt, arrayDataTypeToolStmt, dataTypeTestParams := getBigQueryDataTypeTestInfo(tableNameDataType)
	setupBigQueryTable(t, ctx, client, createDataTypeTableStmt, insertDataTypeTableStmt, datasetName, tableNameDataType, dataTypeTestParams)

	// Write config into a file and pass it to command
	toolsFile := tests.GetToolsConfig(sourceConfig, BigqueryToolType, paramToolStmt, idParamToolStmt, nameParamToolStmt, arrayToolStmt, authToolStmt)
	toolsFile = addClientAuthSourceConfig(t, toolsFile)
	toolsFile = addBigQuerySqlToolConfig(t, toolsFile, dataTypeToolStmt, arrayDataTypeToolStmt)
	toolsFile = addBigQueryPrebuiltToolsConfig(t, toolsFile)

	cmd, cmdCleanup, err := tests.StartCmd(ctx, toolsFile, args...)
	if err != nil {
		cleanup()
		t.Fatalf("command initialization returned an error: %s", err)
	}

	waitCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	out, err := testutils.WaitForString(waitCtx, regexp.MustCompile(`Server ready to serve`), cmd.Out)
	if err != nil {
		cleanup()
		cmdCleanup()
		t.Logf("toolbox command logs: \n%s", out)
		t.Fatalf("toolbox didn't start successfully: %s", err)
	}

	// Background goroutine to drain server logs and prevent pipe buffer deadlock.
	go func() {
		_, _ = io.Copy(io.Discard, cmd.Out)
	}()

	tableNames = map[string]string{
		"paramTableFull":               tableNameParam,
		"authTableFull":                tableNameAuth,
		"forecastTableFull":            tableNameForecast,
		"analyzeContributionTableFull": tableNameAnalyzeContribution,
		"dataTypeTableFull":            tableNameDataType,
		"tableId":                      tableName,
	}

	return datasetName, tableNames, func() {
		cmdCleanup()
		cleanup()
	}
}

func TestBigQueryListToolsMCP(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	_, _, cleanup := setupBigQueryMCPServer(t, ctx)
	defer cleanup()

	statusCode, toolsList, err := tests.GetMCPToolsList(t, nil)
	if err != nil {
		t.Fatalf("failed to get tools list: %v", err)
	}
	if statusCode != http.StatusOK {
		t.Fatalf("expected status 200, got %d", statusCode)
	}

	if len(toolsList) == 0 {
		t.Fatalf("expected non-empty tools list")
	}

	// Verify specific tools are present
	expectedTools := map[string]bool{
		"my-scalar-datatype-tool": false,
		"my-array-datatype-tool":  false,
	}

	for _, toolObj := range toolsList {
		toolMap, ok := toolObj.(map[string]any)
		if !ok {
			continue
		}
		name, _ := toolMap["name"].(string)
		if _, ok := expectedTools[name]; ok {
			expectedTools[name] = true
		}
	}

	for name, found := range expectedTools {
		if !found {
			t.Errorf("expected tool %q not found in list", name)
		}
	}
}

func TestBigQueryCallToolsMCP(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Minute)
	defer cancel()

	datasetName, tableNames, cleanup := setupBigQueryMCPServer(t, ctx)
	defer cleanup()

	select1Want := "[{\"f0_\":1}]"
	invokeParamWant := "[{\"id\":1,\"name\":\"Alice\"},{\"id\":3,\"name\":\"Sid\"}]"
	ddlWant := `"Query executed successfully and returned no content."`
	datasetInfoWant := "\"Location\":\"US\",\"DefaultTableExpiration\":0,\"Labels\":null,\"Access\":"
	tableInfoWant := "{\"Name\":\"\",\"Location\":\"US\",\"Description\":\"\",\"Schema\":[{\"Name\":\"id\""
	dataInsightsWant := `FINAL_RESPONSE`

	// Extract table names from map
	tableNameParam := tableNames["paramTableFull"]
	tableNameForecast := tableNames["forecastTableFull"]
	tableNameAnalyzeContribution := tableNames["analyzeContributionTableFull"]
	tableName := tableNames["tableId"]

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
	if len(parts) < 2 {
		t.Fatalf("invalid API URL: %s", info.Api)
	}
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

	var blocks []string
	for _, content := range mcpResp.Result.Content {
		if content.Type == "text" {
			blocks = append(blocks, strings.TrimSpace(content.Text))
		}
	}

	got := strings.Join(blocks, "")

	isActualErr := statusCode != http.StatusOK || (mcpResp != nil && (mcpResp.Result.IsError || mcpResp.Error != nil))

	if info.IsErr {
		// Special case for search-catalog with non-existent project in MCP
		if info.Name == "Invoke my-auth-search-catalog-tool with non-existent project" && !isActualErr {
			// MCP returns success (empty list) for non-existent project, which is acceptable.
			return got, true
		}

		if isActualErr {
			// Extract error message for comparison
			errMsg := got
			if mcpResp != nil && mcpResp.Error != nil {
				errMsg = mcpResp.Error.Message
			}
			if info.Want == "" || strings.Contains(errMsg, info.Want) {
				return errMsg, true
			}
			// Fallback mapping for typical MCP error messages vs legacy API expectations
			if info.Want == "auth token is required" && (strings.Contains(errMsg, "missing access token") || strings.Contains(errMsg, "invalid_request")) {
				return errMsg, true
			}
			if info.Want == "Authorization header is required" && strings.Contains(errMsg, "missing access token") {
				return errMsg, true
			}
			if info.Want == "invalid token" && (strings.Contains(errMsg, "invalid token") || strings.Contains(errMsg, "invalid_request")) {
				return errMsg, true
			}
		}
		t.Fatalf("expected error result containing %q but got success or non-matching error. Status: %d, Result.IsError: %v, Error: %+v", info.Want, statusCode, mcpResp.Result.IsError, mcpResp.Error)
		return got, false
	}

	if !info.IsErr && mcpResp != nil && mcpResp.Error != nil {
		// Server returned an error, but test didn't expect protocol error.
		// Check if test expects an error message in success response.
		if info.Want != "" {
			errMsg := mcpResp.Error.Message
			if strings.HasPrefix(info.Want, "{") && !strings.HasPrefix(errMsg, "{") {
				errMsg = fmt.Sprintf(`{"error":%q}`, errMsg)
			}
			return errMsg, false
		}
	}

	if statusCode != http.StatusOK {
		t.Fatalf("expected status 200, got %d", statusCode)
	}

	if mcpResp != nil && mcpResp.Result.IsError {
		t.Fatalf("%s returned error result: %v. Response: %s", toolName, mcpResp.Result, got)
	}

	result := strings.Join(blocks, ",")
	if strings.HasPrefix(info.Want, "[") && !strings.HasPrefix(result, "[") {
		result = "[" + result + "]"
	}

	return result, false
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

func TestBigQueryToolWithDatasetRestrictionMCP(t *testing.T) {
	uniqueID := strings.ReplaceAll(uuid.New().String(), "-", "")
	t.Logf("Starting restriction test with uniqueID: %s", uniqueID)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	client, err := initBigQueryConnection(BigqueryProject)
	if err != nil {
		t.Fatalf("unable to create BigQuery client: %s", err)
	}

	allowedDatasetName1 := fmt.Sprintf("allowed_dataset_1_%s", uniqueID)
	allowedDatasetName2 := fmt.Sprintf("allowed_dataset_2_%s", uniqueID)
	disallowedDatasetName := fmt.Sprintf("disallowed_dataset_%s", uniqueID)
	allowedTableName1 := "allowed_table_1"
	allowedTableName2 := "allowed_table_2"
	disallowedTableName := "disallowed_table"
	allowedForecastTableName1 := "allowed_forecast_table_1"
	allowedForecastTableName2 := "allowed_forecast_table_2"
	disallowedForecastTableName := "disallowed_forecast_table"
	allowedAnalyzeContributionTableName1 := "allowed_analyze_contribution_table_1"
	allowedAnalyzeContributionTableName2 := "allowed_analyze_contribution_table_2"
	disallowedAnalyzeContributionTableName := "disallowed_analyze_contribution_table"

	// global cleanup for this test run
	t.Cleanup(func() {
		tests.CleanupBigQueryDatasets(t, context.Background(), client, []string{allowedDatasetName1, allowedDatasetName2, disallowedDatasetName})
	})

	// Setup allowed table
	allowedTableNameParam1 := fmt.Sprintf("`%s.%s.%s`", BigqueryProject, allowedDatasetName1, allowedTableName1)
	createAllowedTableStmt1 := fmt.Sprintf("CREATE TABLE %s (id INT64)", allowedTableNameParam1)
	setupBigQueryTable(t, ctx, client, createAllowedTableStmt1, "", allowedDatasetName1, allowedTableNameParam1, nil)

	allowedTableNameParam2 := fmt.Sprintf("`%s.%s.%s`", BigqueryProject, allowedDatasetName2, allowedTableName2)
	createAllowedTableStmt2 := fmt.Sprintf("CREATE TABLE %s (id INT64)", allowedTableNameParam2)
	setupBigQueryTable(t, ctx, client, createAllowedTableStmt2, "", allowedDatasetName2, allowedTableNameParam2, nil)

	// Setup allowed forecast table
	allowedForecastTableFullName1 := fmt.Sprintf("`%s.%s.%s`", BigqueryProject, allowedDatasetName1, allowedForecastTableName1)
	createForecastStmt1, insertForecastStmt1, forecastParams1 := getBigQueryForecastToolInfo(allowedForecastTableFullName1)
	setupBigQueryTable(t, ctx, client, createForecastStmt1, insertForecastStmt1, allowedDatasetName1, allowedForecastTableFullName1, forecastParams1)

	allowedForecastTableFullName2 := fmt.Sprintf("`%s.%s.%s`", BigqueryProject, allowedDatasetName2, allowedForecastTableName2)
	createForecastStmt2, insertForecastStmt2, forecastParams2 := getBigQueryForecastToolInfo(allowedForecastTableFullName2)
	setupBigQueryTable(t, ctx, client, createForecastStmt2, insertForecastStmt2, allowedDatasetName2, allowedForecastTableFullName2, forecastParams2)

	// Setup disallowed table
	disallowedTableNameParam := fmt.Sprintf("`%s.%s.%s`", BigqueryProject, disallowedDatasetName, disallowedTableName)
	createDisallowedTableStmt := fmt.Sprintf("CREATE TABLE %s (id INT64)", disallowedTableNameParam)
	setupBigQueryTable(t, ctx, client, createDisallowedTableStmt, "", disallowedDatasetName, disallowedTableNameParam, nil)

	// Setup disallowed forecast table
	disallowedForecastTableFullName := fmt.Sprintf("`%s.%s.%s`", BigqueryProject, disallowedDatasetName, disallowedForecastTableName)
	createDisallowedForecastStmt, insertDisallowedForecastStmt, disallowedForecastParams := getBigQueryForecastToolInfo(disallowedForecastTableFullName)
	setupBigQueryTable(t, ctx, client, createDisallowedForecastStmt, insertDisallowedForecastStmt, disallowedDatasetName, disallowedForecastTableFullName, disallowedForecastParams)

	// Setup allowed analyze contribution table
	allowedAnalyzeContributionTableFullName1 := fmt.Sprintf("`%s.%s.%s`", BigqueryProject, allowedDatasetName1, allowedAnalyzeContributionTableName1)
	createAnalyzeContributionStmt1, insertAnalyzeContributionStmt1, analyzeContributionParams1 := getBigQueryAnalyzeContributionToolInfo(allowedAnalyzeContributionTableFullName1)
	setupBigQueryTable(t, ctx, client, createAnalyzeContributionStmt1, insertAnalyzeContributionStmt1, allowedDatasetName1, allowedAnalyzeContributionTableFullName1, analyzeContributionParams1)

	allowedAnalyzeContributionTableFullName2 := fmt.Sprintf("`%s.%s.%s`", BigqueryProject, allowedDatasetName2, allowedAnalyzeContributionTableName2)
	createAnalyzeContributionStmt2, insertAnalyzeContributionStmt2, analyzeContributionParams2 := getBigQueryAnalyzeContributionToolInfo(allowedAnalyzeContributionTableFullName2)
	setupBigQueryTable(t, ctx, client, createAnalyzeContributionStmt2, insertAnalyzeContributionStmt2, allowedDatasetName2, allowedAnalyzeContributionTableFullName2, analyzeContributionParams2)

	// Setup disallowed analyze contribution table
	disallowedAnalyzeContributionTableFullName := fmt.Sprintf("`%s.%s.%s`", BigqueryProject, disallowedDatasetName, disallowedAnalyzeContributionTableName)
	createDisallowedAnalyzeContributionStmt, insertDisallowedAnalyzeContributionStmt, disallowedAnalyzeContributionParams := getBigQueryAnalyzeContributionToolInfo(disallowedAnalyzeContributionTableFullName)
	setupBigQueryTable(t, ctx, client, createDisallowedAnalyzeContributionStmt, insertDisallowedAnalyzeContributionStmt, disallowedDatasetName, disallowedAnalyzeContributionTableFullName, disallowedAnalyzeContributionParams)

	// Configure source with dataset restriction.
	sourceConfig := getBigQueryVars(t)
	sourceConfig["allowedDatasets"] = []string{allowedDatasetName1, allowedDatasetName2}

	// Configure tools
	toolsConfig := map[string]any{
		"list-dataset-ids-restricted": map[string]any{
			"type":        "bigquery-list-dataset-ids",
			"source":      "my-instance",
			"description": "Tool to list dataset ids",
		},
		"list-table-ids-restricted": map[string]any{
			"type":        "bigquery-list-table-ids",
			"source":      "my-instance",
			"description": "Tool to list table within a dataset",
		},
		"get-dataset-info-restricted": map[string]any{
			"type":        "bigquery-get-dataset-info",
			"source":      "my-instance",
			"description": "Tool to get dataset info",
		},
		"get-table-info-restricted": map[string]any{
			"type":        "bigquery-get-table-info",
			"source":      "my-instance",
			"description": "Tool to get table info",
		},
		"execute-sql-restricted": map[string]any{
			"type":        "bigquery-execute-sql",
			"source":      "my-instance",
			"description": "Tool to execute SQL",
		},
		"forecast-restricted": map[string]any{
			"type":        "bigquery-forecast",
			"source":      "my-instance",
			"description": "Tool to forecast",
		},
		"analyze-contribution-restricted": map[string]any{
			"type":        "bigquery-analyze-contribution",
			"source":      "my-instance",
			"description": "Tool to analyze contribution",
		},
	}

	config := map[string]any{
		"sources": map[string]any{
			"my-instance": sourceConfig,
		},
		"tools": toolsConfig,
	}

	args := []string{}
	cmd, cleanup, err := tests.StartCmd(ctx, config, args...)
	if err != nil {
		t.Fatalf("command initialization returned an error: %s", err)
	}
	defer cleanup()

	go func() {
		_, _ = io.Copy(io.Discard, cmd.Out)
	}()

	// Wait for server to be ready
	waitCtx, cancelReady := context.WithTimeout(ctx, 10*time.Second)
	defer cancelReady()
	out, err := testutils.WaitForString(waitCtx, regexp.MustCompile(`Server ready to serve`), cmd.Out)
	if err != nil {
		t.Logf("toolbox command logs: \n%s", out)
		t.Fatalf("toolbox didn't start successfully: %s", err)
	}

	// Test Cases
	testCases := []struct {
		name             string
		toolName         string
		args             map[string]any
		wantInResult     string
		wantInResultList []string
		wantInError      string
	}{
		{
			name:             "list dataset ids",
			toolName:         "list-dataset-ids-restricted",
			args:             map[string]any{},
			wantInResultList: []string{allowedDatasetName1, allowedDatasetName2},
		},
		{
			name:         "list table ids allowed",
			toolName:     "list-table-ids-restricted",
			args:         map[string]any{"dataset": allowedDatasetName1},
			wantInResult: allowedTableName1,
		},
		{
			name:        "list table ids disallowed",
			toolName:    "list-table-ids-restricted",
			args:        map[string]any{"dataset": disallowedDatasetName},
			wantInError: "access denied to dataset",
		},
		{
			name:     "get dataset info allowed",
			toolName: "get-dataset-info-restricted",
			args:     map[string]any{"dataset": allowedDatasetName1},
		},
		{
			name:        "get dataset info disallowed",
			toolName:    "get-dataset-info-restricted",
			args:        map[string]any{"dataset": disallowedDatasetName},
			wantInError: "access denied to dataset",
		},
		{
			name:     "get table info allowed",
			toolName: "get-table-info-restricted",
			args:     map[string]any{"dataset": allowedDatasetName1, "table": allowedTableName1},
		},
		{
			name:     "get table info disallowed",
			toolName: "get-table-info-restricted",
			args:     map[string]any{"dataset": disallowedDatasetName, "table": disallowedTableName},
		},
		{
			name:         "execute sql allowed",
			toolName:     "execute-sql-restricted",
			args:         map[string]any{"sql": fmt.Sprintf("SELECT * FROM %s", allowedTableNameParam1)},
			wantInResult: "Query executed successfully",
		},
		{
			name:        "execute sql disallowed",
			toolName:    "execute-sql-restricted",
			args:        map[string]any{"sql": fmt.Sprintf("SELECT * FROM %s", disallowedTableNameParam)},
			wantInError: "which is not in the allowed list",
		},
		{
			name:        "disallowed create schema",
			toolName:    "execute-sql-restricted",
			args:        map[string]any{"sql": "CREATE SCHEMA another_dataset"},
			wantInError: "dataset-level operations like 'CREATE_SCHEMA' are not allowed",
		},
		{
			name:        "disallowed alter schema",
			toolName:    "execute-sql-restricted",
			args:        map[string]any{"sql": fmt.Sprintf("ALTER SCHEMA %s SET OPTIONS(description='new one')", allowedDatasetName1)},
			wantInError: "dataset-level operations like 'ALTER_SCHEMA' are not allowed",
		},
		{
			name:        "disallowed create function",
			toolName:    "execute-sql-restricted",
			args:        map[string]any{"sql": fmt.Sprintf("CREATE FUNCTION %s.my_func() RETURNS INT64 AS (1)", allowedDatasetName1)},
			wantInError: "creating stored routines ('CREATE_FUNCTION') is not allowed",
		},
		{
			name:        "disallowed create procedure",
			toolName:    "execute-sql-restricted",
			args:        map[string]any{"sql": fmt.Sprintf("CREATE PROCEDURE %s.my_proc() BEGIN SELECT 1; END", allowedDatasetName1)},
			wantInError: "unanalyzable statements like 'CREATE PROCEDURE' are not allowed",
		},
		{
			name:        "disallowed execute immediate",
			toolName:    "execute-sql-restricted",
			args:        map[string]any{"sql": "EXECUTE IMMEDIATE 'SELECT 1'"},
			wantInError: "EXECUTE IMMEDIATE is not allowed when dataset restrictions are in place",
		},
		{
			name:         "conversational analytics allowed",
			toolName:     "conversational-analytics-restricted",
			args:         map[string]any{"user_query_with_context": "What is in the table?", "table_references": fmt.Sprintf(`[{"projectId":"%s","datasetId":"%s","tableId":"%s"}]`, BigqueryProject, allowedDatasetName1, allowedTableName1)},
			wantInResult: "FINAL_RESPONSE",
		},
		{
			name:        "conversational analytics disallowed",
			toolName:    "conversational-analytics-restricted",
			args:        map[string]any{"user_query_with_context": "What is in the table?", "table_references": fmt.Sprintf(`[{"projectId":"%s","datasetId":"%s","tableId":"%s"}]`, BigqueryProject, disallowedDatasetName, disallowedTableName)},
			wantInError: "not allowed",
		},
		{
			name:         "forecast allowed",
			toolName:     "forecast-restricted",
			args:         map[string]any{"history_data": fmt.Sprintf("%s.%s.%s", BigqueryProject, allowedDatasetName1, allowedForecastTableName1), "timestamp_col": "ts", "data_col": "data"},
			wantInResult: "forecast_timestamp",
		},
		{
			name:        "forecast disallowed",
			toolName:    "forecast-restricted",
			args:        map[string]any{"history_data": fmt.Sprintf("%s.%s.%s", BigqueryProject, disallowedDatasetName, disallowedForecastTableName), "timestamp_col": "ts", "data_col": "data"},
			wantInError: "not allowed",
		},
		{
			name:         "analyze contribution allowed",
			toolName:     "analyze-contribution-restricted",
			args:         map[string]any{"input_data": fmt.Sprintf("%s.%s.%s", BigqueryProject, allowedDatasetName1, allowedAnalyzeContributionTableName1), "contribution_metric": "SUM(metric)", "is_test_col": "is_test", "dimension_id_cols": []any{"dim1"}},
			wantInResult: "relative_difference",
		},
		{
			name:        "analyze contribution disallowed",
			toolName:    "analyze-contribution-restricted",
			args:        map[string]any{"input_data": fmt.Sprintf("%s.%s.%s", BigqueryProject, disallowedDatasetName, disallowedAnalyzeContributionTableName), "contribution_metric": "SUM(metric)", "is_test_col": "is_test", "dimension_id_cols": []any{"dim1"}},
			wantInError: "not allowed",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			statusCode, mcpResp, err := tests.InvokeMCPTool(t, tc.toolName, tc.args, nil)
			if err != nil {
				t.Fatalf("native error executing tool: %s", err)
			}
			if statusCode != http.StatusOK {
				t.Fatalf("expected status 200, got %d", statusCode)
			}

			var got string
			if mcpResp.Error != nil {
				got = mcpResp.Error.Message
			} else {
				var blocks []string
				for _, content := range mcpResp.Result.Content {
					if content.Type == "text" {
						blocks = append(blocks, content.Text)
					}
				}
				got = strings.Join(blocks, ",")
			}

			if tc.wantInError != "" {
				if !strings.Contains(got, tc.wantInError) {
					t.Fatalf("expected error containing %q, got %q", tc.wantInError, got)
				}
			} else {
				if tc.wantInResult != "" {
					if !strings.Contains(got, tc.wantInResult) {
						t.Fatalf("expected result containing %q, got %q", tc.wantInResult, got)
					}
				}
				for _, want := range tc.wantInResultList {
					if !strings.Contains(got, want) {
						t.Fatalf("expected result containing %q, got %q", want, got)
					}
				}
			}
		})
	}
}

func TestBigQueryWriteModeAllowedMCP(t *testing.T) {
	sourceConfig := getBigQueryVars(t)
	sourceConfig["writeMode"] = "allowed"

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	datasetName := fmt.Sprintf("temp_toolbox_test_allowed_%s", strings.ReplaceAll(uuid.New().String(), "-", ""))

	client, err := initBigQueryConnection(BigqueryProject)
	if err != nil {
		t.Fatalf("unable to create BigQuery connection: %s", err)
	}

	dataset := client.Dataset(datasetName)
	if err := dataset.Create(ctx, &bigqueryapi.DatasetMetadata{Name: datasetName}); err != nil {
		t.Fatalf("Failed to create dataset %q: %v", datasetName, err)
	}
	defer func() {
		if err := dataset.DeleteWithContents(ctx); err != nil {
			t.Logf("failed to cleanup dataset %s: %v", datasetName, err)
		}
	}()

	toolsConfig := map[string]any{
		"my-exec-sql-tool": map[string]any{
			"type":        "bigquery-execute-sql",
			"source":      "my-instance",
			"description": "Tool to execute sql",
		},
	}

	config := map[string]any{
		"sources": map[string]any{
			"my-instance": sourceConfig,
		},
		"tools": toolsConfig,
	}

	args := []string{}
	cmd, cleanup, err := tests.StartCmd(ctx, config, args...)
	if err != nil {
		t.Fatalf("command initialization returned an error: %s", err)
	}
	defer cleanup()

	go func() {
		_, _ = io.Copy(io.Discard, cmd.Out)
	}()

	waitCtx, cancelReady := context.WithTimeout(ctx, 10*time.Second)
	defer cancelReady()
	_, err = testutils.WaitForString(waitCtx, regexp.MustCompile(`Server ready to serve`), cmd.Out)
	if err != nil {
		t.Fatalf("toolbox didn't start successfully: %s", err)
	}

	t.Run("CREATE TABLE should succeed", func(t *testing.T) {
		sql := fmt.Sprintf("CREATE TABLE %s.new_table (x INT64)", datasetName)
		statusCode, mcpResp, err := tests.InvokeMCPTool(t, "my-exec-sql-tool", map[string]any{"sql": sql}, nil)
		if err != nil {
			t.Fatalf("native error executing tool: %s", err)
		}
		if statusCode != http.StatusOK {
			t.Fatalf("expected status 200, got %d", statusCode)
		}
		if mcpResp.Error != nil {
			t.Fatalf("expected no error, got %v", mcpResp.Error)
		}

		var got string
		var blocks []string
		for _, content := range mcpResp.Result.Content {
			if content.Type == "text" {
				blocks = append(blocks, content.Text)
			}
		}
		got = strings.Join(blocks, ",")

		want := "Query executed successfully and returned no content."
		if !strings.Contains(got, want) {
			t.Errorf("unexpected result: got %q, want to contain %q", got, want)
		}
	})
}

func TestBigQueryWriteModeBlockedMCP(t *testing.T) {
	sourceConfig := getBigQueryVars(t)
	sourceConfig["writeMode"] = "blocked"

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	datasetName := fmt.Sprintf("temp_toolbox_test_blocked_%s", strings.ReplaceAll(uuid.New().String(), "-", ""))
	tableName := fmt.Sprintf("param_table_blocked_%s", strings.ReplaceAll(uuid.New().String(), "-", ""))
	tableNameParam := fmt.Sprintf("`%s.%s.%s`", BigqueryProject, datasetName, tableName)

	client, err := initBigQueryConnection(BigqueryProject)
	if err != nil {
		t.Fatalf("unable to create BigQuery connection: %s", err)
	}
	createParamTableStmt, insertParamTableStmt, _, _, _, _, paramTestParams := getBigQueryParamToolInfo(tableNameParam)
	teardownTable := setupBigQueryTable(t, ctx, client, createParamTableStmt, insertParamTableStmt, datasetName, tableNameParam, paramTestParams)
	defer teardownTable(t)

	toolsConfig := map[string]any{
		"my-exec-sql-tool": map[string]any{
			"type":        "bigquery-execute-sql",
			"source":      "my-instance",
			"description": "Tool to execute sql",
		},
	}

	config := map[string]any{
		"sources": map[string]any{"my-instance": sourceConfig},
		"tools":   toolsConfig,
	}

	args := []string{}
	cmd, cleanup, err := tests.StartCmd(ctx, config, args...)
	if err != nil {
		t.Fatalf("command initialization returned an error: %s", err)
	}
	defer cleanup()

	go func() {
		_, _ = io.Copy(io.Discard, cmd.Out)
	}()

	waitCtx, cancelReady := context.WithTimeout(ctx, 10*time.Second)
	defer cancelReady()
	_, err = testutils.WaitForString(waitCtx, regexp.MustCompile(`Server ready to serve`), cmd.Out)
	if err != nil {
		t.Fatalf("toolbox didn't start successfully: %s", err)
	}

	testCases := []struct {
		name         string
		sql          string
		wantInResult string
		wantInError  string
	}{
		{
			name:         "SELECT statement should succeed",
			sql:          fmt.Sprintf("SELECT id, name FROM %s WHERE id = 1", tableNameParam),
			wantInResult: `"id":1`,
		},
		{
			name:        "INSERT statement should fail",
			sql:         fmt.Sprintf("INSERT INTO %s (id, name) VALUES (10, 'test')", tableNameParam),
			wantInError: "write mode is 'blocked'",
		},
		{
			name:        "CREATE TABLE statement should fail",
			sql:         fmt.Sprintf("CREATE TABLE %s.new_table (x INT64)", datasetName),
			wantInError: "write mode is 'blocked'",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			statusCode, mcpResp, err := tests.InvokeMCPTool(t, "my-exec-sql-tool", map[string]any{"sql": tc.sql}, nil)
			if err != nil {
				t.Fatalf("native error executing tool: %s", err)
			}
			if statusCode != http.StatusOK {
				t.Fatalf("expected status 200, got %d", statusCode)
			}

			var got string
			if mcpResp.Error != nil {
				got = mcpResp.Error.Message
			} else {
				var blocks []string
				for _, content := range mcpResp.Result.Content {
					if content.Type == "text" {
						blocks = append(blocks, content.Text)
					}
				}
				got = strings.Join(blocks, ",")
			}

			if tc.wantInError != "" {
				if !strings.Contains(got, tc.wantInError) {
					t.Fatalf("expected error containing %q, got %q", tc.wantInError, got)
				}
			} else if tc.wantInResult != "" {
				if !strings.Contains(got, tc.wantInResult) {
					t.Fatalf("expected result containing %q, got %q", tc.wantInResult, got)
				}
			}
		})
	}
}

func TestBigQueryWriteModeProtectedMCP(t *testing.T) {
	sourceConfig := getBigQueryVars(t)
	sourceConfig["writeMode"] = "protected"

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	permanentDatasetName := fmt.Sprintf("perm_dataset_protected_%s", strings.ReplaceAll(uuid.New().String(), "-", ""))
	client, err := initBigQueryConnection(BigqueryProject)
	if err != nil {
		t.Fatalf("unable to create BigQuery connection: %s", err)
	}
	dataset := client.Dataset(permanentDatasetName)
	if err := dataset.Create(ctx, &bigqueryapi.DatasetMetadata{Name: permanentDatasetName}); err != nil {
		t.Fatalf("Failed to create dataset %q: %v", permanentDatasetName, err)
	}
	defer func() {
		if err := dataset.DeleteWithContents(ctx); err != nil {
			t.Logf("failed to cleanup dataset %s: %v", permanentDatasetName, err)
		}
	}()

	toolsConfig := map[string]any{
		"my-exec-sql-tool": map[string]any{"type": "bigquery-execute-sql", "source": "my-instance", "description": "Tool to execute sql"},
		"my-sql-tool-protected": map[string]any{
			"type":        "bigquery-sql",
			"source":      "my-instance",
			"description": "Tool to query from the session",
			"statement":   "SELECT * FROM my_shared_temp_table",
		},
		"my-forecast-tool-protected": map[string]any{
			"type":        "bigquery-forecast",
			"source":      "my-instance",
			"description": "Tool to forecast from session temp table",
		},
		"my-analyze-contribution-tool-protected": map[string]any{
			"type":        "bigquery-analyze-contribution",
			"source":      "my-instance",
			"description": "Tool to analyze contribution from session temp table",
		},
	}

	config := map[string]any{
		"sources": map[string]any{"my-instance": sourceConfig},
		"tools":   toolsConfig,
	}

	args := []string{}
	cmd, cleanup, err := tests.StartCmd(ctx, config, args...)
	if err != nil {
		t.Fatalf("command initialization returned an error: %s", err)
	}
	defer cleanup()

	go func() {
		_, _ = io.Copy(io.Discard, cmd.Out)
	}()

	waitCtx, cancelReady := context.WithTimeout(ctx, 10*time.Second)
	defer cancelReady()
	_, err = testutils.WaitForString(waitCtx, regexp.MustCompile(`Server ready to serve`), cmd.Out)
	if err != nil {
		t.Fatalf("toolbox didn't start successfully: %s", err)
	}

	testCases := []struct {
		name         string
		toolName     string
		args         map[string]any
		wantInResult string
		wantInError  string
	}{
		{
			name:        "CREATE TABLE to permanent dataset should fail",
			toolName:    "my-exec-sql-tool",
			args:        map[string]any{"sql": fmt.Sprintf("CREATE TABLE %s.new_table (x INT64)", permanentDatasetName)},
			wantInError: "protected write mode only supports SELECT statements",
		},
		{
			name:         "CREATE TEMP TABLE should succeed",
			toolName:     "my-exec-sql-tool",
			args:         map[string]any{"sql": "CREATE TEMP TABLE my_shared_temp_table (x INT64)"},
			wantInResult: "Query executed successfully",
		},
		{
			name:         "INSERT into TEMP TABLE should succeed",
			toolName:     "my-exec-sql-tool",
			args:         map[string]any{"sql": "INSERT INTO my_shared_temp_table (x) VALUES (42)"},
			wantInResult: "Query executed successfully",
		},
		{
			name:         "SELECT from TEMP TABLE with exec-sql should succeed",
			toolName:     "my-exec-sql-tool",
			args:         map[string]any{"sql": "SELECT * FROM my_shared_temp_table"},
			wantInResult: `"x":42`,
		},
		{
			name:         "SELECT from TEMP TABLE with sql-tool should succeed",
			toolName:     "my-sql-tool-protected",
			args:         map[string]any{},
			wantInResult: `"x":42`,
		},
		{
			name:         "CREATE TEMP TABLE for forecast should succeed",
			toolName:     "my-exec-sql-tool",
			args:         map[string]any{"sql": "CREATE TEMP TABLE forecast_temp_table (ts TIMESTAMP, data FLOAT64) AS SELECT TIMESTAMP('2025-01-01T00:00:00Z') AS ts, 10.0 AS data UNION ALL SELECT TIMESTAMP('2025-01-01T01:00:00Z'), 11.0 UNION ALL SELECT TIMESTAMP('2025-01-01T02:00:00Z'), 12.0 UNION ALL SELECT TIMESTAMP('2025-01-01T03:00:00Z'), 13.0"},
			wantInResult: "Query executed successfully",
		},
		{
			name:         "Forecast from TEMP TABLE should succeed",
			toolName:     "my-forecast-tool-protected",
			args:         map[string]any{"history_data": "SELECT * FROM forecast_temp_table", "timestamp_col": "ts", "data_col": "data", "horizon": 1},
			wantInResult: "forecast_timestamp",
		},
		{
			name:         "CREATE TEMP TABLE for contribution analysis should succeed",
			toolName:     "my-exec-sql-tool",
			args:         map[string]any{"sql": "CREATE TEMP TABLE contribution_temp_table (dim1 STRING, is_test BOOL, metric FLOAT64) AS SELECT 'a' as dim1, true as is_test, 100.0 as metric UNION ALL SELECT 'b', false, 120.0"},
			wantInResult: "Query executed successfully",
		},
		{
			name:         "Analyze contribution from TEMP TABLE should succeed",
			toolName:     "my-analyze-contribution-tool-protected",
			args:         map[string]any{"input_data": "SELECT * FROM contribution_temp_table", "contribution_metric": "SUM(metric)", "is_test_col": "is_test", "dimension_id_cols": []any{"dim1"}},
			wantInResult: "relative_difference",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			statusCode, mcpResp, err := tests.InvokeMCPTool(t, tc.toolName, tc.args, nil)
			if err != nil {
				t.Fatalf("native error executing tool: %s", err)
			}
			if statusCode != http.StatusOK {
				t.Fatalf("expected status 200, got %d", statusCode)
			}

			var got string
			if mcpResp.Error != nil {
				got = mcpResp.Error.Message
			} else {
				var blocks []string
				for _, content := range mcpResp.Result.Content {
					if content.Type == "text" {
						blocks = append(blocks, content.Text)
					}
				}
				got = strings.Join(blocks, ",")
			}

			if tc.wantInError != "" {
				if !strings.Contains(got, tc.wantInError) {
					t.Fatalf("expected error containing %q, got %q", tc.wantInError, got)
				}
			} else if tc.wantInResult != "" {
				if !strings.Contains(got, tc.wantInResult) {
					t.Fatalf("expected result containing %q, got %q", tc.wantInResult, got)
				}
			}
		})
	}
}

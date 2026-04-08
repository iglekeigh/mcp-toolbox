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
	"fmt"
	"io"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/googleapis/genai-toolbox/internal/sources"
	"github.com/googleapis/genai-toolbox/tests"
)

type ToolTestInfo struct {
	Name          string
	Api           string
	RequestHeader map[string]string
	RequestBody   io.Reader
	Want          string
	IsErr         bool
}

type TestRunner func(t *testing.T, info ToolTestInfo)

func runBigQueryExecuteSqlToolInvokeTestCommon(t *testing.T, select1Want, invokeParamWant, tableNameParam, ddlWant string, runner TestRunner) {
	idToken, err := tests.GetGoogleIdToken(tests.ClientId)
	if err != nil {
		t.Fatalf("error getting Google ID token: %s", err)
	}

	accessToken, err := sources.GetIAMAccessToken(t.Context())
	if err != nil {
		t.Fatalf("error getting access token from ADC: %s", err)
	}
	accessToken = "Bearer " + accessToken

	invokeTcs := []ToolTestInfo{
		{
			Name:          "invoke my-exec-sql-tool without body",
			Api:           "http://127.0.0.1:5000/api/tool/my-exec-sql-tool/invoke",
			RequestHeader: map[string]string{},
			RequestBody:   bytes.NewBuffer([]byte(`{}`)),
			Want:          `parameter "sql" is required`,
			IsErr:         true,
		},
		{
			Name:          "invoke my-exec-sql-tool",
			Api:           "http://127.0.0.1:5000/api/tool/my-exec-sql-tool/invoke",
			RequestHeader: map[string]string{},
			RequestBody:   bytes.NewBuffer([]byte(`{"sql":"SELECT 1"}`)),
			Want:          select1Want,
			IsErr:         false,
		},
		{
			Name:          "invoke my-exec-sql-tool with data present in table",
			Api:           "http://127.0.0.1:5000/api/tool/my-exec-sql-tool/invoke",
			RequestHeader: map[string]string{},
			RequestBody:   bytes.NewBuffer([]byte(fmt.Sprintf("{\"sql\":\"SELECT id, name FROM %s WHERE id = 3 OR name = 'Alice' ORDER BY id\"}", tableNameParam))),
			Want:          invokeParamWant,
			IsErr:         false,
		},
		{
			Name:          "invoke my-exec-sql-tool with no matching rows",
			Api:           "http://127.0.0.1:5000/api/tool/my-exec-sql-tool/invoke",
			RequestHeader: map[string]string{},
			RequestBody:   bytes.NewBuffer([]byte(fmt.Sprintf("{\"sql\":\"SELECT * FROM %s WHERE id = 999\"}", tableNameParam))),
			Want:          `The query returned 0 rows.`,
			IsErr:         false,
		},
		{
			Name:          "invoke my-exec-sql-tool insert entry",
			Api:           "http://127.0.0.1:5000/api/tool/my-exec-sql-tool/invoke",
			RequestHeader: map[string]string{},
			RequestBody:   bytes.NewBuffer([]byte(fmt.Sprintf("{\"sql\":\"INSERT INTO %s (id, name) VALUES (4, 'test_name')\"}", tableNameParam))),
			Want:          ddlWant,
			IsErr:         false,
		},
		{
			Name:          "Invoke my-auth-exec-sql-tool with auth token",
			Api:           "http://127.0.0.1:5000/api/tool/my-auth-exec-sql-tool/invoke",
			RequestHeader: map[string]string{"my-google-auth_token": idToken},
			RequestBody:   bytes.NewBuffer([]byte(`{"sql":"SELECT 1"}`)),
			IsErr:         false,
			Want:          select1Want,
		},
		{
			Name:          "Invoke my-auth-exec-sql-tool with invalid auth token",
			Api:           "http://127.0.0.1:5000/api/tool/my-auth-exec-sql-tool/invoke",
			RequestHeader: map[string]string{"my-google-auth_token": "INVALID_TOKEN"},
			RequestBody:   bytes.NewBuffer([]byte(`{"sql":"SELECT 1"}`)),
			IsErr:         true,
			Want:          `invalid token`,
		},
		{
			Name:          "Invoke my-auth-exec-sql-tool without auth token",
			Api:           "http://127.0.0.1:5000/api/tool/my-auth-exec-sql-tool/invoke",
			RequestHeader: map[string]string{},
			RequestBody:   bytes.NewBuffer([]byte(`{"sql":"SELECT 1"}`)),
			IsErr:         true,
			Want:          `auth token is required`,
		},
		{
			Name:          "Invoke my-client-auth-exec-sql-tool with auth token",
			Api:           "http://127.0.0.1:5000/api/tool/my-client-auth-exec-sql-tool/invoke",
			RequestHeader: map[string]string{"Authorization": accessToken},
			RequestBody:   bytes.NewBuffer([]byte(`{"sql":"SELECT 1"}`)),
			Want:          select1Want,
			IsErr:         false,
		},
		{
			Name:          "Invoke my-client-auth-exec-sql-tool without auth token",
			Api:           "http://127.0.0.1:5000/api/tool/my-client-auth-exec-sql-tool/invoke",
			RequestHeader: map[string]string{},
			RequestBody:   bytes.NewBuffer([]byte(`{"sql":"SELECT 1"}`)),
			IsErr:         true,
			Want:          `Authorization header is required`,
		},
	}

	for _, tc := range invokeTcs {
		t.Run(tc.Name, func(t *testing.T) {
			runner(t, tc)
		})
	}
}

func runBigQueryForecastToolInvokeTestCommon(t *testing.T, tableName string, runner TestRunner) {
	idToken, err := tests.GetGoogleIdToken(tests.ClientId)
	if err != nil {
		t.Fatalf("error getting Google ID token: %s", err)
	}

	accessToken, err := sources.GetIAMAccessToken(t.Context())
	if err != nil {
		t.Fatalf("error getting access token from ADC: %s", err)
	}
	accessToken = "Bearer " + accessToken

	historyDataTable := strings.ReplaceAll(tableName, "`", "")
	historyDataQuery := fmt.Sprintf("SELECT ts, data, id FROM %s", tableName)

	invokeTcs := []ToolTestInfo{
		{
			Name:          "invoke my-forecast-tool without required params",
			Api:           "http://127.0.0.1:5000/api/tool/my-forecast-tool/invoke",
			RequestHeader: map[string]string{},
			RequestBody:   bytes.NewBuffer([]byte(fmt.Sprintf(`{"history_data": "%s"}`, historyDataTable))),
			IsErr:         true,
		},
		{
			Name:          "invoke my-forecast-tool with table",
			Api:           "http://127.0.0.1:5000/api/tool/my-forecast-tool/invoke",
			RequestHeader: map[string]string{},
			RequestBody:   bytes.NewBuffer([]byte(fmt.Sprintf(`{"history_data": "%s", "timestamp_col": "ts", "data_col": "data"}`, historyDataTable))),
			Want:          `"forecast_timestamp"`,
			IsErr:         false,
		},
		{
			Name:          "invoke my-forecast-tool with query and horizon",
			Api:           "http://127.0.0.1:5000/api/tool/my-forecast-tool/invoke",
			RequestHeader: map[string]string{},
			RequestBody:   bytes.NewBuffer([]byte(fmt.Sprintf(`{"history_data": "%s", "timestamp_col": "ts", "data_col": "data", "horizon": 5}`, historyDataQuery))),
			Want:          `"forecast_timestamp"`,
			IsErr:         false,
		},
		{
			Name:          "invoke my-forecast-tool with id_cols",
			Api:           "http://127.0.0.1:5000/api/tool/my-forecast-tool/invoke",
			RequestHeader: map[string]string{},
			RequestBody:   bytes.NewBuffer([]byte(fmt.Sprintf(`{"history_data": "%s", "timestamp_col": "ts", "data_col": "data", "id_cols": ["id"]}`, historyDataTable))),
			Want:          `"id"`,
			IsErr:         false,
		},
		{
			Name:          "invoke my-auth-forecast-tool with auth token",
			Api:           "http://127.0.0.1:5000/api/tool/my-auth-forecast-tool/invoke",
			RequestHeader: map[string]string{"my-google-auth_token": idToken},
			RequestBody:   bytes.NewBuffer([]byte(fmt.Sprintf(`{"history_data": "%s", "timestamp_col": "ts", "data_col": "data"}`, historyDataTable))),
			Want:          `"forecast_timestamp"`,
			IsErr:         false,
		},
		{
			Name:          "invoke my-auth-forecast-tool with invalid auth token",
			Api:           "http://127.0.0.1:5000/api/tool/my-auth-forecast-tool/invoke",
			RequestHeader: map[string]string{"my-google-auth_token": "INVALID_TOKEN"},
			RequestBody:   bytes.NewBuffer([]byte(fmt.Sprintf(`{"history_data": "%s", "timestamp_col": "ts", "data_col": "data"}`, historyDataTable))),
			IsErr:         true,
		},
		{
			Name:          "Invoke my-client-auth-forecast-tool with auth token",
			Api:           "http://127.0.0.1:5000/api/tool/my-client-auth-forecast-tool/invoke",
			RequestHeader: map[string]string{"Authorization": accessToken},
			RequestBody:   bytes.NewBuffer([]byte(fmt.Sprintf(`{"history_data": "%s", "timestamp_col": "ts", "data_col": "data"}`, historyDataTable))),
			Want:          `"forecast_timestamp"`,
			IsErr:         false,
		},
		{
			Name:          "Invoke my-client-auth-forecast-tool without auth token",
			Api:           "http://127.0.0.1:5000/api/tool/my-client-auth-forecast-tool/invoke",
			RequestHeader: map[string]string{},
			RequestBody:   bytes.NewBuffer([]byte(fmt.Sprintf(`{"history_data": "%s", "timestamp_col": "ts", "data_col": "data"}`, historyDataTable))),
			IsErr:         true,
		},
		{
			Name:          "Invoke my-client-auth-forecast-tool with invalid auth token",
			Api:           "http://127.0.0.1:5000/api/tool/my-client-auth-forecast-tool/invoke",
			RequestHeader: map[string]string{"Authorization": "Bearer invalid-token"},
			RequestBody:   bytes.NewBuffer([]byte(fmt.Sprintf(`{"history_data": "%s", "timestamp_col": "ts", "data_col": "data"}`, historyDataTable))),
			IsErr:         true,
		},
	}

	for _, tc := range invokeTcs {
		t.Run(tc.Name, func(t *testing.T) {
			runner(t, tc)
		})
	}
}

func runBigQueryAnalyzeContributionToolInvokeTestCommon(t *testing.T, tableName string, runner TestRunner) {
	idToken, err := tests.GetGoogleIdToken(tests.ClientId)
	if err != nil {
		t.Fatalf("error getting Google ID token: %s", err)
	}

	accessToken, err := sources.GetIAMAccessToken(t.Context())
	if err != nil {
		t.Fatalf("error getting access token from ADC: %s", err)
	}
	accessToken = "Bearer " + accessToken

	dataTable := strings.ReplaceAll(tableName, "`", "")

	invokeTcs := []ToolTestInfo{
		{
			Name:          "invoke my-analyze-contribution-tool without required params",
			Api:           "http://127.0.0.1:5000/api/tool/my-analyze-contribution-tool/invoke",
			RequestHeader: map[string]string{},
			RequestBody:   bytes.NewBuffer([]byte(fmt.Sprintf(`{"input_data": "%s"}`, dataTable))),
			IsErr:         true,
		},
		{
			Name:          "invoke my-analyze-contribution-tool with table",
			Api:           "http://127.0.0.1:5000/api/tool/my-analyze-contribution-tool/invoke",
			RequestHeader: map[string]string{},
			RequestBody:   bytes.NewBuffer([]byte(fmt.Sprintf(`{"input_data": "%s", "contribution_metric": "SUM(metric)", "is_test_col": "is_test", "dimension_id_cols": ["dim1", "dim2"]}`, dataTable))),
			Want:          `"relative_difference"`,
			IsErr:         false,
		},
		{
			Name:          "invoke my-auth-analyze-contribution-tool with auth token",
			Api:           "http://127.0.0.1:5000/api/tool/my-auth-analyze-contribution-tool/invoke",
			RequestHeader: map[string]string{"my-google-auth_token": idToken},
			RequestBody:   bytes.NewBuffer([]byte(fmt.Sprintf(`{"input_data": "%s", "contribution_metric": "SUM(metric)", "is_test_col": "is_test", "dimension_id_cols": ["dim1", "dim2"]}`, dataTable))),
			Want:          `"relative_difference"`,
			IsErr:         false,
		},
		{
			Name:          "invoke my-auth-analyze-contribution-tool with invalid auth token",
			Api:           "http://127.0.0.1:5000/api/tool/my-auth-analyze-contribution-tool/invoke",
			RequestHeader: map[string]string{"my-google-auth_token": "INVALID_TOKEN"},
			RequestBody:   bytes.NewBuffer([]byte(fmt.Sprintf(`{"input_data": "%s", "contribution_metric": "SUM(metric)", "is_test_col": "is_test", "dimension_id_cols": ["dim1", "dim2"]}`, dataTable))),
			IsErr:         true,
		},
		{
			Name:          "Invoke my-client-auth-analyze-contribution-tool with auth token",
			Api:           "http://127.0.0.1:5000/api/tool/my-client-auth-analyze-contribution-tool/invoke",
			RequestHeader: map[string]string{"Authorization": accessToken},
			RequestBody:   bytes.NewBuffer([]byte(fmt.Sprintf(`{"input_data": "%s", "contribution_metric": "SUM(metric)", "is_test_col": "is_test", "dimension_id_cols": ["dim1", "dim2"]}`, dataTable))),
			Want:          `"relative_difference"`,
			IsErr:         false,
		},
		{
			Name:          "Invoke my-client-auth-analyze-contribution-tool without auth token",
			Api:           "http://127.0.0.1:5000/api/tool/my-client-auth-analyze-contribution-tool/invoke",
			RequestHeader: map[string]string{},
			RequestBody:   bytes.NewBuffer([]byte(fmt.Sprintf(`{"input_data": "%s", "contribution_metric": "SUM(metric)", "is_test_col": "is_test", "dimension_id_cols": ["dim1", "dim2"]}`, dataTable))),
			IsErr:         true,
		},
		{
			Name:          "Invoke my-client-auth-analyze-contribution-tool with invalid auth token",
			Api:           "http://127.0.0.1:5000/api/tool/my-client-auth-analyze-contribution-tool/invoke",
			RequestHeader: map[string]string{"Authorization": "Bearer invalid-token"},
			RequestBody:   bytes.NewBuffer([]byte(fmt.Sprintf(`{"input_data": "%s", "contribution_metric": "SUM(metric)", "is_test_col": "is_test", "dimension_id_cols": ["dim1", "dim2"]}`, dataTable))),
			IsErr:         true,
		},
	}

	for _, tc := range invokeTcs {
		t.Run(tc.Name, func(t *testing.T) {
			runner(t, tc)
		})
	}
}

func runBigQueryListDatasetToolInvokeTestCommon(t *testing.T, datasetWant string, runner TestRunner) {
	idToken, err := tests.GetGoogleIdToken(tests.ClientId)
	if err != nil {
		t.Fatalf("error getting Google ID token: %s", err)
	}

	accessToken, err := sources.GetIAMAccessToken(t.Context())
	if err != nil {
		t.Fatalf("error getting access token from ADC: %s", err)
	}
	accessToken = "Bearer " + accessToken

	invokeTcs := []ToolTestInfo{
		{
			Name:          "invoke my-list-dataset-ids-tool",
			Api:           "http://127.0.0.1:5000/api/tool/my-list-dataset-ids-tool/invoke",
			RequestHeader: map[string]string{},
			RequestBody:   bytes.NewBuffer([]byte(`{}`)),
			IsErr:         false,
			Want:          datasetWant,
		},
		{
			Name:          "invoke my-list-dataset-ids-tool with project",
			Api:           "http://127.0.0.1:5000/api/tool/my-auth-list-dataset-ids-tool/invoke",
			RequestHeader: map[string]string{"my-google-auth_token": idToken},
			RequestBody:   bytes.NewBuffer([]byte(fmt.Sprintf("{\"project\":\"%s\"}", BigqueryProject))),
			IsErr:         false,
			Want:          datasetWant,
		},
		{
			Name:          "invoke my-list-dataset-ids-tool with non-existent project",
			Api:           "http://127.0.0.1:5000/api/tool/my-auth-list-dataset-ids-tool/invoke",
			RequestHeader: map[string]string{"my-google-auth_token": idToken},
			RequestBody:   bytes.NewBuffer([]byte(fmt.Sprintf("{\"project\":\"%s-%s\"}", BigqueryProject, uuid.NewString()))),
			IsErr:         true,
		},
		{
			Name:          "invoke my-auth-list-dataset-ids-tool",
			Api:           "http://127.0.0.1:5000/api/tool/my-auth-list-dataset-ids-tool/invoke",
			RequestHeader: map[string]string{"my-google-auth_token": idToken},
			RequestBody:   bytes.NewBuffer([]byte(`{}`)),
			IsErr:         false,
			Want:          datasetWant,
		},
		{
			Name:          "Invoke my-client-auth-list-dataset-ids-tool with auth token",
			Api:           "http://127.0.0.1:5000/api/tool/my-client-auth-list-dataset-ids-tool/invoke",
			RequestHeader: map[string]string{"Authorization": accessToken},
			RequestBody:   bytes.NewBuffer([]byte(`{}`)),
			IsErr:         false,
			Want:          datasetWant,
		},
		{
			Name:          "Invoke my-client-auth-list-dataset-ids-tool without auth token",
			Api:           "http://127.0.0.1:5000/api/tool/my-client-auth-list-dataset-ids-tool/invoke",
			RequestHeader: map[string]string{},
			RequestBody:   bytes.NewBuffer([]byte(`{}`)),
			IsErr:         true,
		},
		{
			Name:          "Invoke my-client-auth-list-dataset-ids-tool with invalid auth token",
			Api:           "http://127.0.0.1:5000/api/tool/my-client-auth-list-dataset-ids-tool/invoke",
			RequestHeader: map[string]string{"Authorization": "Bearer invalid-token"},
			RequestBody:   bytes.NewBuffer([]byte(`{}`)),
			IsErr:         true,
		},
	}

	for _, tc := range invokeTcs {
		t.Run(tc.Name, func(t *testing.T) {
			runner(t, tc)
		})
	}
}

func runBigQueryGetDatasetInfoToolInvokeTestCommon(t *testing.T, datasetName, datasetInfoWant string, runner TestRunner) {
	idToken, err := tests.GetGoogleIdToken(tests.ClientId)
	if err != nil {
		t.Fatalf("error getting Google ID token: %s", err)
	}

	accessToken, err := sources.GetIAMAccessToken(t.Context())
	if err != nil {
		t.Fatalf("error getting access token from ADC: %s", err)
	}
	accessToken = "Bearer " + accessToken

	invokeTcs := []ToolTestInfo{
		{
			Name:          "invoke my-get-dataset-info-tool without body",
			Api:           "http://127.0.0.1:5000/api/tool/my-get-dataset-info-tool/invoke",
			RequestHeader: map[string]string{},
			RequestBody:   bytes.NewBuffer([]byte(`{}`)),
			IsErr:         true,
		},
		{
			Name:          "invoke my-get-dataset-info-tool",
			Api:           "http://127.0.0.1:5000/api/tool/my-get-dataset-info-tool/invoke",
			RequestHeader: map[string]string{},
			RequestBody:   bytes.NewBuffer([]byte(fmt.Sprintf("{\"dataset\":\"%s\"}", datasetName))),
			Want:          datasetInfoWant,
			IsErr:         false,
		},
		{
			Name:          "Invoke my-auth-get-dataset-info-tool with correct project",
			Api:           "http://127.0.0.1:5000/api/tool/my-auth-get-dataset-info-tool/invoke",
			RequestHeader: map[string]string{"my-google-auth_token": idToken},
			RequestBody:   bytes.NewBuffer([]byte(fmt.Sprintf("{\"project\":\"%s\", \"dataset\":\"%s\"}", BigqueryProject, datasetName))),
			Want:          datasetInfoWant,
			IsErr:         false,
		},
		{
			Name:          "Invoke my-auth-get-dataset-info-tool with non-existent project",
			Api:           "http://127.0.0.1:5000/api/tool/my-auth-get-dataset-info-tool/invoke",
			RequestHeader: map[string]string{"my-google-auth_token": idToken},
			RequestBody:   bytes.NewBuffer([]byte(fmt.Sprintf("{\"project\":\"%s-%s\", \"dataset\":\"%s\"}", BigqueryProject, uuid.NewString(), datasetName))),
			IsErr:         true,
		},
		{
			Name:          "invoke my-auth-get-dataset-info-tool without body",
			Api:           "http://127.0.0.1:5000/api/tool/my-get-dataset-info-tool/invoke",
			RequestHeader: map[string]string{},
			RequestBody:   bytes.NewBuffer([]byte(`{}`)),
			IsErr:         true,
		},
		{
			Name:          "Invoke my-auth-get-dataset-info-tool with auth token",
			Api:           "http://127.0.0.1:5000/api/tool/my-auth-get-dataset-info-tool/invoke",
			RequestHeader: map[string]string{"my-google-auth_token": idToken},
			RequestBody:   bytes.NewBuffer([]byte(fmt.Sprintf("{\"dataset\":\"%s\"}", datasetName))),
			Want:          datasetInfoWant,
			IsErr:         false,
		},
		{
			Name:          "Invoke my-auth-get-dataset-info-tool with invalid auth token",
			Api:           "http://127.0.0.1:5000/api/tool/my-auth-get-dataset-info-tool/invoke",
			RequestHeader: map[string]string{"my-google-auth_token": "INVALID_TOKEN"},
			RequestBody:   bytes.NewBuffer([]byte(fmt.Sprintf("{\"dataset\":\"%s\"}", datasetName))),
			IsErr:         true,
		},
		{
			Name:          "Invoke my-auth-get-dataset-info-tool without auth token",
			Api:           "http://127.0.0.1:5000/api/tool/my-auth-get-dataset-info-tool/invoke",
			RequestHeader: map[string]string{},
			RequestBody:   bytes.NewBuffer([]byte(fmt.Sprintf("{\"dataset\":\"%s\"}", datasetName))),
			IsErr:         true,
		},
		{
			Name:          "Invoke my-client-auth-get-dataset-info-tool with auth token",
			Api:           "http://127.0.0.1:5000/api/tool/my-client-auth-get-dataset-info-tool/invoke",
			RequestHeader: map[string]string{"Authorization": accessToken},
			RequestBody:   bytes.NewBuffer([]byte(fmt.Sprintf("{\"dataset\":\"%s\"}", datasetName))),
			Want:          datasetInfoWant,
			IsErr:         false,
		},
		{
			Name:          "Invoke my-client-auth-get-dataset-info-tool without auth token",
			Api:           "http://127.0.0.1:5000/api/tool/my-client-auth-get-dataset-info-tool/invoke",
			RequestHeader: map[string]string{},
			RequestBody:   bytes.NewBuffer([]byte(fmt.Sprintf("{\"dataset\":\"%s\"}", datasetName))),
			IsErr:         true,
		},
		{
			Name:          "Invoke my-client-auth-get-dataset-info-tool with invalid auth token",
			Api:           "http://127.0.0.1:5000/api/tool/my-client-auth-get-dataset-info-tool/invoke",
			RequestHeader: map[string]string{"Authorization": "Bearer invalid-token"},
			RequestBody:   bytes.NewBuffer([]byte(fmt.Sprintf("{\"dataset\":\"%s\"}", datasetName))),
			IsErr:         true,
		},
	}

	for _, tc := range invokeTcs {
		t.Run(tc.Name, func(t *testing.T) {
			runner(t, tc)
		})
	}
}

func runBigQueryListTableIdsToolInvokeTestCommon(t *testing.T, datasetName, tablename_want string, runner TestRunner) {
	idToken, err := tests.GetGoogleIdToken(tests.ClientId)
	if err != nil {
		t.Fatalf("error getting Google ID token: %s", err)
	}

	accessToken, err := sources.GetIAMAccessToken(t.Context())
	if err != nil {
		t.Fatalf("error getting access token from ADC: %s", err)
	}
	accessToken = "Bearer " + accessToken

	invokeTcs := []ToolTestInfo{
		{
			Name:          "invoke my-list-table-ids-tool without body",
			Api:           "http://127.0.0.1:5000/api/tool/my-list-table-ids-tool/invoke",
			RequestHeader: map[string]string{},
			RequestBody:   bytes.NewBuffer([]byte(`{}`)),
			IsErr:         true,
		},
		{
			Name:          "invoke my-list-table-ids-tool",
			Api:           "http://127.0.0.1:5000/api/tool/my-list-table-ids-tool/invoke",
			RequestHeader: map[string]string{},
			RequestBody:   bytes.NewBuffer([]byte(fmt.Sprintf("{\"dataset\":\"%s\"}", datasetName))),
			Want:          tablename_want,
			IsErr:         false,
		},
		{
			Name:          "Invoke my-auth-list-table-ids-tool with auth token",
			Api:           "http://127.0.0.1:5000/api/tool/my-auth-list-table-ids-tool/invoke",
			RequestHeader: map[string]string{"my-google-auth_token": idToken},
			RequestBody:   bytes.NewBuffer([]byte(fmt.Sprintf("{\"dataset\":\"%s\"}", datasetName))),
			Want:          tablename_want,
			IsErr:         false,
		},
		{
			Name:          "Invoke my-auth-list-table-ids-tool with correct project",
			Api:           "http://127.0.0.1:5000/api/tool/my-auth-list-table-ids-tool/invoke",
			RequestHeader: map[string]string{"my-google-auth_token": idToken},
			RequestBody:   bytes.NewBuffer([]byte(fmt.Sprintf("{\"project\":\"%s\", \"dataset\":\"%s\"}", BigqueryProject, datasetName))),
			Want:          tablename_want,
			IsErr:         false,
		},
		{
			Name:          "Invoke my-auth-list-table-ids-tool with non-existent project",
			Api:           "http://127.0.0.1:5000/api/tool/my-auth-list-table-ids-tool/invoke",
			RequestHeader: map[string]string{"my-google-auth_token": idToken},
			RequestBody:   bytes.NewBuffer([]byte(fmt.Sprintf("{\"project\":\"%s-%s\", \"dataset\":\"%s\"}", BigqueryProject, uuid.NewString(), datasetName))),
			IsErr:         true,
		},
		{
			Name:          "Invoke my-auth-list-table-ids-tool with invalid auth token",
			Api:           "http://127.0.0.1:5000/api/tool/my-auth-list-table-ids-tool/invoke",
			RequestHeader: map[string]string{"my-google-auth_token": "INVALID_TOKEN"},
			RequestBody:   bytes.NewBuffer([]byte(fmt.Sprintf("{\"dataset\":\"%s\"}", datasetName))),
			IsErr:         true,
		},
		{
			Name:          "Invoke my-auth-list-table-ids-tool without auth token",
			Api:           "http://127.0.0.1:5000/api/tool/my-auth-list-table-ids-tool/invoke",
			RequestHeader: map[string]string{},
			RequestBody:   bytes.NewBuffer([]byte(fmt.Sprintf("{\"dataset\":\"%s\"}", datasetName))),
			IsErr:         true,
		},
		{
			Name:          "Invoke my-client-auth-list-table-ids-tool with auth token",
			Api:           "http://127.0.0.1:5000/api/tool/my-client-auth-list-table-ids-tool/invoke",
			RequestHeader: map[string]string{"Authorization": accessToken},
			RequestBody:   bytes.NewBuffer([]byte(fmt.Sprintf("{\"dataset\":\"%s\"}", datasetName))),
			Want:          tablename_want,
			IsErr:         false,
		},
		{
			Name:          "Invoke my-client-auth-list-table-ids-tool without auth token",
			Api:           "http://127.0.0.1:5000/api/tool/my-client-auth-list-table-ids-tool/invoke",
			RequestHeader: map[string]string{},
			RequestBody:   bytes.NewBuffer([]byte(fmt.Sprintf("{\"dataset\":\"%s\"}", datasetName))),
			IsErr:         true,
		},
		{
			Name:          "Invoke my-client-auth-list-table-ids-tool with invalid auth token",
			Api:           "http://127.0.0.1:5000/api/tool/my-client-auth-list-table-ids-tool/invoke",
			RequestHeader: map[string]string{"Authorization": "Bearer invalid-token"},
			RequestBody:   bytes.NewBuffer([]byte(fmt.Sprintf("{\"dataset\":\"%s\"}", datasetName))),
			IsErr:         true,
		},
	}

	for _, tc := range invokeTcs {
		t.Run(tc.Name, func(t *testing.T) {
			runner(t, tc)
		})
	}
}

func runBigQueryGetTableInfoToolInvokeTestCommon(t *testing.T, datasetName, tableName, tableInfoWant string, runner TestRunner) {
	idToken, err := tests.GetGoogleIdToken(tests.ClientId)
	if err != nil {
		t.Fatalf("error getting Google ID token: %s", err)
	}

	accessToken, err := sources.GetIAMAccessToken(t.Context())
	if err != nil {
		t.Fatalf("error getting access token from ADC: %s", err)
	}
	accessToken = "Bearer " + accessToken

	invokeTcs := []ToolTestInfo{
		{
			Name:          "invoke my-get-table-info-tool without body",
			Api:           "http://127.0.0.1:5000/api/tool/my-get-table-info-tool/invoke",
			RequestHeader: map[string]string{},
			RequestBody:   bytes.NewBuffer([]byte(`{}`)),
			IsErr:         true,
		},
		{
			Name:          "invoke my-get-table-info-tool",
			Api:           "http://127.0.0.1:5000/api/tool/my-get-table-info-tool/invoke",
			RequestHeader: map[string]string{},
			RequestBody:   bytes.NewBuffer([]byte(fmt.Sprintf("{\"dataset\":\"%s\", \"table\":\"%s\"}", datasetName, tableName))),
			Want:          tableInfoWant,
			IsErr:         false,
		},
		{
			Name:          "invoke my-auth-get-table-info-tool without body",
			Api:           "http://127.0.0.1:5000/api/tool/my-get-table-info-tool/invoke",
			RequestHeader: map[string]string{},
			RequestBody:   bytes.NewBuffer([]byte(`{}`)),
			IsErr:         true,
		},
		{
			Name:          "Invoke my-auth-get-table-info-tool with auth token",
			Api:           "http://127.0.0.1:5000/api/tool/my-auth-get-table-info-tool/invoke",
			RequestHeader: map[string]string{"my-google-auth_token": idToken},
			RequestBody:   bytes.NewBuffer([]byte(fmt.Sprintf("{\"dataset\":\"%s\", \"table\":\"%s\"}", datasetName, tableName))),
			Want:          tableInfoWant,
			IsErr:         false,
		},
		{
			Name:          "Invoke my-auth-get-table-info-tool with correct project",
			Api:           "http://127.0.0.1:5000/api/tool/my-auth-get-table-info-tool/invoke",
			RequestHeader: map[string]string{"my-google-auth_token": idToken},
			RequestBody:   bytes.NewBuffer([]byte(fmt.Sprintf("{\"project\":\"%s\", \"dataset\":\"%s\", \"table\":\"%s\"}", BigqueryProject, datasetName, tableName))),
			Want:          tableInfoWant,
			IsErr:         false,
		},
		{
			Name:          "Invoke my-auth-get-table-info-tool with non-existent project",
			Api:           "http://127.0.0.1:5000/api/tool/my-auth-get-table-info-tool/invoke",
			RequestHeader: map[string]string{"my-google-auth_token": idToken},
			RequestBody:   bytes.NewBuffer([]byte(fmt.Sprintf("{\"project\":\"%s-%s\", \"dataset\":\"%s\", \"table\":\"%s\"}", BigqueryProject, uuid.NewString(), datasetName, tableName))),
			IsErr:         true,
		},
		{
			Name:          "Invoke my-auth-get-table-info-tool with invalid auth token",
			Api:           "http://127.0.0.1:5000/api/tool/my-auth-get-table-info-tool/invoke",
			RequestHeader: map[string]string{"my-google-auth_token": "INVALID_TOKEN"},
			RequestBody:   bytes.NewBuffer([]byte(fmt.Sprintf("{\"dataset\":\"%s\", \"table\":\"%s\"}", datasetName, tableName))),
			IsErr:         true,
		},
		{
			Name:          "Invoke my-auth-get-table-info-tool without auth token",
			Api:           "http://127.0.0.1:5000/api/tool/my-auth-get-table-info-tool/invoke",
			RequestHeader: map[string]string{},
			RequestBody:   bytes.NewBuffer([]byte(fmt.Sprintf("{\"dataset\":\"%s\", \"table\":\"%s\"}", datasetName, tableName))),
			IsErr:         true,
		},
		{
			Name:          "Invoke my-client-auth-get-table-info-tool with auth token",
			Api:           "http://127.0.0.1:5000/api/tool/my-client-auth-get-table-info-tool/invoke",
			RequestHeader: map[string]string{"Authorization": accessToken},
			RequestBody:   bytes.NewBuffer([]byte(fmt.Sprintf("{\"dataset\":\"%s\", \"table\":\"%s\"}", datasetName, tableName))),
			Want:          tableInfoWant,
			IsErr:         false,
		},
		{
			Name:          "Invoke my-client-auth-get-table-info-tool without auth token",
			Api:           "http://127.0.0.1:5000/api/tool/my-client-auth-get-table-info-tool/invoke",
			RequestHeader: map[string]string{},
			RequestBody:   bytes.NewBuffer([]byte(fmt.Sprintf("{\"dataset\":\"%s\", \"table\":\"%s\"}", datasetName, tableName))),
			IsErr:         true,
		},
		{
			Name:          "Invoke my-client-auth-get-table-info-tool with invalid auth token",
			Api:           "http://127.0.0.1:5000/api/tool/my-client-auth-get-table-info-tool/invoke",
			RequestHeader: map[string]string{"Authorization": "Bearer invalid-token"},
			RequestBody:   bytes.NewBuffer([]byte(fmt.Sprintf("{\"dataset\":\"%s\", \"table\":\"%s\"}", datasetName, tableName))),
			IsErr:         true,
		},
	}

	for _, tc := range invokeTcs {
		t.Run(tc.Name, func(t *testing.T) {
			runner(t, tc)
		})
	}
}

func runBigQueryConversationalAnalyticsInvokeTestCommon(t *testing.T, datasetName, tableName, dataInsightsWant string, runner TestRunner) {
	idToken, err := tests.GetGoogleIdToken(tests.ClientId)
	if err != nil {
		t.Fatalf("error getting Google ID token: %s", err)
	}

	accessToken, err := sources.GetIAMAccessToken(t.Context())
	if err != nil {
		t.Fatalf("error getting access token from ADC: %s", err)
	}
	accessToken = "Bearer " + accessToken

	tableRefsJSON := fmt.Sprintf(`[{"projectId":"%s","datasetId":"%s","tableId":"%s"}]`, BigqueryProject, datasetName, tableName)

	invokeTcs := []ToolTestInfo{
		{
			Name:          "invoke my-conversational-analytics-tool successfully",
			Api:           "http://127.0.0.1:5000/api/tool/my-conversational-analytics-tool/invoke",
			RequestHeader: map[string]string{},
			RequestBody: bytes.NewBuffer([]byte(fmt.Sprintf(
				`{"user_query_with_context": "What are the names in the table?", "table_references": %q}`,
				tableRefsJSON,
			))),
			Want:  dataInsightsWant,
			IsErr: false,
		},
		{
			Name:          "invoke my-auth-conversational-analytics-tool with auth token",
			Api:           "http://127.0.0.1:5000/api/tool/my-auth-conversational-analytics-tool/invoke",
			RequestHeader: map[string]string{"my-google-auth_token": idToken},
			RequestBody: bytes.NewBuffer([]byte(fmt.Sprintf(
				`{"user_query_with_context": "What are the names in the table?", "table_references": %q}`,
				tableRefsJSON,
			))),
			Want:  dataInsightsWant,
			IsErr: false,
		},
		{
			Name:          "invoke my-auth-conversational-analytics-tool without auth token",
			Api:           "http://127.0.0.1:5000/api/tool/my-auth-conversational-analytics-tool/invoke",
			RequestHeader: map[string]string{},
			RequestBody:   bytes.NewBuffer([]byte(`{"user_query_with_context": "What are the names in the table?"}`)),
			IsErr:         true,
		},
		{
			Name:          "Invoke my-client-auth-conversational-analytics-tool with auth token",
			Api:           "http://127.0.0.1:5000/api/tool/my-client-auth-conversational-analytics-tool/invoke",
			RequestHeader: map[string]string{"Authorization": accessToken},
			RequestBody: bytes.NewBuffer([]byte(fmt.Sprintf(
				`{"user_query_with_context": "What are the names in the table?", "table_references": %q}`,
				tableRefsJSON,
			))),
			Want:  dataInsightsWant,
			IsErr: false,
		},
		{
			Name:          "Invoke my-client-auth-conversational-analytics-tool without auth token",
			Api:           "http://127.0.0.1:5000/api/tool/my-client-auth-conversational-analytics-tool/invoke",
			RequestHeader: map[string]string{},
			RequestBody: bytes.NewBuffer([]byte(fmt.Sprintf(
				`{"user_query_with_context": "What are the names in the table?", "table_references": %q}`,
				tableRefsJSON,
			))),
			IsErr: true,
		},
		{
			Name:          "Invoke my-client-auth-conversational-analytics-tool with invalid auth token",
			Api:           "http://127.0.0.1:5000/api/tool/my-client-auth-conversational-analytics-tool/invoke",
			RequestHeader: map[string]string{"Authorization": "Bearer invalid-token"},
			RequestBody: bytes.NewBuffer([]byte(fmt.Sprintf(
				`{"user_query_with_context": "What are the names in the table?", "table_references": %q}`,
				tableRefsJSON,
			))),
			IsErr: true,
		},
	}

	for _, tc := range invokeTcs {
		t.Run(tc.Name, func(t *testing.T) {
			runner(t, tc)
		})
	}
}

func runBigQuerySearchCatalogToolInvokeTestCommon(t *testing.T, datasetName string, tableName string, runner TestRunner) {
	idToken, err := tests.GetGoogleIdToken(tests.ClientId)
	if err != nil {
		t.Fatalf("error getting Google ID token: %s", err)
	}

	accessToken, err := sources.GetIAMAccessToken(t.Context())
	if err != nil {
		t.Fatalf("error getting access token from ADC: %s", err)
	}
	accessToken = "Bearer " + accessToken

	invokeTcs := []ToolTestInfo{
		{
			Name:          "invoke my-search-catalog-tool without body",
			Api:           "http://127.0.0.1:5000/api/tool/my-search-catalog-tool/invoke",
			RequestHeader: map[string]string{},
			RequestBody:   bytes.NewBuffer([]byte(`{}`)),
			IsErr:         true,
		},
		{
			Name:          "invoke my-search-catalog-tool",
			Api:           "http://127.0.0.1:5000/api/tool/my-search-catalog-tool/invoke",
			RequestHeader: map[string]string{},
			RequestBody:   bytes.NewBuffer([]byte(fmt.Sprintf("{\"prompt\":\"%s\", \"types\":[\"TABLE\"], \"datasetIds\":[\"%s\"]}", tableName, datasetName))),
			Want:          "DisplayName",
			IsErr:         false,
		},
		{
			Name:          "Invoke my-auth-search-catalog-tool with auth token",
			Api:           "http://127.0.0.1:5000/api/tool/my-auth-search-catalog-tool/invoke",
			RequestHeader: map[string]string{"my-google-auth_token": idToken},
			RequestBody:   bytes.NewBuffer([]byte(fmt.Sprintf("{\"prompt\":\"%s\", \"types\":[\"TABLE\"], \"datasetIds\":[\"%s\"]}", tableName, datasetName))),
			Want:          "DisplayName",
			IsErr:         false,
		},
		{
			Name:          "Invoke my-auth-search-catalog-tool with correct project",
			Api:           "http://127.0.0.1:5000/api/tool/my-auth-search-catalog-tool/invoke",
			RequestHeader: map[string]string{"my-google-auth_token": idToken},
			RequestBody:   bytes.NewBuffer([]byte(fmt.Sprintf("{\"prompt\":\"%s\", \"types\":[\"TABLE\"], \"projectIds\":[\"%s\"], \"datasetIds\":[\"%s\"]}", tableName, BigqueryProject, datasetName))),
			Want:          "DisplayName",
			IsErr:         false,
		},
		{
			Name:          "Invoke my-auth-search-catalog-tool with non-existent project",
			Api:           "http://127.0.0.1:5000/api/tool/my-auth-search-catalog-tool/invoke",
			RequestHeader: map[string]string{"my-google-auth_token": idToken},
			RequestBody:   bytes.NewBuffer([]byte(fmt.Sprintf("{\"prompt\":\"%s\", \"types\":[\"TABLE\"], \"projectIds\":[\"%s-%s\"], \"datasetIds\":[\"%s\"]}", tableName, BigqueryProject, uuid.NewString(), datasetName))),
			IsErr:         true,
		},
		{
			Name:          "Invoke my-auth-search-catalog-tool with invalid auth token",
			Api:           "http://127.0.0.1:5000/api/tool/my-auth-search-catalog-tool/invoke",
			RequestHeader: map[string]string{"my-google-auth_token": "INVALID_TOKEN"},
			RequestBody:   bytes.NewBuffer([]byte(fmt.Sprintf("{\"prompt\":\"%s\", \"types\":[\"TABLE\"], \"datasetIds\":[\"%s\"]}", tableName, datasetName))),
			IsErr:         true,
		},
		{
			Name:          "Invoke my-auth-search-catalog-tool without auth token",
			Api:           "http://127.0.0.1:5000/api/tool/my-auth-search-catalog-tool/invoke",
			RequestHeader: map[string]string{},
			RequestBody:   bytes.NewBuffer([]byte(fmt.Sprintf("{\"prompt\":\"%s\", \"types\":[\"TABLE\"], \"datasetIds\":[\"%s\"]}", tableName, datasetName))),
			IsErr:         true,
		},
		{
			Name:          "Invoke my-client-auth-search-catalog-tool without auth token",
			Api:           "http://127.0.0.1:5000/api/tool/my-client-auth-search-catalog-tool/invoke",
			RequestHeader: map[string]string{},
			RequestBody:   bytes.NewBuffer([]byte(fmt.Sprintf("{\"prompt\":\"%s\", \"types\":[\"TABLE\"], \"datasetIds\":[\"%s\"]}", tableName, datasetName))),
			IsErr:         true,
		},
		{
			Name:          "Invoke my-client-auth-search-catalog-tool with auth token",
			Api:           "http://127.0.0.1:5000/api/tool/my-client-auth-search-catalog-tool/invoke",
			RequestHeader: map[string]string{"Authorization": accessToken},
			RequestBody:   bytes.NewBuffer([]byte(fmt.Sprintf("{\"prompt\":\"%s\", \"types\":[\"TABLE\"], \"datasetIds\":[\"%s\"]}", tableName, datasetName))),
			Want:          "DisplayName",
			IsErr:         false,
		},
	}

	for _, tc := range invokeTcs {
		t.Run(tc.Name, func(t *testing.T) {
			runner(t, tc)
		})
	}
}

func runBigQueryDataTypeTestsCommon(t *testing.T, runner TestRunner) {
	invokeTcs := []ToolTestInfo{
		{
			Name:          "invoke my-scalar-datatype-tool with values",
			Api:           "http://127.0.0.1:5000/api/tool/my-scalar-datatype-tool/invoke",
			RequestHeader: map[string]string{},
			RequestBody:   bytes.NewBuffer([]byte(`{"int_val": 123, "string_val": "hello", "float_val": 3.14, "bool_val": true}`)),
			Want:          `[{"id":1,"int_val":123,"string_val":"hello","float_val":3.14,"bool_val":true}]`,
			IsErr:         false,
		},
		{
			Name:          "invoke my-scalar-datatype-tool with missing params",
			Api:           "http://127.0.0.1:5000/api/tool/my-scalar-datatype-tool/invoke",
			RequestHeader: map[string]string{},
			RequestBody:   bytes.NewBuffer([]byte(`{"int_val": 123}`)),
			Want:          `{"error":"parameter \"string_val\" is required"}`,
			IsErr:         false,
		},
		{
			Name:          "invoke my-array-datatype-tool",
			Api:           "http://127.0.0.1:5000/api/tool/my-array-datatype-tool/invoke",
			RequestHeader: map[string]string{},
			RequestBody:   bytes.NewBuffer([]byte(`{"int_array": [123, 789], "string_array": ["hello", "test"], "float_array": [3.14, 100.1], "bool_array": [true]}`)),
			Want:          `[{"id":1,"int_val":123,"string_val":"hello","float_val":3.14,"bool_val":true},{"id":3,"int_val":789,"string_val":"test","float_val":100.1,"bool_val":true}]`,
			IsErr:         false,
		},
	}

	for _, tc := range invokeTcs {
		t.Run(tc.Name, func(t *testing.T) {
			runner(t, tc)
		})
	}
}

func runBigQueryExecuteSqlToolInvokeDryRunTestCommon(t *testing.T, datasetName string, runner TestRunner) {
	idToken, err := tests.GetGoogleIdToken(tests.ClientId)
	if err != nil {
		t.Fatalf("error getting Google ID token: %s", err)
	}

	newTableName := fmt.Sprintf("%s.new_dry_run_table_%s", datasetName, strings.ReplaceAll(uuid.New().String(), "-", ""))

	invokeTcs := []ToolTestInfo{
		{
			Name:          "invoke my-exec-sql-tool with dryRun",
			Api:           "http://127.0.0.1:5000/api/tool/my-exec-sql-tool/invoke",
			RequestHeader: map[string]string{},
			RequestBody:   bytes.NewBuffer([]byte(`{"sql":"SELECT 1", "dry_run": true}`)),
			Want:          `\"statementType\": \"SELECT\"`,
			IsErr:         false,
		},
		{
			Name:          "invoke my-exec-sql-tool with dryRun create table",
			Api:           "http://127.0.0.1:5000/api/tool/my-exec-sql-tool/invoke",
			RequestHeader: map[string]string{},
			RequestBody:   bytes.NewBuffer([]byte(fmt.Sprintf(`{"sql":"CREATE TABLE %s (id INT64, name STRING)", "dry_run": true}`, newTableName))),
			Want:          `\"statementType\": \"CREATE_TABLE\"`,
			IsErr:         false,
		},
		{
			Name:          "invoke my-exec-sql-tool with dryRun execute immediate",
			Api:           "http://127.0.0.1:5000/api/tool/my-exec-sql-tool/invoke",
			RequestHeader: map[string]string{},
			RequestBody:   bytes.NewBuffer([]byte(fmt.Sprintf(`{"sql":"EXECUTE IMMEDIATE \"CREATE TABLE %s (id INT64, name STRING)\"", "dry_run": true}`, newTableName))),
			Want:          `\"statementType\": \"SCRIPT\"`,
			IsErr:         false,
		},
		{
			Name:          "Invoke my-auth-exec-sql-tool with dryRun and auth token",
			Api:           "http://127.0.0.1:5000/api/tool/my-auth-exec-sql-tool/invoke",
			RequestHeader: map[string]string{"my-google-auth_token": idToken},
			RequestBody:   bytes.NewBuffer([]byte(`{"sql":"SELECT 1", "dry_run": true}`)),
			IsErr:         false,
			Want:          `\"statementType\": \"SELECT\"`,
		},
		{
			Name:          "Invoke my-auth-exec-sql-tool with dryRun and invalid auth token",
			Api:           "http://127.0.0.1:5000/api/tool/my-auth-exec-sql-tool/invoke",
			RequestHeader: map[string]string{"my-google-auth_token": "INVALID_TOKEN"},
			RequestBody:   bytes.NewBuffer([]byte(`{"sql":"SELECT 1","dry_run": true}`)),
			IsErr:         true,
		},
		{
			Name:          "Invoke my-auth-exec-sql-tool with dryRun and without auth token",
			Api:           "http://127.0.0.1:5000/api/tool/my-auth-exec-sql-tool/invoke",
			RequestHeader: map[string]string{},
			RequestBody:   bytes.NewBuffer([]byte(`{"sql":"SELECT 1", "dry_run": true}`)),
			IsErr:         true,
		},
	}

	for _, tc := range invokeTcs {
		t.Run(tc.Name, func(t *testing.T) {
			runner(t, tc)
		})
	}
}

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

package sources_test

import (
	"context"
	"testing"

	"github.com/googleapis/mcp-toolbox/internal/sources"
	"go.opentelemetry.io/otel/trace"
)

type mockSourceConfig struct{}

func (m mockSourceConfig) SourceConfigType() string {
	return "mock-type"
}

func (m mockSourceConfig) Initialize(ctx context.Context, tracer trace.Tracer) (sources.Source, error) {
	return nil, nil
}

func TestMetadataSource(t *testing.T) {
	cfg := mockSourceConfig{}
	src := sources.MetadataSource{Config: cfg}

	if src.SourceType() != "mock-type" {
		t.Errorf("expected SourceType() to be 'mock-type', got %q", src.SourceType())
	}

	if src.ToConfig() != cfg {
		t.Errorf("expected ToConfig() to return the original config")
	}

	if src.GetDefaultProject() != "" {
		t.Errorf("expected GetDefaultProject() to be empty, got %q", src.GetDefaultProject())
	}

	if src.UseClientAuthorization() {
		t.Errorf("expected UseClientAuthorization() to be false")
	}

	if src.GetAuthTokenHeaderName() != "Authorization" {
		t.Errorf("expected GetAuthTokenHeaderName() to be 'Authorization', got %q", src.GetAuthTokenHeaderName())
	}

	res, err := src.Query(context.Background(), "SELECT 1")
	if err != nil || res != nil {
		t.Errorf("expected Query to return (nil, nil), got (%v, %v)", res, err)
	}

	res2, err := src.RunSQL(context.Background(), "SELECT 1", nil)
	if err != nil || res2 != nil {
		t.Errorf("expected RunSQL to return (nil, nil), got (%v, %v)", res2, err)
	}
}

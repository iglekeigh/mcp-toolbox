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

package embeddingmodels

import (
	"context"
	"testing"
)

type mockEmbeddingModelConfig struct{}

func (m mockEmbeddingModelConfig) EmbeddingModelConfigType() string {
	return "mock-model-type"
}

func (m mockEmbeddingModelConfig) Initialize(context.Context) (EmbeddingModel, error) {
	return nil, nil
}

func TestMetadataEmbeddingModel(t *testing.T) {
	mockCfg := mockEmbeddingModelConfig{}
	model := MetadataEmbeddingModel{Config: mockCfg}

	if model.EmbeddingModelType() != "mock-model-type" {
		t.Errorf("expected EmbeddingModelType to be 'mock-model-type', got %q", model.EmbeddingModelType())
	}

	if model.ToConfig() != mockCfg {
		t.Errorf("expected ToConfig to return the provided config")
	}

	embeddings, err := model.EmbedParameters(context.Background(), []string{"test"})
	if embeddings != nil {
		t.Errorf("expected EmbedParameters to return nil embeddings, got %v", embeddings)
	}
	if err != nil {
		t.Errorf("expected EmbedParameters to return nil error, got %v", err)
	}
}

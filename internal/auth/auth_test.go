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

package auth

import (
	"context"
	"net/http"
	"testing"
)

type mockAuthServiceConfig struct{}

func (m mockAuthServiceConfig) AuthServiceConfigType() string {
	return "mock-auth-type"
}

func (m mockAuthServiceConfig) Initialize() (AuthService, error) {
	return nil, nil
}

func TestMetadataAuthService(t *testing.T) {
	mockCfg := mockAuthServiceConfig{}
	svc := MetadataAuthService{Config: mockCfg}

	if svc.AuthServiceType() != "mock-auth-type" {
		t.Errorf("expected AuthServiceType to be 'mock-auth-type', got %q", svc.AuthServiceType())
	}

	if svc.GetName() != "" {
		t.Errorf("expected GetName to be empty, got %q", svc.GetName())
	}

	claims, err := svc.GetClaimsFromHeader(context.Background(), http.Header{})
	if claims != nil {
		t.Errorf("expected GetClaimsFromHeader to return nil claims, got %v", claims)
	}
	if err != nil {
		t.Errorf("expected GetClaimsFromHeader to return nil error, got %v", err)
	}

	if svc.ToConfig() != mockCfg {
		t.Errorf("expected ToConfig to return the provided config")
	}
}

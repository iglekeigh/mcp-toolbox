// Copyright 2024 Google LLC
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

package tools

import (
	"fmt"
	"regexp"
)

type ToolsetConfig struct {
	Name      string   `yaml:"name"`
	ToolNames []string `yaml:",inline"`
}

type Toolset struct {
	ToolsetConfig
	Tools       []*Tool `yaml:",inline"`
	toolNameSet map[string]struct{}
}

func (t Toolset) ToConfig() ToolsetConfig {
	return t.ToolsetConfig
}

// ContainsTool reports whether the toolset includes a tool with the given name.
// When built via Initialize, lookups are O(1) via toolNameSet; for Toolsets
// constructed directly (e.g., in tests), falls back to a linear scan of ToolNames.
func (t Toolset) ContainsTool(name string) bool {
	if t.toolNameSet != nil {
		_, ok := t.toolNameSet[name]
		return ok
	}
	for _, n := range t.ToolNames {
		if n == name {
			return true
		}
	}
	return false
}

type ToolsetManifest struct {
	ServerVersion string              `json:"serverVersion"`
	ToolsManifest map[string]Manifest `json:"tools"`
}

func (t ToolsetConfig) Initialize(serverVersion string, toolsMap map[string]Tool) (Toolset, error) {
	// finish toolset setup
	// Check each declared tool name exists
	toolset := Toolset{
		ToolsetConfig: t,
		Tools:         make([]*Tool, 0, len(t.ToolNames)),
		toolNameSet:   make(map[string]struct{}, len(t.ToolNames)),
	}
	if !IsValidName(toolset.Name) {
		return toolset, fmt.Errorf("invalid toolset name: %s", toolset.Name)
	}
	for _, toolName := range t.ToolNames {
		tool, ok := toolsMap[toolName]
		if !ok {
			return toolset, fmt.Errorf("tool does not exist: %s", toolName)
		}
		toolset.Tools = append(toolset.Tools, &tool)
		toolset.toolNameSet[toolName] = struct{}{}
	}
	return toolset, nil
}

// BuildManifest computes the toolset manifest at call time, using dynamic
// manifest resolution for tools that implement DynamicManifestTool.
func (t Toolset) BuildManifest(serverVersion string, sp SourceProvider) ToolsetManifest {
	m := ToolsetManifest{
		ServerVersion: serverVersion,
		ToolsManifest: make(map[string]Manifest, len(t.Tools)),
	}
	for i, tool := range t.Tools {
		if tool == nil {
			continue
		}
		toolName := t.ToolNames[i]
		if dt, ok := (*tool).(DynamicManifestTool); ok {
			m.ToolsManifest[toolName] = dt.DynamicManifest(sp)
		} else {
			m.ToolsManifest[toolName] = (*tool).Manifest()
		}
	}
	return m
}

// BuildMcpManifest computes the MCP manifest slice at call time, using dynamic
// manifest resolution for tools that implement DynamicManifestTool.
func (t Toolset) BuildMcpManifest(sp SourceProvider) []McpManifest {
	result := make([]McpManifest, 0, len(t.Tools))
	for _, tool := range t.Tools {
		if tool == nil {
			continue
		}
		if dt, ok := (*tool).(DynamicManifestTool); ok {
			result = append(result, dt.DynamicMcpManifest(sp))
		} else {
			result = append(result, (*tool).McpManifest())
		}
	}
	return result
}

var validName = regexp.MustCompile(`^[a-zA-Z0-9_-]*$`)

func IsValidName(s string) bool {
	return validName.MatchString(s)
}

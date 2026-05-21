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

package listtools

import (
	"context"
	"fmt"
	"sort"

	"github.com/googleapis/mcp-toolbox/cmd/internal"
	"github.com/googleapis/mcp-toolbox/internal/server"
	"github.com/googleapis/mcp-toolbox/internal/server/resources"
	"github.com/spf13/cobra"
)

// NewCommand creates the list-tools command.
func NewCommand(opts *internal.ToolboxOptions) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list-tools",
		Short: "List all available tools",
		Long:  `List all available tools along with their descriptions.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runListTools(cmd, args, opts)
		},
	}
	
	// Register config file flags
	internal.ConfigFileFlags(cmd.Flags(), opts)
	
	return cmd
}

func runListTools(cmd *cobra.Command, args []string, opts *internal.ToolboxOptions) error {
	ctx, cancel := context.WithCancel(cmd.Context())
	defer cancel()

	if !cmd.Flags().Changed("log-level") {
		_ = opts.Cfg.LogLevel.Set("warn")
	}

	ctx, shutdown, err := opts.Setup(ctx)
	if err != nil {
		return err
	}
	defer func() {
		_ = shutdown(ctx)
	}()

	_, err = opts.LoadConfig(ctx, &internal.ConfigParser{})
	if err != nil {
		return err
	}

	sourcesMap, authServicesMap, embeddingModelsMap, toolsMap, toolsetsMap, promptsMap, promptsetsMap, err := server.InitializeConfigs(ctx, opts.Cfg)
	if err != nil {
		return fmt.Errorf("failed to initialize resources: %w", err)
	}

	resourceMgr := resources.NewResourceManager(sourcesMap, authServicesMap, embeddingModelsMap, toolsMap, toolsetsMap, promptsMap, promptsetsMap)

	tools := resourceMgr.GetToolsMap()
	
	// Sort tool names for consistent output
	var names []string
	for name := range tools {
		names = append(names, name)
	}
	sort.Strings(names)

	fmt.Fprintln(opts.IOStreams.Out, "Available Tools:")
	for _, name := range names {
		t := tools[name]
		desc := t.Manifest().Description
		if desc == "" {
			desc = "(No description)"
		}
		fmt.Fprintf(opts.IOStreams.Out, "  %-30s %s\n", name, desc)
	}

	return nil
}

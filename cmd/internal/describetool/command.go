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

package describetool

import (
	"context"
	"fmt"

	"github.com/googleapis/mcp-toolbox/cmd/internal"
	"github.com/googleapis/mcp-toolbox/internal/server"
	"github.com/googleapis/mcp-toolbox/internal/server/resources"
	"github.com/spf13/cobra"
)

// NewCommand creates the describe-tool command.
func NewCommand(opts *internal.ToolboxOptions) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "describe-tool <tool-name>",
		Short: "Describe a tool",
		Long:  `Provide the full description of the tool including its parameter definition.`,
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runDescribeTool(cmd, args, opts)
		},
	}
	
	// Register config file flags
	internal.ConfigFileFlags(cmd.Flags(), opts)
	
	return cmd
}

func runDescribeTool(cmd *cobra.Command, args []string, opts *internal.ToolboxOptions) error {
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

	toolName := args[0]
	tool, ok := resourceMgr.GetTool(toolName)
	if !ok {
		return fmt.Errorf("tool %q not found", toolName)
	}

	fmt.Fprintf(opts.IOStreams.Out, "Tool: %s\n", toolName)
	fmt.Fprintf(opts.IOStreams.Out, "Description: %s\n", tool.Manifest().Description)
	fmt.Fprintln(opts.IOStreams.Out, "\nParameters:")

	params := tool.GetParameters()
	if len(params) == 0 {
		fmt.Fprintln(opts.IOStreams.Out, "  None")
		return nil
	}

	for _, p := range params {
		reqStr := "[Optional]"
		if p.Manifest().Required {
			reqStr = "[Required]"
		}
		fmt.Fprintf(opts.IOStreams.Out, "  --%-15s %-10s %-10s %s\n", p.GetName(), p.GetType(), reqStr, p.Manifest().Description)
	}

	return nil
}

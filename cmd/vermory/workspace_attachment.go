package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"vermory/internal/resolver"

	"github.com/spf13/cobra"
)

func newWorkspaceAttachmentCommand() *cobra.Command {
	var cwd string
	var namespace string
	var format string
	command := &cobra.Command{
		Use:   "workspace-attachment",
		Short: "Probe a local Git workspace for trusted MCP startup attachment",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if strings.TrimSpace(cwd) == "" {
				var err error
				cwd, err = os.Getwd()
				if err != nil {
					return fmt.Errorf("resolve current directory: %w", err)
				}
			}
			attachment, err := resolver.ProbeGitWorkspace(context.Background(), cwd, namespace)
			if err != nil {
				return err
			}
			switch strings.ToLower(strings.TrimSpace(format)) {
			case "json":
				return json.NewEncoder(cmd.OutOrStdout()).Encode(attachment)
			case "base64":
				encoded, err := resolver.EncodeWorkspaceAttachment(attachment)
				if err != nil {
					return err
				}
				_, err = fmt.Fprintln(cmd.OutOrStdout(), encoded)
				return err
			default:
				return fmt.Errorf("--format must be json or base64")
			}
		},
	}
	command.Flags().StringVar(&cwd, "cwd", "", "local working directory; defaults to the current directory")
	command.Flags().StringVar(&namespace, "filesystem-namespace", "", "trusted opaque filesystem namespace shared by local client adapters")
	command.Flags().StringVar(&format, "format", "json", "output format: json or base64")
	_ = command.MarkFlagRequired("filesystem-namespace")
	return command
}

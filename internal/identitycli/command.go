package identitycli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"vermory/internal/authn"
	"vermory/internal/brand"
	"vermory/internal/runtime"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/spf13/cobra"
)

type adminOptions struct {
	databaseURL string
}

type tokenIssueOutput struct {
	Token      string                `json:"token,omitempty"`
	Inspection authn.TokenInspection `json:"inspection"`
	Replayed   bool                  `json:"replayed"`
}

func NewIdentityCommand() *cobra.Command {
	options := adminOptions{}
	identity := &cobra.Command{Use: "identity", Short: "Manage server-issued identities"}
	identity.PersistentFlags().StringVar(&options.databaseURL, "database-url", "", "admin PostgreSQL connection URL")
	token := &cobra.Command{Use: "token", Short: "Manage API token lifecycle"}

	var issueOperationID, issueTenantID, issueSubjectID, issueRole, issueExpiresAt string
	issue := &cobra.Command{
		Use:   "issue",
		Short: "Issue one API token and print its secret once",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			expiresAt, err := time.Parse(time.RFC3339, issueExpiresAt)
			if err != nil {
				return errors.New("--expires-at must be an RFC3339 timestamp")
			}
			return withAdminPool(cmd.Context(), options, func(pool *pgxpool.Pool) error {
				receipt, err := authn.IssueToken(cmd.Context(), pool, authn.IssueTokenRequest{
					OperationID: issueOperationID,
					TenantID:    issueTenantID,
					SubjectID:   issueSubjectID,
					Role:        authn.Role(issueRole),
					ExpiresAt:   expiresAt,
				})
				if err != nil {
					return err
				}
				return writeJSON(cmd, tokenIssueOutput{
					Token:      receipt.Token.Reveal(),
					Inspection: receipt.Inspection,
					Replayed:   receipt.Replayed,
				})
			})
		},
	}
	issue.Flags().StringVar(&issueOperationID, "operation-id", "", "idempotency key")
	issue.Flags().StringVar(&issueTenantID, "tenant-id", "", "server tenant identifier")
	issue.Flags().StringVar(&issueSubjectID, "subject-id", "", "subject identifier")
	issue.Flags().StringVar(&issueRole, "role", "", "identity role: client, operator, or owner")
	issue.Flags().StringVar(&issueExpiresAt, "expires-at", "", "RFC3339 expiry timestamp")
	markRequired(issue, "operation-id", "tenant-id", "subject-id", "role", "expires-at")

	var inspectPublicID string
	inspect := &cobra.Command{
		Use:   "inspect",
		Short: "Inspect API token metadata without its secret",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return withAdminPool(cmd.Context(), options, func(pool *pgxpool.Pool) error {
				inspection, err := authn.InspectToken(cmd.Context(), pool, inspectPublicID)
				if err != nil {
					return err
				}
				return writeJSON(cmd, inspection)
			})
		},
	}
	inspect.Flags().StringVar(&inspectPublicID, "public-id", "", "public token identifier")
	markRequired(inspect, "public-id")

	var revokeOperationID, revokePublicID string
	revoke := &cobra.Command{
		Use:   "revoke",
		Short: "Revoke one API token",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return withAdminPool(cmd.Context(), options, func(pool *pgxpool.Pool) error {
				inspection, err := authn.RevokeToken(cmd.Context(), pool, authn.RevokeTokenRequest{
					OperationID: revokeOperationID,
					PublicID:    revokePublicID,
				})
				if err != nil {
					return err
				}
				return writeJSON(cmd, inspection)
			})
		},
	}
	revoke.Flags().StringVar(&revokeOperationID, "operation-id", "", "idempotency key")
	revoke.Flags().StringVar(&revokePublicID, "public-id", "", "public token identifier")
	markRequired(revoke, "operation-id", "public-id")

	token.AddCommand(issue, inspect, revoke)
	identity.AddCommand(token)
	return identity
}

func NewDatabaseCommand() *cobra.Command {
	options := adminOptions{}
	database := &cobra.Command{Use: "database", Short: "Manage the Vermory database boundary"}
	database.PersistentFlags().StringVar(&options.databaseURL, "database-url", "", "PostgreSQL connection URL; mutating commands require admin privileges")

	compatibility := &cobra.Command{
		Use:   "compatibility",
		Short: "Inspect binary and database schema compatibility without changing state",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			report := runtime.NewSchemaCompatibilityPreflightReport(brand.Revision)
			if strings.TrimSpace(options.databaseURL) == "" {
				if err := writeJSON(cmd, report); err != nil {
					return err
				}
				return errors.New("--database-url is required")
			}
			store, err := runtime.OpenStore(cmd.Context(), options.databaseURL)
			if err != nil {
				if writeErr := writeJSON(cmd, report); writeErr != nil {
					return writeErr
				}
				return runtime.ErrSchemaCompatibilityPreflight
			}
			defer store.Close()
			report, err = store.SchemaCompatibility(cmd.Context(), brand.Revision)
			if writeErr := writeJSON(cmd, report); writeErr != nil {
				return writeErr
			}
			if err != nil {
				return err
			}
			return report.ErrorIfIncompatible()
		},
	}

	migrate := &cobra.Command{
		Use:   "migrate",
		Short: "Apply database migrations with explicit admin credentials",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if strings.TrimSpace(options.databaseURL) == "" {
				return errors.New("--database-url is required")
			}
			store, err := runtime.OpenStore(cmd.Context(), options.databaseURL)
			if err != nil {
				return err
			}
			defer store.Close()
			if err := store.Migrate(cmd.Context()); err != nil {
				return err
			}
			return writeJSON(cmd, map[string]string{"status": "migrated"})
		},
	}

	var runtimeRole string
	grantRuntime := &cobra.Command{
		Use:   "grant-runtime",
		Short: "Grant the restricted runtime role boundary",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return withAdminPool(cmd.Context(), options, func(pool *pgxpool.Pool) error {
				if err := authn.GrantRuntimeRole(cmd.Context(), pool, runtimeRole); err != nil {
					return err
				}
				return writeJSON(cmd, map[string]string{"status": "granted", "role": runtimeRole})
			})
		},
	}
	grantRuntime.Flags().StringVar(&runtimeRole, "role", "", "restricted PostgreSQL login role")
	markRequired(grantRuntime, "role")

	rebuildProjections := &cobra.Command{
		Use:   "rebuild-projections",
		Short: "Rebuild disposable search projections from active governed memory",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if strings.TrimSpace(options.databaseURL) == "" {
				return errors.New("--database-url is required")
			}
			store, err := runtime.OpenStore(cmd.Context(), options.databaseURL)
			if err != nil {
				return err
			}
			defer store.Close()
			documents, err := store.RebuildAllProjections(cmd.Context())
			if err != nil {
				return err
			}
			return writeJSON(cmd, map[string]any{"status": "rebuilt", "documents": documents})
		},
	}

	database.AddCommand(compatibility, migrate, grantRuntime, rebuildProjections)
	return database
}

func withAdminPool(ctx context.Context, options adminOptions, run func(*pgxpool.Pool) error) error {
	if strings.TrimSpace(options.databaseURL) == "" {
		return errors.New("--database-url is required")
	}
	pool, err := pgxpool.New(ctx, options.databaseURL)
	if err != nil {
		return fmt.Errorf("open admin database: %w", err)
	}
	defer pool.Close()
	if err := pool.Ping(ctx); err != nil {
		return fmt.Errorf("connect admin database: %w", err)
	}
	return run(pool)
}

func writeJSON(cmd *cobra.Command, value any) error {
	encoder := json.NewEncoder(cmd.OutOrStdout())
	encoder.SetEscapeHTML(false)
	return encoder.Encode(value)
}

func markRequired(command *cobra.Command, names ...string) {
	for _, name := range names {
		_ = command.MarkFlagRequired(name)
	}
}

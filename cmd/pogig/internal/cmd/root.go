package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var (
	baseURL string
	apiKey  string
)

// Execute runs the pogig root command.
func Execute() error {
	return rootCmd.Execute()
}

var rootCmd = &cobra.Command{
	Use:   "pogig",
	Short: "CLI for the kilhog IPAM API",
	Long: `pogig (Breton for "chick") is the command-line client for kilhog.

It talks to the kilhog REST API through the shared Go SDK (pkg/kilhog).
Configure the target server with --base-url / KILHOG_BASE_URL and
--api-key / KILHOG_API_KEY when the server requires authentication.`,
	SilenceUsage: true,
}

func init() {
	rootCmd.PersistentFlags().StringVar(&baseURL, "base-url", "", "kilhog API base URL (default $KILHOG_BASE_URL or http://localhost:8080)")
	rootCmd.PersistentFlags().StringVar(&apiKey, "api-key", "", "API key for protected routes (default $KILHOG_API_KEY)")

	rootCmd.AddCommand(healthCmd)
	rootCmd.AddCommand(networkCmd)
	rootCmd.AddCommand(subnetCmd)
}

func exitErr(err error) error {
	fmt.Fprintln(os.Stderr, err)
	return err
}

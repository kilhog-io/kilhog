package cmd

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/kilhog-io/kilhog/pkg/kilhog"
	"github.com/spf13/cobra"
)

func newClient() (*kilhog.Client, error) {
	cfg := kilhog.ClientConfig{
		BaseURL: resolveBaseURL(),
		APIKey:  resolveAPIKey(),
	}
	return kilhog.NewClient(cfg)
}

func resolveBaseURL() string {
	if strings.TrimSpace(baseURL) != "" {
		return baseURL
	}
	if value := strings.TrimSpace(os.Getenv("KILHOG_BASE_URL")); value != "" {
		return value
	}
	return "http://localhost:8080"
}

func resolveAPIKey() string {
	if strings.TrimSpace(apiKey) != "" {
		return apiKey
	}
	return strings.TrimSpace(os.Getenv("KILHOG_API_KEY"))
}

var healthCmd = &cobra.Command{
	Use:   "health",
	Short: "Check kilhog server health",
	RunE: func(cmd *cobra.Command, _ []string) error {
		client, err := newClient()
		if err != nil {
			return exitErr(err)
		}

		status, err := client.Health(context.Background())
		if err != nil {
			return exitErr(err)
		}

		fmt.Printf("kilhog is %s\n", status.Status)
		return nil
	},
}

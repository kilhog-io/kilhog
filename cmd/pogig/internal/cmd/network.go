package cmd

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
	"github.com/kilhog-io/kilhog/pkg/kilhog"
	"github.com/spf13/cobra"
)

var networkCmd = &cobra.Command{
	Use:   "network",
	Short: "Manage kilhog networks",
}

var networkListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all networks",
	RunE: func(cmd *cobra.Command, _ []string) error {
		client, err := newClient()
		if err != nil {
			return exitErr(err)
		}

		networks, err := client.ListNetworks(context.Background())
		if err != nil {
			return exitErr(err)
		}

		return printJSON(networks)
	},
}

var networkGetCmd = &cobra.Command{
	Use:   "get <uuid>",
	Short: "Get a network by UUID",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		id, err := uuid.Parse(args[0])
		if err != nil {
			return exitErr(fmt.Errorf("invalid network uuid: %w", err))
		}

		client, err := newClient()
		if err != nil {
			return exitErr(err)
		}

		network, err := client.GetNetwork(context.Background(), id)
		if err != nil {
			return exitErr(err)
		}

		return printJSON(network)
	},
}

var (
	networkCreateName        string
	networkCreateDescription string
)

var networkCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "Create a network",
	RunE: func(cmd *cobra.Command, _ []string) error {
		client, err := newClient()
		if err != nil {
			return exitErr(err)
		}

		network, err := client.CreateNetwork(context.Background(), kilhog.CreateNetworkInput{
			Name:        networkCreateName,
			Description: networkCreateDescription,
		})
		if err != nil {
			return exitErr(err)
		}

		return printJSON(network)
	},
}

var (
	networkUpdateName        string
	networkUpdateDescription string
)

var networkUpdateCmd = &cobra.Command{
	Use:   "update <uuid>",
	Short: "Update a network",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		id, err := uuid.Parse(args[0])
		if err != nil {
			return exitErr(fmt.Errorf("invalid network uuid: %w", err))
		}

		client, err := newClient()
		if err != nil {
			return exitErr(err)
		}

		network, err := client.UpdateNetwork(context.Background(), id, kilhog.UpdateNetworkInput{
			Name:        networkUpdateName,
			Description: networkUpdateDescription,
		})
		if err != nil {
			return exitErr(err)
		}

		return printJSON(network)
	},
}

var networkDeleteCmd = &cobra.Command{
	Use:   "delete <uuid>",
	Short: "Delete a network",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		id, err := uuid.Parse(args[0])
		if err != nil {
			return exitErr(fmt.Errorf("invalid network uuid: %w", err))
		}

		client, err := newClient()
		if err != nil {
			return exitErr(err)
		}

		if err := client.DeleteNetwork(context.Background(), id); err != nil {
			return exitErr(err)
		}

		fmt.Println("network deleted")
		return nil
	},
}

func init() {
	networkCreateCmd.Flags().StringVar(&networkCreateName, "name", "", "Network name (required)")
	networkCreateCmd.Flags().StringVar(&networkCreateDescription, "description", "", "Network description")
	_ = networkCreateCmd.MarkFlagRequired("name")

	networkUpdateCmd.Flags().StringVar(&networkUpdateName, "name", "", "Network name (required)")
	networkUpdateCmd.Flags().StringVar(&networkUpdateDescription, "description", "", "Network description")
	_ = networkUpdateCmd.MarkFlagRequired("name")

	networkCmd.AddCommand(networkListCmd)
	networkCmd.AddCommand(networkGetCmd)
	networkCmd.AddCommand(networkCreateCmd)
	networkCmd.AddCommand(networkUpdateCmd)
	networkCmd.AddCommand(networkDeleteCmd)
}

func printJSON(v any) error {
	out, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return exitErr(fmt.Errorf("encode JSON failed: %w", err))
	}
	fmt.Println(string(out))
	return nil
}

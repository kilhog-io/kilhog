package cmd

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/kilhog-io/kilhog/pkg/kilhog"
	"github.com/spf13/cobra"
)

var subnetCmd = &cobra.Command{
	Use:   "subnet",
	Short: "Manage kilhog subnets",
}

var (
	subnetNetworkUUID string
)

var subnetListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all subnets in a network",
	RunE: func(cmd *cobra.Command, _ []string) error {
		networkID, err := uuid.Parse(subnetNetworkUUID)
		if err != nil {
			return exitErr(fmt.Errorf("invalid network uuid: %w", err))
		}

		client, err := newClient()
		if err != nil {
			return exitErr(err)
		}

		subnets, err := client.ListSubnets(context.Background(), networkID)
		if err != nil {
			return exitErr(err)
		}

		return printJSON(subnets)
	},
}

var subnetGetCmd = &cobra.Command{
	Use:   "get <subnet-uuid>",
	Short: "Get a subnet by UUID within a network",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		networkID, err := uuid.Parse(subnetNetworkUUID)
		if err != nil {
			return exitErr(fmt.Errorf("invalid network uuid: %w", err))
		}

		subnetID, err := uuid.Parse(args[0])
		if err != nil {
			return exitErr(fmt.Errorf("invalid subnet uuid: %w", err))
		}

		client, err := newClient()
		if err != nil {
			return exitErr(err)
		}

		subnet, err := client.GetSubnet(context.Background(), networkID, subnetID)
		if err != nil {
			return exitErr(err)
		}

		return printJSON(subnet)
	},
}

var (
	subnetCreateName        string
	subnetCreateDescription string
	subnetCreatePrefix      int
	subnetCreateAddress     string
	subnetCreateParentUUID  string
)

var subnetCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "Create a subnet in a network",
	RunE: func(cmd *cobra.Command, _ []string) error {
		networkID, err := uuid.Parse(subnetNetworkUUID)
		if err != nil {
			return exitErr(fmt.Errorf("invalid network uuid: %w", err))
		}

		client, err := newClient()
		if err != nil {
			return exitErr(err)
		}

		input := kilhog.CreateSubnetInput{
			Name:        subnetCreateName,
			Description: subnetCreateDescription,
			Prefix:      subnetCreatePrefix,
			Address:     subnetCreateAddress,
			Type:        kilhog.AddressTypeIPv4,
		}

		var subnet *kilhog.Subnet
		if subnetCreateParentUUID != "" {
			parentID, err := uuid.Parse(subnetCreateParentUUID)
			if err != nil {
				return exitErr(fmt.Errorf("invalid parent subnet uuid: %w", err))
			}
			subnet, err = client.CreateSubnetUnderParent(context.Background(), networkID, parentID, input)
		} else {
			subnet, err = client.CreateSubnetInNetwork(context.Background(), networkID, input)
		}
		if err != nil {
			return exitErr(err)
		}

		return printJSON(subnet)
	},
}

var subnetUpdateDescription string

var subnetUpdateCmd = &cobra.Command{
	Use:   "update <subnet-uuid>",
	Short: "Update a subnet description",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		networkID, err := uuid.Parse(subnetNetworkUUID)
		if err != nil {
			return exitErr(fmt.Errorf("invalid network uuid: %w", err))
		}

		subnetID, err := uuid.Parse(args[0])
		if err != nil {
			return exitErr(fmt.Errorf("invalid subnet uuid: %w", err))
		}

		client, err := newClient()
		if err != nil {
			return exitErr(err)
		}

		subnet, err := client.UpdateSubnet(context.Background(), networkID, subnetID, kilhog.UpdateSubnetInput{
			Description: subnetUpdateDescription,
		})
		if err != nil {
			return exitErr(err)
		}

		return printJSON(subnet)
	},
}

var subnetDeleteCmd = &cobra.Command{
	Use:   "delete <subnet-uuid>",
	Short: "Delete a subnet",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		networkID, err := uuid.Parse(subnetNetworkUUID)
		if err != nil {
			return exitErr(fmt.Errorf("invalid network uuid: %w", err))
		}

		subnetID, err := uuid.Parse(args[0])
		if err != nil {
			return exitErr(fmt.Errorf("invalid subnet uuid: %w", err))
		}

		client, err := newClient()
		if err != nil {
			return exitErr(err)
		}

		if err := client.DeleteSubnet(context.Background(), networkID, subnetID); err != nil {
			return exitErr(err)
		}

		fmt.Println("subnet deleted")
		return nil
	},
}

func init() {
	subnetListCmd.Flags().StringVar(&subnetNetworkUUID, "network", "", "Network UUID (required)")
	subnetGetCmd.Flags().StringVar(&subnetNetworkUUID, "network", "", "Network UUID (required)")
	subnetCreateCmd.Flags().StringVar(&subnetNetworkUUID, "network", "", "Network UUID (required)")
	subnetUpdateCmd.Flags().StringVar(&subnetNetworkUUID, "network", "", "Network UUID (required)")
	subnetDeleteCmd.Flags().StringVar(&subnetNetworkUUID, "network", "", "Network UUID (required)")

	subnetCreateCmd.Flags().StringVar(&subnetCreateName, "name", "", "Subnet name (required)")
	subnetCreateCmd.Flags().StringVar(&subnetCreateDescription, "description", "", "Subnet description")
	subnetCreateCmd.Flags().IntVar(&subnetCreatePrefix, "prefix", 0, "IPv4 prefix length (required)")
	subnetCreateCmd.Flags().StringVar(&subnetCreateAddress, "address", "", "Subnet address (required when parent is the network)")
	subnetCreateCmd.Flags().StringVar(&subnetCreateParentUUID, "parent-subnet", "", "Parent subnet UUID (omit for direct network child)")
	_ = subnetCreateCmd.MarkFlagRequired("name")
	_ = subnetCreateCmd.MarkFlagRequired("prefix")

	subnetUpdateCmd.Flags().StringVar(&subnetUpdateDescription, "description", "", "New subnet description (required)")
	_ = subnetUpdateCmd.MarkFlagRequired("description")

	for _, cmd := range []*cobra.Command{subnetListCmd, subnetGetCmd, subnetCreateCmd, subnetUpdateCmd, subnetDeleteCmd} {
		_ = cmd.MarkFlagRequired("network")
	}

	subnetCmd.AddCommand(subnetListCmd)
	subnetCmd.AddCommand(subnetGetCmd)
	subnetCmd.AddCommand(subnetCreateCmd)
	subnetCmd.AddCommand(subnetUpdateCmd)
	subnetCmd.AddCommand(subnetDeleteCmd)
}

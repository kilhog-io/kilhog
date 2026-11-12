package cmd

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/kilhog-io/kilhog/pkg/kilhog"
	"github.com/spf13/cobra"
)

var grantCmd = &cobra.Command{
	Use:   "grant",
	Short: "Manage kilhog RBAC grants",
}

var (
	grantNetworkUUID      string
	grantSubnetUUID       string
	grantPrincipalKind    string
	grantLocalUserUUID    string
	grantIdentityPoolUUID string
	grantSubject          string
	grantGroup            string
	grantMachineUUID      string
	grantMachinePoolUUID  string
	grantOwner            bool
	grantCreate           bool
	grantRead             bool
	grantUpdate           bool
	grantDelete           bool
	grantFromKind         string
	grantFromLocalUser    string
	grantFromPool         string
	grantFromSubject      string
	grantFromGroup        string
	grantFromMachine      string
	grantFromMachinePool  string
	grantToKind           string
	grantToLocalUser      string
	grantToPool           string
	grantToSubject        string
	grantToGroup          string
	grantToMachine        string
	grantToMachinePool    string
)

var grantMeCmd = &cobra.Command{
	Use:   "me",
	Short: "List grants for the current principal",
	RunE: func(cmd *cobra.Command, _ []string) error {
		client, err := newClient()
		if err != nil {
			return exitErr(err)
		}
		grants, err := client.ListMyGrants(context.Background())
		if err != nil {
			return exitErr(err)
		}
		return printJSON(grants)
	},
}

var grantPlatformCmd = &cobra.Command{
	Use:   "platform",
	Short: "Manage platform create_networks grants",
}

var grantPlatformListCmd = &cobra.Command{
	Use:   "list",
	Short: "List platform grants",
	RunE: func(cmd *cobra.Command, _ []string) error {
		client, err := newClient()
		if err != nil {
			return exitErr(err)
		}
		grants, err := client.ListPlatformGrants(context.Background())
		if err != nil {
			return exitErr(err)
		}
		return printJSON(grants)
	},
}

var grantPlatformCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "Create a platform create_networks grant",
	RunE: func(cmd *cobra.Command, _ []string) error {
		principal, err := grantPrincipalFromFlags(grantPrincipalKind, grantLocalUserUUID, grantIdentityPoolUUID, grantSubject, grantGroup, grantMachineUUID, grantMachinePoolUUID)
		if err != nil {
			return exitErr(err)
		}
		client, err := newClient()
		if err != nil {
			return exitErr(err)
		}
		grant, err := client.CreatePlatformGrant(context.Background(), kilhog.CreatePlatformGrantInput{
			Principal:  principal,
			Capability: "create_networks",
		})
		if err != nil {
			return exitErr(err)
		}
		return printJSON(grant)
	},
}

var grantPlatformDeleteCmd = &cobra.Command{
	Use:   "delete <grant-uuid>",
	Short: "Delete a platform grant",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		id, err := uuid.Parse(args[0])
		if err != nil {
			return exitErr(fmt.Errorf("invalid grant uuid: %w", err))
		}
		client, err := newClient()
		if err != nil {
			return exitErr(err)
		}
		if err := client.DeletePlatformGrant(context.Background(), id); err != nil {
			return exitErr(err)
		}
		return nil
	},
}

var grantListCmd = &cobra.Command{
	Use:   "list",
	Short: "List grants on a network or subnet",
	RunE: func(cmd *cobra.Command, _ []string) error {
		networkID, subnetID, err := parseGrantResourceFlags()
		if err != nil {
			return exitErr(err)
		}
		client, err := newClient()
		if err != nil {
			return exitErr(err)
		}
		var grants []kilhog.Grant
		if subnetID != uuid.Nil {
			grants, err = client.ListSubnetGrants(context.Background(), networkID, subnetID)
		} else {
			grants, err = client.ListNetworkGrants(context.Background(), networkID)
		}
		if err != nil {
			return exitErr(err)
		}
		return printJSON(grants)
	},
}

var grantCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "Create a grant on a network or subnet",
	RunE: func(cmd *cobra.Command, _ []string) error {
		networkID, subnetID, err := parseGrantResourceFlags()
		if err != nil {
			return exitErr(err)
		}
		principal, err := grantPrincipalFromFlags(grantPrincipalKind, grantLocalUserUUID, grantIdentityPoolUUID, grantSubject, grantGroup, grantMachineUUID, grantMachinePoolUUID)
		if err != nil {
			return exitErr(err)
		}
		client, err := newClient()
		if err != nil {
			return exitErr(err)
		}
		input := kilhog.CreateResourceGrantInput{
			Principal: principal,
			Permissions: kilhog.Permissions{
				Create: grantCreate,
				Read:   grantRead,
				Update: grantUpdate,
				Delete: grantDelete,
			},
			Owner: grantOwner,
		}
		var grant *kilhog.Grant
		if subnetID != uuid.Nil {
			grant, err = client.CreateSubnetGrant(context.Background(), networkID, subnetID, input)
		} else {
			grant, err = client.CreateNetworkGrant(context.Background(), networkID, input)
		}
		if err != nil {
			return exitErr(err)
		}
		return printJSON(grant)
	},
}

var grantUpdateCmd = &cobra.Command{
	Use:   "update <grant-uuid>",
	Short: "Replace grant flags on a network or subnet",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		grantID, err := uuid.Parse(args[0])
		if err != nil {
			return exitErr(fmt.Errorf("invalid grant uuid: %w", err))
		}
		networkID, subnetID, err := parseGrantResourceFlags()
		if err != nil {
			return exitErr(err)
		}
		client, err := newClient()
		if err != nil {
			return exitErr(err)
		}
		input := kilhog.UpdateGrantInput{
			Permissions: kilhog.Permissions{
				Create: grantCreate,
				Read:   grantRead,
				Update: grantUpdate,
				Delete: grantDelete,
			},
			Owner: grantOwner,
		}
		var grant *kilhog.Grant
		if subnetID != uuid.Nil {
			grant, err = client.UpdateSubnetGrant(context.Background(), networkID, subnetID, grantID, input)
		} else {
			grant, err = client.UpdateNetworkGrant(context.Background(), networkID, grantID, input)
		}
		if err != nil {
			return exitErr(err)
		}
		return printJSON(grant)
	},
}

var grantDeleteCmd = &cobra.Command{
	Use:   "delete <grant-uuid>",
	Short: "Delete a grant on a network or subnet",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		grantID, err := uuid.Parse(args[0])
		if err != nil {
			return exitErr(fmt.Errorf("invalid grant uuid: %w", err))
		}
		networkID, subnetID, err := parseGrantResourceFlags()
		if err != nil {
			return exitErr(err)
		}
		client, err := newClient()
		if err != nil {
			return exitErr(err)
		}
		if subnetID != uuid.Nil {
			err = client.DeleteSubnetGrant(context.Background(), networkID, subnetID, grantID)
		} else {
			err = client.DeleteNetworkGrant(context.Background(), networkID, grantID)
		}
		if err != nil {
			return exitErr(err)
		}
		return nil
	},
}

var grantTransferCmd = &cobra.Command{
	Use:   "transfer",
	Short: "Transfer ownership of a network or subnet",
	RunE: func(cmd *cobra.Command, _ []string) error {
		networkID, subnetID, err := parseGrantResourceFlags()
		if err != nil {
			return exitErr(err)
		}
		to, err := grantPrincipalFromFlags(grantToKind, grantToLocalUser, grantToPool, grantToSubject, grantToGroup, grantToMachine, grantToMachinePool)
		if err != nil {
			return exitErr(fmt.Errorf("to principal: %w", err))
		}
		input := kilhog.TransferOwnershipInput{To: to}
		if grantFromKind != "" {
			from, err := grantPrincipalFromFlags(grantFromKind, grantFromLocalUser, grantFromPool, grantFromSubject, grantFromGroup, grantFromMachine, grantFromMachinePool)
			if err != nil {
				return exitErr(fmt.Errorf("from principal: %w", err))
			}
			input.From = &from
		}
		client, err := newClient()
		if err != nil {
			return exitErr(err)
		}
		var grant *kilhog.Grant
		if subnetID != uuid.Nil {
			grant, err = client.TransferSubnetOwnership(context.Background(), networkID, subnetID, input)
		} else {
			grant, err = client.TransferNetworkOwnership(context.Background(), networkID, input)
		}
		if err != nil {
			return exitErr(err)
		}
		return printJSON(grant)
	},
}

func parseGrantResourceFlags() (uuid.UUID, uuid.UUID, error) {
	if grantNetworkUUID == "" {
		return uuid.Nil, uuid.Nil, fmt.Errorf("--network is required")
	}
	networkID, err := uuid.Parse(grantNetworkUUID)
	if err != nil {
		return uuid.Nil, uuid.Nil, fmt.Errorf("invalid network uuid: %w", err)
	}
	if grantSubnetUUID == "" {
		return networkID, uuid.Nil, nil
	}
	subnetID, err := uuid.Parse(grantSubnetUUID)
	if err != nil {
		return uuid.Nil, uuid.Nil, fmt.Errorf("invalid subnet uuid: %w", err)
	}
	return networkID, subnetID, nil
}

func grantPrincipalFromFlags(kind, localUser, pool, subject, group, machine, machinePool string) (kilhog.GrantPrincipal, error) {
	p := kilhog.GrantPrincipal{Kind: kilhog.GrantPrincipalKind(kind)}
	switch p.Kind {
	case kilhog.GrantPrincipalLocalUser:
		id, err := uuid.Parse(localUser)
		if err != nil {
			return kilhog.GrantPrincipal{}, fmt.Errorf("invalid local user uuid: %w", err)
		}
		p.LocalUserUUID = &id
	case kilhog.GrantPrincipalOIDC:
		id, err := uuid.Parse(pool)
		if err != nil {
			return kilhog.GrantPrincipal{}, fmt.Errorf("invalid identity pool uuid: %w", err)
		}
		if subject == "" {
			return kilhog.GrantPrincipal{}, fmt.Errorf("subject is required for oidc principals")
		}
		p.IdentityPoolUUID = &id
		p.Subject = subject
	case kilhog.GrantPrincipalOIDCGroup:
		id, err := uuid.Parse(pool)
		if err != nil {
			return kilhog.GrantPrincipal{}, fmt.Errorf("invalid identity pool uuid: %w", err)
		}
		if group == "" {
			return kilhog.GrantPrincipal{}, fmt.Errorf("group is required for oidc_group principals")
		}
		p.IdentityPoolUUID = &id
		p.Group = group
	case kilhog.GrantPrincipalMachine:
		id, err := uuid.Parse(machine)
		if err != nil {
			return kilhog.GrantPrincipal{}, fmt.Errorf("invalid machine uuid: %w", err)
		}
		p.MachineUUID = &id
	case kilhog.GrantPrincipalMachinePool:
		id, err := uuid.Parse(machinePool)
		if err != nil {
			return kilhog.GrantPrincipal{}, fmt.Errorf("invalid machine pool uuid: %w", err)
		}
		p.MachinePoolUUID = &id
	default:
		return kilhog.GrantPrincipal{}, fmt.Errorf("unknown principal kind %q", kind)
	}
	return p, nil
}

func init() {
	grantPlatformCreateCmd.Flags().StringVar(&grantPrincipalKind, "principal-kind", "", "Principal kind")
	grantPlatformCreateCmd.Flags().StringVar(&grantLocalUserUUID, "local-user", "", "Local user UUID")
	grantPlatformCreateCmd.Flags().StringVar(&grantIdentityPoolUUID, "identity-pool", "", "OIDC identity pool UUID")
	grantPlatformCreateCmd.Flags().StringVar(&grantSubject, "subject", "", "OIDC subject")
	grantPlatformCreateCmd.Flags().StringVar(&grantGroup, "group", "", "OIDC group name")
	grantPlatformCreateCmd.Flags().StringVar(&grantMachineUUID, "machine", "", "Machine UUID")
	grantPlatformCreateCmd.Flags().StringVar(&grantMachinePoolUUID, "machine-pool", "", "Machine pool UUID")
	_ = grantPlatformCreateCmd.MarkFlagRequired("principal-kind")

	for _, c := range []*cobra.Command{grantListCmd, grantCreateCmd, grantUpdateCmd, grantDeleteCmd, grantTransferCmd} {
		c.Flags().StringVar(&grantNetworkUUID, "network", "", "Network UUID (required)")
		c.Flags().StringVar(&grantSubnetUUID, "subnet", "", "Subnet UUID (optional)")
		_ = c.MarkFlagRequired("network")
	}

	grantCreateCmd.Flags().StringVar(&grantPrincipalKind, "principal-kind", "", "Principal kind")
	grantCreateCmd.Flags().StringVar(&grantLocalUserUUID, "local-user", "", "Local user UUID")
	grantCreateCmd.Flags().StringVar(&grantIdentityPoolUUID, "identity-pool", "", "OIDC identity pool UUID")
	grantCreateCmd.Flags().StringVar(&grantSubject, "subject", "", "OIDC subject")
	grantCreateCmd.Flags().StringVar(&grantGroup, "group", "", "OIDC group name")
	grantCreateCmd.Flags().StringVar(&grantMachineUUID, "machine", "", "Machine UUID")
	grantCreateCmd.Flags().StringVar(&grantMachinePoolUUID, "machine-pool", "", "Machine pool UUID")
	grantCreateCmd.Flags().BoolVar(&grantOwner, "owner", false, "Share ownership")
	grantCreateCmd.Flags().BoolVar(&grantCreate, "create", false, "Create permission")
	grantCreateCmd.Flags().BoolVar(&grantRead, "read", false, "Read permission")
	grantCreateCmd.Flags().BoolVar(&grantUpdate, "update", false, "Update permission")
	grantCreateCmd.Flags().BoolVar(&grantDelete, "delete", false, "Delete permission")
	_ = grantCreateCmd.MarkFlagRequired("principal-kind")

	grantUpdateCmd.Flags().BoolVar(&grantOwner, "owner", false, "Owner flag")
	grantUpdateCmd.Flags().BoolVar(&grantCreate, "create", false, "Create permission")
	grantUpdateCmd.Flags().BoolVar(&grantRead, "read", false, "Read permission")
	grantUpdateCmd.Flags().BoolVar(&grantUpdate, "update", false, "Update permission")
	grantUpdateCmd.Flags().BoolVar(&grantDelete, "delete", false, "Delete permission")

	grantTransferCmd.Flags().StringVar(&grantToKind, "to-kind", "", "Destination principal kind")
	grantTransferCmd.Flags().StringVar(&grantToLocalUser, "to-local-user", "", "Destination local user UUID")
	grantTransferCmd.Flags().StringVar(&grantToPool, "to-identity-pool", "", "Destination OIDC pool UUID")
	grantTransferCmd.Flags().StringVar(&grantToSubject, "to-subject", "", "Destination OIDC subject")
	grantTransferCmd.Flags().StringVar(&grantToGroup, "to-group", "", "Destination OIDC group")
	grantTransferCmd.Flags().StringVar(&grantToMachine, "to-machine", "", "Destination machine UUID")
	grantTransferCmd.Flags().StringVar(&grantToMachinePool, "to-machine-pool", "", "Destination machine pool UUID")
	grantTransferCmd.Flags().StringVar(&grantFromKind, "from-kind", "", "Source principal kind (optional)")
	grantTransferCmd.Flags().StringVar(&grantFromLocalUser, "from-local-user", "", "Source local user UUID")
	grantTransferCmd.Flags().StringVar(&grantFromPool, "from-identity-pool", "", "Source OIDC pool UUID")
	grantTransferCmd.Flags().StringVar(&grantFromSubject, "from-subject", "", "Source OIDC subject")
	grantTransferCmd.Flags().StringVar(&grantFromGroup, "from-group", "", "Source OIDC group")
	grantTransferCmd.Flags().StringVar(&grantFromMachine, "from-machine", "", "Source machine UUID")
	grantTransferCmd.Flags().StringVar(&grantFromMachinePool, "from-machine-pool", "", "Source machine pool UUID")
	_ = grantTransferCmd.MarkFlagRequired("to-kind")

	grantPlatformCmd.AddCommand(grantPlatformListCmd)
	grantPlatformCmd.AddCommand(grantPlatformCreateCmd)
	grantPlatformCmd.AddCommand(grantPlatformDeleteCmd)

	grantCmd.AddCommand(grantMeCmd)
	grantCmd.AddCommand(grantPlatformCmd)
	grantCmd.AddCommand(grantListCmd)
	grantCmd.AddCommand(grantCreateCmd)
	grantCmd.AddCommand(grantUpdateCmd)
	grantCmd.AddCommand(grantDeleteCmd)
	grantCmd.AddCommand(grantTransferCmd)
}
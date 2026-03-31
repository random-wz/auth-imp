package main

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

func userCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "user",
		Short: "User management",
	}

	createCmd := &cobra.Command{
		Use:   "create <username> <password> <email>",
		Short: "Create user",
		Args:  cobra.ExactArgs(3),
		RunE: func(cmd *cobra.Command, args []string) error {
			body := fmt.Sprintf(`{"username":"%s","password":"%s","email":"%s"}`, args[0], args[1], args[2])
			resp, err := restCall("POST", "/api/v1/users", strings.NewReader(body))
			if err != nil {
				return err
			}
			fmt.Println(string(resp))
			return nil
		},
	}

	getCmd := &cobra.Command{
		Use:   "get <user_id|username>",
		Short: "Get user by ID or username",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if useUDS {
				payload := map[string]string{"username": args[0]}
				resp, err := udsCall("get_user", payload)
				if err != nil {
					return err
				}
				fmt.Println(string(resp))
				return nil
			}

			resp, err := restCall("GET", "/api/v1/users/"+args[0], nil)
			if err != nil {
				return err
			}
			fmt.Println(string(resp))
			return nil
		},
	}

	listCmd := &cobra.Command{
		Use:   "list",
		Short: "List users",
		RunE: func(cmd *cobra.Command, args []string) error {
			page, _ := cmd.Flags().GetInt("page")
			pageSize, _ := cmd.Flags().GetInt("page-size")
			url := fmt.Sprintf("/api/v1/users?page=%d&page_size=%d", page, pageSize)
			resp, err := restCall("GET", url, nil)
			if err != nil {
				return err
			}
			fmt.Println(string(resp))
			return nil
		},
	}
	listCmd.Flags().Int("page", 1, "Page number")
	listCmd.Flags().Int("page-size", 10, "Page size")

	updateCmd := &cobra.Command{
		Use:   "update <user_id> <json>",
		Short: "Update user (json: {\"display_name\":\"...\"})",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			resp, err := restCall("PUT", "/api/v1/users/"+args[0], strings.NewReader(args[1]))
			if err != nil {
				return err
			}
			fmt.Println(string(resp))
			return nil
		},
	}

	deleteCmd := &cobra.Command{
		Use:   "delete <user_id>",
		Short: "Delete user",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			_, err := restCall("DELETE", "/api/v1/users/"+args[0], nil)
			if err != nil {
				return err
			}
			fmt.Println("User deleted")
			return nil
		},
	}

	onlineCountCmd := &cobra.Command{
		Use:   "online-count",
		Short: "Get online user count",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			resp, err := restCall("GET", "/api/v1/users/online/count", nil)
			if err != nil {
				return err
			}
			fmt.Println(string(resp))
			return nil
		},
	}

	cmd.AddCommand(createCmd, getCmd, listCmd, updateCmd, deleteCmd, onlineCountCmd)
	return cmd
}

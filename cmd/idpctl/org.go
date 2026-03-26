package main

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

func orgCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "org",
		Short: "Organization management",
	}

	createCmd := &cobra.Command{
		Use:   "create <name> [parent_id]",
		Short: "Create organization",
		Args:  cobra.RangeArgs(1, 2),
		RunE: func(cmd *cobra.Command, args []string) error {
			body := fmt.Sprintf(`{"name":"%s"`, args[0])
			if len(args) > 1 {
				body += fmt.Sprintf(`,"parent_id":"%s"`, args[1])
			}
			body += "}"

			if useUDS {
				var payload map[string]interface{}
				json.Unmarshal([]byte(body), &payload)
				resp, err := udsCall("create_org", payload)
				if err != nil {
					return err
				}
				fmt.Println(string(resp))
				return nil
			}

			resp, err := restCall("POST", "/api/v1/directory/organizations", strings.NewReader(body))
			if err != nil {
				return err
			}
			fmt.Println(string(resp))
			return nil
		},
	}

	getCmd := &cobra.Command{
		Use:   "get <org_id>",
		Short: "Get organization",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if useUDS {
				payload := map[string]string{"org_id": args[0]}
				resp, err := udsCall("get_org", payload)
				if err != nil {
					return err
				}
				fmt.Println(string(resp))
				return nil
			}

			resp, err := restCall("GET", "/api/v1/directory/organizations/"+args[0], nil)
			if err != nil {
				return err
			}
			fmt.Println(string(resp))
			return nil
		},
	}

	listCmd := &cobra.Command{
		Use:   "list [parent_id]",
		Short: "List organizations",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			path := "/api/v1/directory/organizations"
			if len(args) > 0 {
				path += "?parent_id=" + args[0]
			}

			if useUDS {
				payload := map[string]interface{}{}
				if len(args) > 0 {
					payload["parent_id"] = args[0]
				}
				resp, err := udsCall("list_orgs", payload)
				if err != nil {
					return err
				}
				fmt.Println(string(resp))
				return nil
			}

			resp, err := restCall("GET", path, nil)
			if err != nil {
				return err
			}
			fmt.Println(string(resp))
			return nil
		},
	}

	membersCmd := &cobra.Command{
		Use:   "members <org_id>",
		Short: "List organization members",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if useUDS {
				payload := map[string]string{"org_id": args[0]}
				resp, err := udsCall("list_org_members", payload)
				if err != nil {
					return err
				}
				fmt.Println(string(resp))
				return nil
			}

			resp, err := restCall("GET", "/api/v1/directory/organizations/"+args[0]+"/members", nil)
			if err != nil {
				return err
			}
			fmt.Println(string(resp))
			return nil
		},
	}

	addMemberCmd := &cobra.Command{
		Use:   "add-member <org_id> <user_id> [role]",
		Short: "Add member to organization",
		Args:  cobra.RangeArgs(2, 3),
		RunE: func(cmd *cobra.Command, args []string) error {
			role := "member"
			if len(args) > 2 {
				role = args[2]
			}
			body := fmt.Sprintf(`{"user_id":"%s","role":"%s"}`, args[1], role)

			if useUDS {
				var payload map[string]interface{}
				json.Unmarshal([]byte(body), &payload)
				payload["org_id"] = args[0]
				resp, err := udsCall("add_org_member", payload)
				if err != nil {
					return err
				}
				fmt.Println(string(resp))
				return nil
			}

			resp, err := restCall("POST", "/api/v1/directory/organizations/"+args[0]+"/members", strings.NewReader(body))
			if err != nil {
				return err
			}
			fmt.Println(string(resp))
			return nil
		},
	}

	removeMemberCmd := &cobra.Command{
		Use:   "remove-member <org_id> <user_id>",
		Short: "Remove member from organization",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			if useUDS {
				payload := map[string]string{"org_id": args[0], "user_id": args[1]}
				resp, err := udsCall("remove_org_member", payload)
				if err != nil {
					return err
				}
				fmt.Println(string(resp))
				return nil
			}

			_, err := restCall("DELETE", "/api/v1/directory/organizations/"+args[0]+"/members/"+args[1], nil)
			if err != nil {
				return err
			}
			fmt.Println("Member removed")
			return nil
		},
	}

	deleteCmd := &cobra.Command{
		Use:   "delete <org_id>",
		Short: "Delete organization",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			_, err := restCall("DELETE", "/api/v1/directory/organizations/"+args[0], nil)
			if err != nil {
				return err
			}
			fmt.Println("Organization deleted")
			return nil
		},
	}

	cmd.AddCommand(createCmd, getCmd, listCmd, membersCmd, addMemberCmd, removeMemberCmd, deleteCmd)
	return cmd
}

package main

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

func groupCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "group",
		Short: "Group management",
	}

	createCmd := &cobra.Command{
		Use:   "create <name> <type>",
		Short: "Create group (type: project|department|team)",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			body := fmt.Sprintf(`{"name":"%s","type":"%s"}`, args[0], args[1])

			if useUDS {
				var payload map[string]interface{}
				json.Unmarshal([]byte(body), &payload)
				resp, err := udsCall("create_group", payload)
				if err != nil {
					return err
				}
				fmt.Println(string(resp))
				return nil
			}

			resp, err := restCall("POST", "/api/v1/directory/groups", strings.NewReader(body))
			if err != nil {
				return err
			}
			fmt.Println(string(resp))
			return nil
		},
	}

	getCmd := &cobra.Command{
		Use:   "get <group_id>",
		Short: "Get group",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if useUDS {
				payload := map[string]string{"group_id": args[0]}
				resp, err := udsCall("get_group", payload)
				if err != nil {
					return err
				}
				fmt.Println(string(resp))
				return nil
			}

			resp, err := restCall("GET", "/api/v1/directory/groups/"+args[0], nil)
			if err != nil {
				return err
			}
			fmt.Println(string(resp))
			return nil
		},
	}

	listCmd := &cobra.Command{
		Use:   "list",
		Short: "List groups",
		RunE: func(cmd *cobra.Command, args []string) error {
			if useUDS {
				resp, err := udsCall("list_groups", map[string]interface{}{})
				if err != nil {
					return err
				}
				fmt.Println(string(resp))
				return nil
			}

			resp, err := restCall("GET", "/api/v1/directory/groups", nil)
			if err != nil {
				return err
			}
			fmt.Println(string(resp))
			return nil
		},
	}

	membersCmd := &cobra.Command{
		Use:   "members <group_id>",
		Short: "List group members",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if useUDS {
				payload := map[string]string{"group_id": args[0]}
				resp, err := udsCall("list_group_members", payload)
				if err != nil {
					return err
				}
				fmt.Println(string(resp))
				return nil
			}

			resp, err := restCall("GET", "/api/v1/directory/groups/"+args[0]+"/members", nil)
			if err != nil {
				return err
			}
			fmt.Println(string(resp))
			return nil
		},
	}

	addMemberCmd := &cobra.Command{
		Use:   "add-member <group_id> <user_id>",
		Short: "Add member to group",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			body := fmt.Sprintf(`{"user_id":"%s"}`, args[1])

			if useUDS {
				payload := map[string]string{"group_id": args[0], "user_id": args[1]}
				resp, err := udsCall("add_group_member", payload)
				if err != nil {
					return err
				}
				fmt.Println(string(resp))
				return nil
			}

			resp, err := restCall("POST", "/api/v1/directory/groups/"+args[0]+"/members", strings.NewReader(body))
			if err != nil {
				return err
			}
			fmt.Println(string(resp))
			return nil
		},
	}

	removeMemberCmd := &cobra.Command{
		Use:   "remove-member <group_id> <user_id>",
		Short: "Remove member from group",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			if useUDS {
				payload := map[string]string{"group_id": args[0], "user_id": args[1]}
				resp, err := udsCall("remove_group_member", payload)
				if err != nil {
					return err
				}
				fmt.Println(string(resp))
				return nil
			}

			_, err := restCall("DELETE", "/api/v1/directory/groups/"+args[0]+"/members/"+args[1], nil)
			if err != nil {
				return err
			}
			fmt.Println("Member removed")
			return nil
		},
	}

	deleteCmd := &cobra.Command{
		Use:   "delete <group_id>",
		Short: "Delete group",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			_, err := restCall("DELETE", "/api/v1/directory/groups/"+args[0], nil)
			if err != nil {
				return err
			}
			fmt.Println("Group deleted")
			return nil
		},
	}

	cmd.AddCommand(createCmd, getCmd, listCmd, membersCmd, addMemberCmd, removeMemberCmd, deleteCmd)
	return cmd
}

package main

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

func authCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "auth",
		Short: "Authentication operations",
	}

	loginCmd := &cobra.Command{
		Use:   "login <username> <password>",
		Short: "Login and get token",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			if useUDS {
				payload := map[string]string{"username": args[0], "password": args[1]}
				resp, err := udsCall("auth", payload)
				if err != nil {
					return err
				}
				fmt.Println(string(resp))
				return nil
			}

			body := fmt.Sprintf(`{"username":"%s","password":"%s"}`, args[0], args[1])
			resp, err := restCall("POST", "/api/v1/auth/login", strings.NewReader(body))
			if err != nil {
				return err
			}

			fmt.Println(string(resp))
			return nil
		},
	}

	cmd.AddCommand(loginCmd)
	return cmd
}

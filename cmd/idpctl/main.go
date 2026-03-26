package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

var (
	apiURL  string
	udsPath string
	token   string
	useUDS  bool
)

func main() {
	rootCmd := &cobra.Command{
		Use:   "idpctl",
		Short: "IDP Service CLI tool",
	}

	rootCmd.PersistentFlags().StringVar(&apiURL, "api", "http://localhost:8080", "API base URL")
	rootCmd.PersistentFlags().StringVar(&udsPath, "uds", "/tmp/idp-uds.sock", "UDS socket path")
	rootCmd.PersistentFlags().StringVar(&token, "token", "", "JWT token for REST API")
	rootCmd.PersistentFlags().BoolVar(&useUDS, "use-uds", false, "Use UDS instead of REST API")

	rootCmd.AddCommand(authCmd())
	rootCmd.AddCommand(userCmd())
	rootCmd.AddCommand(orgCmd())
	rootCmd.AddCommand(groupCmd())

	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

// REST API helper
func restCall(method, path string, body io.Reader) ([]byte, error) {
	req, _ := http.NewRequest(method, apiURL+path, body)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+strings.Trim(token, `"`))
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(data))
	}
	return data, nil
}

// UDS helper
func udsCall(action string, payload interface{}) ([]byte, error) {
	conn, err := net.Dial("unix", udsPath)
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	// Handshake
	handshake := `{"version":"1.0","format":"json"}` + "\n"
	if _, err := conn.Write([]byte(handshake)); err != nil {
		return nil, err
	}

	// Read handshake response
	buf := make([]byte, 4096)
	conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	n, err := conn.Read(buf)
	if err != nil {
		return nil, err
	}
	lines := strings.Split(string(buf[:n]), "\n")
	if len(lines) < 1 {
		return nil, fmt.Errorf("no handshake response")
	}

	// Send request
	payloadBytes, _ := json.Marshal(payload)
	req := fmt.Sprintf(`{"action":"%s","payload":%s}`, action, string(payloadBytes)) + "\n"
	if _, err := conn.Write([]byte(req)); err != nil {
		return nil, err
	}

	// Read response
	n, err = conn.Read(buf)
	if err != nil {
		return nil, err
	}
	respLines := strings.Split(string(buf[:n]), "\n")
	if len(respLines) < 1 {
		return nil, fmt.Errorf("no response")
	}
	return []byte(respLines[0]), nil
}

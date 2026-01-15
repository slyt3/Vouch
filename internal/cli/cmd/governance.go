package cmd

import (
	"fmt"
	"io"
	"net/http"
	"os"
)

func ApproveCommand() {
	if len(os.Args) < 3 {
		fmt.Println("Usage: vouch approve <event-id>")
		os.Exit(1)
	}

	eventID := os.Args[2]

	// Send HTTP request to proxy API
	url := fmt.Sprintf("http://localhost:9998/api/approve/%s", eventID)
	resp, err := http.Post(url, "application/json", nil)
	if err != nil {
		fmt.Printf("Error: Failed to connect to Vouch proxy: %v\n", err)
		fmt.Println("Make sure the proxy is running on port 9998")
		os.Exit(1)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		fmt.Printf("Error: Failed to read response body: %v\n", err)
		os.Exit(1)
	}

	if resp.StatusCode == http.StatusOK {
		fmt.Printf("✓ Event %s approved successfully\n", eventID)
	} else {
		fmt.Printf("✗ Failed to approve event: %s\n", string(body))
		os.Exit(1)
	}
}

func RejectCommand() {
	if len(os.Args) < 3 {
		fmt.Println("Usage: vouch reject <event-id>")
		os.Exit(1)
	}

	eventID := os.Args[2]

	// Send HTTP request to proxy API
	url := fmt.Sprintf("http://localhost:9998/api/reject/%s", eventID)
	resp, err := http.Post(url, "application/json", nil)
	if err != nil {
		fmt.Printf("Error: Failed to connect to Vouch proxy: %v\n", err)
		fmt.Println("Make sure the proxy is running on port 9998")
		os.Exit(1)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		fmt.Printf("Error: Failed to read response body: %v\n", err)
		os.Exit(1)
	}

	if resp.StatusCode == http.StatusOK {
		fmt.Printf("✓ Event %s rejected successfully\n", eventID)
	} else {
		fmt.Printf("✗ Failed to reject event: %s\n", string(body))
		os.Exit(1)
	}
}

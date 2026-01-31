package tests

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestIntegration(t *testing.T) {
	// 1. Build binaries
	wd, _ := os.Getwd()
	logryphPath := filepath.Join(wd, "logryph")
	cliPath := filepath.Join(wd, "logryph-cli")

	cmd := exec.Command("go", "build", "-o", logryphPath, "../main.go")
	if err := cmd.Run(); err != nil {
		t.Fatalf("Failed to build logryph: %v", err)
	}
	t.Cleanup(func() {
		if err := os.Remove(logryphPath); err != nil && !os.IsNotExist(err) {
			t.Errorf("Failed to remove logryph binary: %v", err)
		}
	})

	cmd = exec.Command("go", "build", "-o", cliPath, "../cmd/logryph-cli/main.go")
	if err := cmd.Run(); err != nil {
		t.Fatalf("Failed to build logryph-cli: %v", err)
	}
	t.Cleanup(func() {
		if err := os.Remove(cliPath); err != nil && !os.IsNotExist(err) {
			t.Errorf("Failed to remove logryph-cli binary: %v", err)
		}
	})

	// 2. Setup environment
	tmpDir, _ := os.MkdirTemp("", "logryph-integration-*")
	t.Cleanup(func() {
		if err := os.RemoveAll(tmpDir); err != nil {
			t.Errorf("Failed to remove temp dir: %v", err)
		}
	})

	// Copy schema and policy
	schemaContent, _ := os.ReadFile("../schema.sql")
	_ = os.WriteFile(filepath.Join(tmpDir, "schema.sql"), schemaContent, 0644)

	policyContent := `
version: "2026.1"
policies:
  - id: critical-infra
    match_methods: ["aws:ec2:launch"]
    risk_level: critical
`
	_ = os.WriteFile(filepath.Join(tmpDir, "logryph-policy.yaml"), []byte(policyContent), 0644)

	// 3. Start Mock MCP Server
	mockServer := &http.Server{
		Addr: ":8080",
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			var mcpReq map[string]interface{}
			_ = json.NewDecoder(r.Body).Decode(&mcpReq)

			resp := map[string]interface{}{
				"jsonrpc": "2.0",
				"id":      mcpReq["id"],
				"result":  map[string]interface{}{"success": true},
			}
			json.NewEncoder(w).SetIndent("", "  ")
			if err := json.NewEncoder(w).Encode(resp); err != nil {
				log.Printf("Mock server failed to encode response: %v", err)
			}
		}),
	}
	go func() {
		_ = mockServer.ListenAndServe()
	}()
	t.Cleanup(func() {
		if err := mockServer.Close(); err != nil {
			t.Errorf("Failed to close mock server: %v", err)
		}
	})

	// 4. Start Logryph Proxy
	logryphCmd := exec.Command(logryphPath)
	logryphCmd.Dir = tmpDir
	// Create a pipe for stderr (standard log output goes here)
	stderr, _ := logryphCmd.StderrPipe()
	stdout, _ := logryphCmd.StdoutPipe()
	if err := logryphCmd.Start(); err != nil {
		t.Fatalf("Failed to start logryph: %v", err)
	}
	defer func() { _ = logryphCmd.Process.Kill() }()

	// Wait for readiness
	ready := make(chan bool)
	monitor := func(r io.Reader, name string) {
		scanner := bufio.NewScanner(r)
		const maxLogLines = 10000
		for i := 0; i < maxLogLines; i++ {
			if !scanner.Scan() {
				break
			}
			line := scanner.Text()
			fmt.Printf("[%s] %s\n", name, line)
			if strings.Contains(line, "Admin API:") {
				select {
				case ready <- true:
				default:
				}
			}
		}
	}
	go monitor(stdout, "logryph-out")
	go monitor(stderr, "logryph-err")

	select {
	case <-ready:
		// Proxy is ready, give it a moment to fully bind
		time.Sleep(500 * time.Millisecond)
	case <-time.After(45 * time.Second): // Build and startup might take time
		t.Fatal("Timeout waiting for proxy readiness")
	}

	// 5. Test normal request (allow)
	req1 := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "mcp:list_tools",
		"params":  map[string]interface{}{},
	}
	resp1, err := sendRequest("http://localhost:9999", req1)
	if err != nil {
		t.Fatalf("Request 1 failed: %v", err)
	}
	if resp1["result"] == nil {
		t.Errorf("Expected result in response 1")
	}

	// 6. Test Forensic Capture (critical risk)
	req2 := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      2,
		"method":  "aws:ec2:launch",
		"params":  map[string]interface{}{"type": "t2.micro"},
	}

	// In passive mode, this returns IMMEDIATELY (no stall)
	resp2, err := sendRequest("http://localhost:9999", req2)
	if err != nil {
		t.Fatalf("Request 2 failed: %v", err)
	}
	if resp2["result"] == nil {
		t.Errorf("Expected result in response 2 (passive mode should not block)")
	}

	// Wait for async ledger write
	time.Sleep(1 * time.Second)

	// 7. Verify recording and tagging via CLI
	cliRiskCmd := exec.Command(cliPath, "risk")
	cliRiskCmd.Dir = tmpDir
	out, _ := cliRiskCmd.CombinedOutput()
	fmt.Printf("[CLI RISK] %s\n", string(out))

	if !strings.Contains(strings.ToLower(string(out)), "critical") {
		t.Errorf("Expected 'critical' risk tag for aws:ec2:launch in CLI output")
	}

	// 8. Verify ledger integrity
	verifyCmd := exec.Command(cliPath, "verify", "--skip-live")
	verifyCmd.Dir = tmpDir
	out, err = verifyCmd.CombinedOutput()
	if err != nil {
		t.Fatalf("Verification failed: %v\nOutput: %s", err, string(out))
	}
	if !strings.Contains(string(out), "Chain is valid") {
		t.Errorf("Verification output should contain 'Chain is valid', got:\n%s", string(out))
	}
}

func sendRequest(url string, reqBody interface{}) (map[string]interface{}, error) {
	b, _ := json.Marshal(reqBody)
	resp, err := http.Post(url, "application/json", bytes.NewBuffer(b))
	if err != nil {
		return nil, err
	}

	body, err := io.ReadAll(resp.Body)
	closeErr := resp.Body.Close()
	if err != nil {
		return nil, err
	}
	if closeErr != nil {
		return nil, closeErr
	}
	var res map[string]interface{}
	_ = json.Unmarshal(body, &res)
	return res, nil
}

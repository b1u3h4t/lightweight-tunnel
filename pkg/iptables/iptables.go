package iptables

import (
	"fmt"
	"log"
	"os/exec"
	"runtime"
	"strings"
	"sync"
)

// IPTablesManager manages iptables rules for raw socket TCP
type IPTablesManager struct {
	rules []string
	mu    sync.Mutex
}

// NewIPTablesManager creates a new iptables manager
func NewIPTablesManager() *IPTablesManager {
	return &IPTablesManager{
		rules: make([]string, 0),
	}
}

// AddRuleForPort adds an iptables rule to drop RST packets for a specific port
// This is essential for raw socket TCP to work properly
// On macOS, this is a no-op as the kernel doesn't send RST packets in the same way
func (m *IPTablesManager) AddRuleForPort(port uint16, isServer bool) error {
	if isMacOS() {
		log.Printf("macOS: skipping iptables rule for port %d (not required)", port)
		return nil
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	var rule string
	if isServer {
		// Server: drop RST packets sent by kernel in response to raw TCP packets
		// Match TCP packets destined to port 9000 with RST flag set
		// This catches RST packets kernel sends when it doesn't recognize our raw TCP sessions
		rule = fmt.Sprintf("OUTPUT -p tcp --dport %d --tcp-flags RST RST -j DROP", port)
	} else {
		// Client: drop RST packets sent by kernel for our outgoing raw TCP connections
		// Match TCP packets from our source port with RST flag set
		rule = fmt.Sprintf("OUTPUT -p tcp --sport %d --tcp-flags RST RST -j DROP", port)
	}

	// Check if rule already exists
	if m.ruleExists(rule) {
		log.Printf("iptables rule already exists: %s", rule)
		return nil
	}

	// Add the rule
	args := strings.Split(rule, " ")
	args = append([]string{"-A"}, args...)

	cmd := exec.Command("iptables", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to add iptables rule: %v, output: %s", err, output)
	}

	m.rules = append(m.rules, rule)
	log.Printf("Added iptables rule: iptables -A %s", rule)
	return nil
}

// AddRuleForConnection adds iptables rules for a specific connection (both directions)
// On macOS, this is a no-op
func (m *IPTablesManager) AddRuleForConnection(localIP string, localPort uint16, remoteIP string, remotePort uint16, isServer bool) error {
	if isMacOS() {
		log.Printf("macOS: skipping iptables rule for connection %s:%d -> %s:%d (not required)", localIP, localPort, remoteIP, remotePort)
		return nil
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	var rules []string

	if isServer {
		// Server: drop RST for this specific connection
		rules = []string{
			fmt.Sprintf("OUTPUT -p tcp --tcp-flags RST RST -s %s --sport %d -d %s --dport %d -j DROP",
				localIP, localPort, remoteIP, remotePort),
		}
	} else {
		// Client: drop RST for this specific connection
		rules = []string{
			fmt.Sprintf("OUTPUT -p tcp --tcp-flags RST RST -s %s --sport %d -d %s --dport %d -j DROP",
				localIP, localPort, remoteIP, remotePort),
		}
	}

	for _, rule := range rules {
		// Check if rule already exists
		if m.ruleExists(rule) {
			log.Printf("iptables rule already exists: %s", rule)
			continue
		}

		// Add the rule
		args := strings.Split(rule, " ")
		args = append([]string{"-A"}, args...)

		cmd := exec.Command("iptables", args...)
		output, err := cmd.CombinedOutput()
		if err != nil {
			return fmt.Errorf("failed to add iptables rule: %v, output: %s", err, output)
		}

		m.rules = append(m.rules, rule)
		log.Printf("Added iptables rule: iptables -A %s", rule)
	}

	return nil
}

// RemoveAllRules removes all iptables rules added by this manager
// On macOS, this is a no-op
func (m *IPTablesManager) RemoveAllRules() error {
	if isMacOS() {
		m.mu.Lock()
		m.rules = make([]string, 0)
		m.mu.Unlock()
		return nil
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	var errors []string

	for _, rule := range m.rules {
		args := strings.Split(rule, " ")
		args = append([]string{"-D"}, args...)

		cmd := exec.Command("iptables", args...)
		output, err := cmd.CombinedOutput()
		if err != nil {
			errors = append(errors, fmt.Sprintf("failed to remove rule '%s': %v, output: %s", rule, err, output))
			continue
		}

		log.Printf("Removed iptables rule: iptables -D %s", rule)
	}

	m.rules = make([]string, 0)

	if len(errors) > 0 {
		return fmt.Errorf("errors removing rules: %s", strings.Join(errors, "; "))
	}

	return nil
}

// ruleExists checks if an iptables rule already exists
// On macOS, always returns false
func (m *IPTablesManager) ruleExists(rule string) bool {
	if isMacOS() {
		return false
	}

	args := strings.Split(rule, " ")
	args = append([]string{"-C"}, args...)

	cmd := exec.Command("iptables", args...)
	err := cmd.Run()
	return err == nil
}

// GenerateRule generates an iptables rule string without adding it
func GenerateRule(port uint16, isServer bool) string {
	if isServer {
		return fmt.Sprintf("iptables -A OUTPUT -p tcp --tcp-flags RST RST --sport %d -j DROP", port)
	}
	return fmt.Sprintf("iptables -A OUTPUT -p tcp --tcp-flags RST RST --sport %d -j DROP", port)
}

// isMacOS checks if the current OS is macOS
func isMacOS() bool {
	return runtime.GOOS == "darwin"
}

// CheckIPTablesAvailable checks if iptables is available
// On macOS, iptables is not available but raw sockets work without it
func CheckIPTablesAvailable() error {
	// macOS doesn't have iptables, but raw sockets work without it
	// The kernel doesn't send RST packets in the same way as Linux
	if isMacOS() {
		log.Printf("Running on macOS: iptables not required for raw sockets")
		return nil
	}

	cmd := exec.Command("iptables", "--version")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("iptables not available: %v, output: %s", err, output)
	}
	return nil
}

// ClearAllRules removes all rules (static method for cleanup)
// On macOS, this is a no-op
func ClearAllRules(port uint16) error {
	if isMacOS() {
		return nil
	}

	rules := []string{
		fmt.Sprintf("OUTPUT -p tcp --tcp-flags RST RST --sport %d -j DROP", port),
		fmt.Sprintf("OUTPUT -p tcp --tcp-flags RST RST --dport %d -j DROP", port),
	}

	var errors []string
	for _, rule := range rules {
		// Try to remove the rule (ignore errors if it doesn't exist)
		args := strings.Split(rule, " ")
		args = append([]string{"-D"}, args...)

		cmd := exec.Command("iptables", args...)
		output, err := cmd.CombinedOutput()
		if err != nil {
			// Ignore "No chain/target/match by that name" errors
			if !strings.Contains(string(output), "No chain/target/match") {
				errors = append(errors, fmt.Sprintf("failed to remove rule '%s': %v", rule, err))
			}
		}
	}

	if len(errors) > 0 {
		return fmt.Errorf("errors clearing rules: %s", strings.Join(errors, "; "))
	}

	return nil
}

// MonitorAndReAdd monitors iptables and automatically re-adds rules if they are removed
func (m *IPTablesManager) MonitorAndReAdd(stopCh <-chan struct{}) {
	// This is a placeholder for future implementation
	// Can periodically check if rules still exist and re-add them if necessary
	<-stopCh
}

// GetRules returns all active rules managed by this manager
func (m *IPTablesManager) GetRules() []string {
	m.mu.Lock()
	defer m.mu.Unlock()

	rules := make([]string, len(m.rules))
	copy(rules, m.rules)
	return rules
}

// AddCustomRule adds a custom iptables rule
// On macOS, this is a no-op
func (m *IPTablesManager) AddCustomRule(rule string) error {
	if isMacOS() {
		log.Printf("macOS: skipping custom iptables rule (not required)")
		return nil
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	if m.ruleExists(rule) {
		return nil
	}

	args := strings.Split(rule, " ")
	args = append([]string{"-A"}, args...)

	cmd := exec.Command("iptables", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to add custom rule: %v, output: %s", err, output)
	}

	m.rules = append(m.rules, rule)
	return nil
}

package iptables

import "testing"

func TestDarwinPFRuleForPort(t *testing.T) {
	got := darwinPFRuleForPort(22535)
	want := "block drop out quick proto tcp from any port 22535 to any flags R/R"
	if got != want {
		t.Fatalf("darwinPFRuleForPort() = %q, want %q", got, want)
	}
}

func TestDarwinPFRuleForConnection(t *testing.T) {
	got := darwinPFRuleForConnection(22535, "49.232.146.200", 9000)
	want := "block drop out quick proto tcp from any port 22535 to 49.232.146.200 port 9000 flags R/R"
	if got != want {
		t.Fatalf("darwinPFRuleForConnection() = %q, want %q", got, want)
	}
}

func TestDarwinAnchorForConnection(t *testing.T) {
	got := darwinAnchorForConnection(22535, "49.232.146.200", 9000, false)
	want := "lightweight-tunnel/client-conn-22535-49_232_146_200-9000"
	if got != want {
		t.Fatalf("darwinAnchorForConnection() = %q, want %q", got, want)
	}
}

func TestSanitizeAnchorComponent(t *testing.T) {
	got := sanitizeAnchorComponent("2001:db8::1")
	want := "2001_db8__1"
	if got != want {
		t.Fatalf("sanitizeAnchorComponent() = %q, want %q", got, want)
	}
}

func TestDarwinPFEnabled(t *testing.T) {
	if !darwinPFEnabled("Status: Enabled for 0 days 00:01:23           Debug: Urgent") {
		t.Fatal("expected enabled status to be detected")
	}
	if darwinPFEnabled("Status: Disabled                               Debug: Urgent") {
		t.Fatal("expected disabled status to be rejected")
	}
}

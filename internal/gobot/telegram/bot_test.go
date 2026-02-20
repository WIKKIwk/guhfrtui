package telegram

import "testing"

func TestParseCommandSimple(t *testing.T) {
	cmd, args := parseCommand("/scan")
	if cmd != "/scan" {
		t.Fatalf("cmd mismatch: got %q want %q", cmd, "/scan")
	}
	if len(args) != 0 {
		t.Fatalf("args mismatch: got %v want empty", args)
	}
}

func TestParseCommandWithArgs(t *testing.T) {
	cmd, args := parseCommand("/read stop")
	if cmd != "/read" {
		t.Fatalf("cmd mismatch: got %q want %q", cmd, "/read")
	}
	if len(args) != 1 || args[0] != "stop" {
		t.Fatalf("args mismatch: got %v want [stop]", args)
	}
}

func TestParseCommandWithBotMention(t *testing.T) {
	cmd, args := parseCommand("/read@RFIDBot start")
	if cmd != "/read" {
		t.Fatalf("cmd mismatch: got %q want %q", cmd, "/read")
	}
	if len(args) != 1 || args[0] != "start" {
		t.Fatalf("args mismatch: got %v want [start]", args)
	}
}

func TestParseCommandRange20Alias(t *testing.T) {
	cmd, args := parseCommand("/range20_on")
	if cmd != "/range20_on" {
		t.Fatalf("cmd mismatch: got %q want %q", cmd, "/range20_on")
	}
	if len(args) != 0 {
		t.Fatalf("args mismatch: got %v want empty", args)
	}
}

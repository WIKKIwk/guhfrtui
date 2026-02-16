package discovery

import (
	"net/netip"
	"slices"
	"testing"
)

func TestHostsFromPrefix(t *testing.T) {
	prefix := netip.MustParsePrefix("192.168.10.0/24")
	hosts := hostsFromPrefix(prefix, 3)
	if len(hosts) != 3 {
		t.Fatalf("expected 3 hosts, got %d", len(hosts))
	}

	want := []string{"192.168.10.1", "192.168.10.2", "192.168.10.3"}
	for i := range want {
		if hosts[i].String() != want[i] {
			t.Fatalf("unexpected host %d: got %s want %s", i, hosts[i], want[i])
		}
	}
}

func TestHostsFromPrefixPointToPoint(t *testing.T) {
	prefix := netip.MustParsePrefix("10.0.0.0/31")
	hosts := hostsFromPrefix(prefix, 10)
	if len(hosts) != 0 {
		t.Fatalf("expected no hosts for /31, got %d", len(hosts))
	}
}

func TestDefaultOptionsIncludesPort2022(t *testing.T) {
	opts := DefaultOptions()
	if !slices.Contains(opts.Ports, 2022) {
		t.Fatalf("expected default ports to contain 2022, got %v", opts.Ports)
	}
}

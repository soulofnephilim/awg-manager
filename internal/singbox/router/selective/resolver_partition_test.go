package selective

import "testing"

func TestPartitionDNSServers(t *testing.T) {
	primary, public := partitionDNSServers([]string{"192.168.1.1", "1.1.1.1", "8.8.8.8", "9.9.9.9"})
	if len(primary) != 1 || primary[0] != "192.168.1.1" {
		t.Fatalf("primary = %v", primary)
	}
	if len(public) != 3 {
		t.Fatalf("public = %v", public)
	}
}

func TestPartitionDNSServers_OnlyPublic(t *testing.T) {
	primary, public := partitionDNSServers([]string{"1.1.1.1", "8.8.8.8"})
	if len(public) != 0 {
		t.Fatalf("expected no separate public bucket, got %v", public)
	}
	if len(primary) != 2 {
		t.Fatalf("primary = %v", primary)
	}
}

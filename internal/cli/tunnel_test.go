package cli

import "testing"

func TestTunnelArgs(t *testing.T) {
	got := tunnelArgs(15432, "db.internal", 5432, "jump@bastion.example.com")
	want := []string{
		"-N",
		"-o", "ExitOnForwardFailure=yes",
		"-L", "15432:db.internal:5432",
		"jump@bastion.example.com",
	}
	if len(got) != len(want) {
		t.Fatalf("args = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("args[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

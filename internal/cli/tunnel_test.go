package cli

import "testing"

func TestTunnelArgs(t *testing.T) {
	got := tunnelArgs(15432, "db.internal", 5432, "jump@bastion.example.com")
	want := []string{
		"-N",
		"-o", "ExitOnForwardFailure=yes",
		"-L", "15432:db.internal:5432",
		"--", "jump@bastion.example.com",
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

// TestTunnelArgs_OptionLikeBastion checks that a bastion value that starts with
// "-" stays a destination: ssh stops option parsing at "--".
func TestTunnelArgs_OptionLikeBastion(t *testing.T) {
	got := tunnelArgs(15432, "db.internal", 5432, "-oProxyCommand=touch /tmp/x")
	n := len(got)
	if n < 2 || got[n-2] != "--" || got[n-1] != "-oProxyCommand=touch /tmp/x" {
		t.Fatalf("bastion must follow \"--\" as the last argument, got %q", got)
	}
}

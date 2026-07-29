package natsurl

import "testing"

// The cases moved verbatim from backend/msb's TestRewriteServer (M2.1
// extraction) — the behavior msb's e2e proved is pinned here.
func TestRewriteOne(t *testing.T) {
	const alias = "host.example.internal"
	cases := []struct{ in, want string }{
		{"nats://127.0.0.1:4222", "nats://" + alias + ":4222"},
		{"nats://localhost:4222", "nats://" + alias + ":4222"},
		{"nats://[::1]:4222", "nats://" + alias + ":4222"},
		{"nats://127.0.0.1", "nats://" + alias},
		{"tls://127.0.0.1:4222", "tls://" + alias + ":4222"},
		{"nats://10.1.2.3:4222", "nats://10.1.2.3:4222"},
		{"nats://nats.example.com:4222", "nats://nats.example.com:4222"},
		{"127.0.0.1:4222", alias + ":4222"},
		{"localhost", alias},
		{"nats.example.com", "nats.example.com"},
	}
	for _, c := range cases {
		if got := RewriteOne(c.in, alias); got != c.want {
			t.Errorf("RewriteOne(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestRewrite(t *testing.T) {
	got := Rewrite([]string{"nats://127.0.0.1:4222", "nats://r.example.com:4222"}, "alias")
	want := []string{"nats://alias:4222", "nats://r.example.com:4222"}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("Rewrite[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestHasLoopback(t *testing.T) {
	if !HasLoopback([]string{"nats://r.example.com:4222", "nats://127.0.0.1:4222"}) {
		t.Error("HasLoopback missed a loopback URL")
	}
	if HasLoopback([]string{"tls://connect.ngs.global"}) {
		t.Error("HasLoopback false-positived on a routable URL")
	}
}

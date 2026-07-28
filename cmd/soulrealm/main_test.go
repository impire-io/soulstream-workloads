package main

import (
	"testing"

	"github.com/impire-io/soulrealm/backend/msb"
	"github.com/impire-io/soulrealm/backend/native"
)

// TestSelectBackend is FR-001: backend choice is node-side, native by
// default, msb opt-in, and anything unknown fails before a single op.
func TestSelectBackend(t *testing.T) {
	if be, err := selectBackend("", ""); err != nil {
		t.Fatalf("default: %v", err)
	} else if _, ok := be.(*native.Backend); !ok {
		t.Fatalf("default backend = %T, want *native.Backend", be)
	}

	if be, err := selectBackend("native", ""); err != nil {
		t.Fatalf("native: %v", err)
	} else if _, ok := be.(*native.Backend); !ok {
		t.Fatalf("native backend = %T, want *native.Backend", be)
	}

	be, err := selectBackend("msb", "debian:12")
	if err != nil {
		t.Fatalf("msb: %v", err)
	}
	mb, ok := be.(*msb.Backend)
	if !ok {
		t.Fatalf("msb backend = %T, want *msb.Backend", be)
	}
	if mb.Image != "debian:12" {
		t.Fatalf("msb image = %q, want the node-side override", mb.Image)
	}

	if _, err := selectBackend("docker", ""); err == nil {
		t.Fatal("unknown backend name did not fail")
	}
}

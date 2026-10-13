package tray

import (
	"testing"

	"deedles.dev/trayscale/internal/metadata"
	"tailscale.com/ipn"
)

func TestProfileTitle(t *testing.T) {
	withTailnet := ipn.LoginProfile{
		Name: "alice@example.com",
		NetworkProfile: ipn.NetworkProfile{
			DomainName:  "example.com",
			DisplayName: "Example",
		},
	}
	if got := ProfileTitle(withTailnet); got != "alice@example.com (Example)" {
		t.Fatalf("ProfileTitle() = %q, want %q", got, "alice@example.com (Example)")
	}

	sameName := ipn.LoginProfile{
		Name: "example.com",
		NetworkProfile: ipn.NetworkProfile{
			DomainName:  "example.com",
			DisplayName: "example.com",
		},
	}
	if got := ProfileTitle(sameName); got != "example.com" {
		t.Fatalf("ProfileTitle() = %q, want %q", got, "example.com")
	}

	folded := ipn.LoginProfile{
		Name: "Example.com",
		NetworkProfile: ipn.NetworkProfile{
			DomainName:  "example.com",
			DisplayName: "example.com",
		},
	}
	if got := ProfileTitle(folded); got != "Example.com" {
		t.Fatalf("ProfileTitle() = %q, want %q", got, "Example.com")
	}

	noDomain := ipn.LoginProfile{
		Name: "alice@example.com",
		NetworkProfile: ipn.NetworkProfile{
			DisplayName: "Example",
		},
	}
	if got := ProfileTitle(noDomain); got != "alice@example.com" {
		t.Fatalf("ProfileTitle() = %q, want %q", got, "alice@example.com")
	}
}

func TestProfileTitlePrivate(t *testing.T) {
	metadata.Private = true
	t.Cleanup(func() { metadata.Private = false })

	profile := ipn.LoginProfile{
		Name: "alice@example.com",
		NetworkProfile: ipn.NetworkProfile{
			DomainName:  "example.com",
			DisplayName: "Example",
		},
	}
	if got := ProfileTitle(profile); got != "profile@example.com" {
		t.Fatalf("ProfileTitle() = %q, want %q", got, "profile@example.com")
	}
}

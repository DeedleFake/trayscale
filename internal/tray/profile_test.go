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

func TestAccountMenu(t *testing.T) {
	hidden := planAccountMenu([]ipn.LoginProfile{{
		ID:   "one",
		Name: "alice",
	}}, "one")
	if hidden.show || len(hidden.items) != 0 {
		t.Fatalf("one profile menu = %+v, want hidden", hidden)
	}

	empty := planAccountMenu(nil, "")
	if empty.show || len(empty.items) != 0 {
		t.Fatalf("empty menu = %+v, want hidden", empty)
	}

	menu := planAccountMenu([]ipn.LoginProfile{
		{
			ID:   "a",
			Name: "al_ice",
			NetworkProfile: ipn.NetworkProfile{
				DomainName:  "tn.ts.net",
				DisplayName: "Tailnet",
			},
		},
		{ID: "b", Name: "bob"},
	}, "b")
	if !menu.show {
		t.Fatal("two profiles hid the account menu")
	}
	if len(menu.items) != 2 {
		t.Fatalf("len(items) = %d, want 2", len(menu.items))
	}
	if menu.items[0] != (accountItem{id: "a", label: "al__ice (Tailnet)", active: false}) {
		t.Fatalf("first item = %+v", menu.items[0])
	}
	if menu.items[1] != (accountItem{id: "b", label: "bob", active: true}) {
		t.Fatalf("second item = %+v", menu.items[1])
	}
}

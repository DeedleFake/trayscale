// Command tsharness runs a real tailscaled against an in-process
// testcontrol server with a few fake peers (one offering an exit
// node), so Trayscale can be verified without a real tailnet.
//
//	go run . -socket /run/tailscale/tailscaled.sock -dir /tmp/tsharness
//
// It prints READY when the local node is up and the peers are visible,
// then runs until interrupted.
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/netip"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"tailscale.com/tailcfg"
	"tailscale.com/tstest/integration"
	"tailscale.com/tstest/integration/testcontrol"
	"tailscale.com/types/key"
	"testing"
)

// Peer describes one fake node to register.
type Peer struct {
	Host     string
	ExitNode bool
	Online   bool
	Routes   []netip.Prefix
	Location *tailcfg.Location // set for Mullvad exit nodes
}

// Harness is the reusable fixture.
type Harness struct {
	Dir        string // state, logs, binaries
	Socket     string // tailscaled LocalAPI socket
	Tailscaled string // path to tailscaled binary
	Tailscale  string // path to tailscale CLI binary
	Hostname   string // this machine's hostname
	Peers      []Peer
	Churn      time.Duration // if nonzero, flip peer state this often
	Mullvad    bool          // grant this machine the mullvad capability
}

func (h *Harness) Run(ctx context.Context) error {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return err
	}
	ctrl := &testcontrol.Server{
		AllNodesSameUser: true,
		MagicDNSDomain:   "tail-test.ts.net",
		Logf:             func(string, ...any) {},
		// Local DERP+STUN so the node reaches Running with no Internet.
		DERPMap:   integration.RunDERPAndSTUN(fakeTB{}, func(string, ...any) {}, "127.0.0.1"),
		AllOnline: false,
	}
	url := "http://" + ln.Addr().String()
	ctrl.ExplicitBaseURL = url
	go http.Serve(ln, ctrl)
	log.Printf("testcontrol at %s", url)

	if err := os.MkdirAll(filepath.Dir(h.Socket), 0o755); err != nil {
		return err
	}
	logf, err := os.Create(filepath.Join(h.Dir, "tailscaled.log"))
	if err != nil {
		return err
	}
	defer logf.Close()
	d := exec.CommandContext(ctx, h.Tailscaled,
		"--tun=userspace-networking",
		"--statedir="+filepath.Join(h.Dir, "state"),
		"--socket="+h.Socket,
		"--port=0")
	d.Stdout, d.Stderr = logf, logf
	if err := d.Start(); err != nil {
		return err
	}
	defer d.Process.Kill()

	cli := func(args ...string) error {
		c := exec.CommandContext(ctx, h.Tailscale, append([]string{"--socket=" + h.Socket}, args...)...)
		out, err := c.CombinedOutput()
		if err != nil {
			return fmt.Errorf("tailscale %v: %w: %s", args, err, out)
		}
		return nil
	}
	// "tailscale up" can keep waiting after the node is Running (e.g. on
	// DERP); run it in the background and poll for the node instead.
	time.Sleep(2 * time.Second)
	go cli("up", "--login-server="+url, "--hostname="+h.Hostname)
	for i := 0; ctrl.NumNodes() == 0; i++ {
		if i > 60 {
			return fmt.Errorf("node never registered")
		}
		time.Sleep(time.Second)
	}
	time.Sleep(3 * time.Second)

	var selfUser tailcfg.UserID
	var selfKey key.NodePublic
	for _, n := range ctrl.AllNodes() {
		selfUser, selfKey = n.User, n.Key
	}
	if h.Mullvad {
		ctrl.SetNodeCapMap(selfKey, tailcfg.NodeCapMap{"mullvad": nil})
	}
	var peerKeys []key.NodePublic
	for i, p := range h.Peers {
		before := map[tailcfg.NodeID]bool{}
		for _, n := range ctrl.AllNodes() {
			before[n.ID] = true
		}
		ctrl.AddFakeNode()
		for _, n := range ctrl.AllNodes() {
			if before[n.ID] {
				continue
			}
			n.User = selfUser // same user, so peers are not sharee nodes
			n.Name = p.Host + ".tail-test.ts.net."
			n.Hostinfo = (&tailcfg.Hostinfo{Hostname: p.Host, OS: "linux", Location: p.Location}).View()
			// testcontrol picks random non-CGNAT addresses, which the CLI
			// and Trayscale do not treat as Tailscale IPs.
			addr := netip.PrefixFrom(netip.AddrFrom4([4]byte{100, 64, byte(1 + i/250), byte(2 + i%250)}), 32)
			n.Addresses = []netip.Prefix{addr}
			n.AllowedIPs = []netip.Prefix{addr}
			if p.Location != nil {
				n.Tags = []string{"tag:mullvad-exit-node"}
			}
			online := p.Online
			n.Online = &online
			if !online {
				t := time.Now().Add(-26 * time.Hour)
				n.LastSeen = &t
			}
			n.Created = time.Now().Add(-30 * 24 * time.Hour)
			if p.ExitNode {
				n.AllowedIPs = append(n.AllowedIPs, netip.MustParsePrefix("0.0.0.0/0"), netip.MustParsePrefix("::/0"))
			}
			n.AllowedIPs = append(n.AllowedIPs, p.Routes...)
			n.PrimaryRoutes = append(n.PrimaryRoutes, p.Routes...)
			ctrl.UpdateNode(n)
			peerKeys = append(peerKeys, n.Key)
		}
	}
	if u := os.Getenv("USER"); u != "" {
		// Avoid Trayscale's "not Tailscale operator" dialog.
		if err := cli("set", "--operator="+u); err != nil {
			return err
		}
	}
	if err := cli("status"); err != nil {
		return err
	}
	fmt.Println("READY", url)
	if h.Churn > 0 {
		go churn(ctx, ctrl, peerKeys, h.Churn)
	}
	<-ctx.Done()
	_ = cli("down")
	return nil
}

// churn flips one peer's online state and route set per tick, so the
// client sees a steady stream of PeersChanged updates.
func churn(ctx context.Context, ctrl *testcontrol.Server, keys []key.NodePublic, every time.Duration) {
	extra := netip.MustParsePrefix("10.99.0.0/24")
	t := time.NewTicker(every)
	defer t.Stop()
	for i := 0; ; i++ {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
		n := ctrl.Node(keys[i%len(keys)])
		if n == nil {
			continue
		}
		online := n.Online == nil || !*n.Online
		n.Online = &online
		now := time.Now()
		n.LastSeen = &now
		if i := slices.Index(n.AllowedIPs, extra); i >= 0 {
			n.AllowedIPs = slices.Delete(n.AllowedIPs, i, i+1)
			n.PrimaryRoutes = slices.DeleteFunc(n.PrimaryRoutes, func(p netip.Prefix) bool { return p == extra })
		} else if len(n.PrimaryRoutes) > 0 {
			n.AllowedIPs = append(n.AllowedIPs, extra)
			n.PrimaryRoutes = append(n.PrimaryRoutes, extra)
		}
		ctrl.UpdateNode(n)
	}
}

func main() {
	dir := flag.String("dir", "/tmp/tsharness", "state dir")
	sock := flag.String("socket", "/run/tailscale/tailscaled.sock", "LocalAPI socket")
	extra := flag.Int("extra", 0, "additional peers to register (every third offline)")
	churnEvery := flag.Duration("churn", 0, "flip one peer's state this often (0 disables)")
	mullvad := flag.Int("mullvad", 0, "Mullvad exit nodes to register (also grants the mullvad capability)")
	flag.Parse()
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()
	os.MkdirAll(*dir, 0o755)
	h := Harness{
		Dir: *dir, Socket: *sock, Hostname: "thismachine",
		Tailscaled: filepath.Join(*dir, "tailscaled"), Tailscale: filepath.Join(*dir, "tailscale"),
		Peers: []Peer{
			{Host: "alpha", Online: true},
			{Host: "bravo-exit", Online: true, ExitNode: true},
			{Host: "charlie", Online: false},
			{Host: "echo-down", Online: false, ExitNode: true},
			{Host: "delta-router", Online: true, Routes: []netip.Prefix{netip.MustParsePrefix("192.168.50.0/24")}},
		},
		Churn:   *churnEvery,
		Mullvad: *mullvad > 0,
	}
	for i := range *extra {
		h.Peers = append(h.Peers, Peer{Host: fmt.Sprintf("peer-%02d", i), Online: i%3 != 0})
	}
	cities := []tailcfg.Location{
		{Country: "Germany", CountryCode: "DE", City: "Berlin", CityCode: "ber"},
		{Country: "Germany", CountryCode: "DE", City: "Frankfurt", CityCode: "fra"},
		{Country: "Sweden", CountryCode: "SE", City: "Stockholm", CityCode: "sto"},
		{Country: "USA", CountryCode: "US", City: "New York, NY", CityCode: "nyc"},
		{Country: "USA", CountryCode: "US", City: "Dallas, TX", CityCode: "dal"},
		{Country: "Japan", CountryCode: "JP", City: "Tokyo", CityCode: "tyo"},
	}
	for i := range *mullvad {
		loc := cities[i%len(cities)]
		loc.Priority = i
		h.Peers = append(h.Peers, Peer{Host: fmt.Sprintf("%s-%s-wg-%03d", strings.ToLower(loc.CountryCode), loc.CityCode, i), Online: true, ExitNode: true, Location: &loc})
	}
	if err := h.Run(ctx); err != nil {
		log.Fatal(err)
	}
}

// fakeTB satisfies the bits of testing.TB that RunDERPAndSTUN uses.
type fakeTB struct{ testing.TB }

func (fakeTB) Helper()                   {}
func (fakeTB) Cleanup(func())            {}
func (fakeTB) Logf(string, ...any)       {}
func (fakeTB) Fatal(args ...any)         { log.Fatal(args...) }
func (fakeTB) Fatalf(f string, a ...any) { log.Fatalf(f, a...) }

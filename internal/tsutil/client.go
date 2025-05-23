package tsutil

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/netip"
	"time"

	"tailscale.com/client/local"
	"tailscale.com/client/tailscale/apitype"
	"tailscale.com/cmd/tailscale/cli"
	"tailscale.com/ipn"
	"tailscale.com/ipn/ipnstate"
	"tailscale.com/net/netcheck"
	"tailscale.com/net/netmon"
	"tailscale.com/tailcfg"
	"tailscale.com/types/logger"
	"tailscale.com/util/eventbus"
)

var (
	localClient    local.Client
	bus            = eventbus.New()
	monitor        = initMonitor()
	netcheckClient = netcheck.Client{
		NetMon: monitor,
		Logf:   logger.Discard,
	}
)

func initMonitor() *netmon.Monitor {
	monitor, err := netmon.New(bus, logger.Discard)
	if err != nil {
		slog.Error("init netmon monitor", "err", err)
	}
	return monitor
}

// GetStatus returns the status of the connection to the Tailscale
// network. If the network is not currently connected, it returns
// nil, nil.
func GetStatus(ctx context.Context) (*ipnstate.Status, error) {
	st, err := localClient.Status(ctx)
	if err != nil {
		return nil, fmt.Errorf("get tailscale status: %w", err)
	}
	return st, nil
}

// Prefs returns the options of the local node.
func Prefs(ctx context.Context) (*ipn.Prefs, error) {
	return localClient.GetPrefs(ctx)
}

// Start connects the local peer to the Tailscale network.
func Start(ctx context.Context) error {
	return cli.Run([]string{"up"})
}

// Stop disconnects the local peer from the Tailscale network.
func Stop(ctx context.Context) error {
	return cli.Run([]string{"down"})
}

// ExitNode uses the specified peer as an exit node, or unsets
// an existing exit node if peer is an empty string.
func ExitNode(ctx context.Context, peer tailcfg.StableNodeID) error {
	if peer == "" {
		var prefs ipn.Prefs
		prefs.ClearExitNode()
		_, err := localClient.EditPrefs(ctx, &ipn.MaskedPrefs{
			Prefs:         prefs,
			ExitNodeIDSet: true,
			ExitNodeIPSet: true,
		})
		if err != nil {
			return fmt.Errorf("edit prefs: %w", err)
		}
		return nil
	}

	prefs := ipn.Prefs{
		ExitNodeID: peer,
	}
	_, err := localClient.EditPrefs(ctx, &ipn.MaskedPrefs{
		Prefs:         prefs,
		ExitNodeIDSet: true,
	})
	if err != nil {
		return fmt.Errorf("edit prefs: %w", err)
	}

	return nil
}

func SetUseExitNode(ctx context.Context, use bool) error {
	err := localClient.SetUseExitNode(ctx, use)
	if err == nil {
		return nil
	}

	// TODO: If there's no prior exit node, get a suggested node and use
	// that? Unfortunately, the returned errors seem to be mostly opaque
	// strings, so that kind of complicates detecting that specific
	// situation...

	return err
}

// AdvertiseExitNode enables and disables exit node advertisement for
// the current node.
func AdvertiseExitNode(ctx context.Context, enable bool) error {
	var prefs ipn.Prefs
	prefs.SetAdvertiseExitNode(enable)

	_, err := localClient.EditPrefs(ctx, &ipn.MaskedPrefs{
		Prefs:              prefs,
		AdvertiseRoutesSet: true,
	})
	if err != nil {
		return fmt.Errorf("edit prefs: %w", err)
	}

	return nil
}

func AdvertiseRoutes(ctx context.Context, routes []netip.Prefix) error {
	prefs, err := Prefs(ctx)
	if err != nil {
		return fmt.Errorf("get prefs: %w", err)
	}
	exit := prefs.AdvertisesExitNode()
	prefs.AdvertiseRoutes = routes
	prefs.SetAdvertiseExitNode(exit)

	_, err = localClient.EditPrefs(ctx, &ipn.MaskedPrefs{
		Prefs:              *prefs,
		AdvertiseRoutesSet: true,
	})
	if err != nil {
		return fmt.Errorf("edit prefs: %w", err)
	}

	return nil
}

// AllowLANAccess enables and disables the ability for the current
// node to get access to the regular LAN that it is connected to while
// an exit node is in use.
func AllowLANAccess(ctx context.Context, allow bool) error {
	prefs := ipn.Prefs{
		ExitNodeAllowLANAccess: allow,
	}

	_, err := localClient.EditPrefs(ctx, &ipn.MaskedPrefs{
		Prefs:                     prefs,
		ExitNodeAllowLANAccessSet: true,
	})
	if err != nil {
		return fmt.Errorf("edit prefs: %w", err)
	}

	return nil
}

// AcceptRoutes sets whether or not all shared subnet routes from
// other nodes should be used by the local node.
func AcceptRoutes(ctx context.Context, accept bool) error {
	prefs := ipn.Prefs{
		RouteAll: accept,
	}

	_, err := localClient.EditPrefs(ctx, &ipn.MaskedPrefs{
		Prefs:       prefs,
		RouteAllSet: true,
	})
	if err != nil {
		return fmt.Errorf("edit prefs: %w", err)
	}

	return nil
}

// SetControlURL changes the URL of the control plane server used by
// the daemon. If controlURL is empty, the default Tailscale server is
// used.
func SetControlURL(ctx context.Context, controlURL string) error {
	prefs, err := Prefs(ctx)
	if err != nil {
		return fmt.Errorf("get prefs: %w", err)
	}
	prefs.ControlURL = controlURL

	err = localClient.Start(ctx, ipn.Options{
		UpdatePrefs: prefs,
	})
	if err != nil {
		return fmt.Errorf("start local client: %w", err)
	}

	return nil
}

func NetCheck(ctx context.Context, full bool) (*netcheck.Report, *tailcfg.DERPMap, error) {
	err := netcheckClient.Standalone(ctx, "")
	if err != nil {
		return nil, nil, fmt.Errorf("standalone: %w", err)
	}

	dm, err := localClient.CurrentDERPMap(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("current DERP map: %w", err)
	}

	if full {
		netcheckClient.MakeNextReportFull()
	}
	r, err := netcheckClient.GetReport(ctx, dm, nil)
	if err != nil {
		return nil, nil, fmt.Errorf("get netcheck report: %w", err)
	}

	return r, dm, nil
}

func PushFile(ctx context.Context, target tailcfg.StableNodeID, size int64, name string, r io.Reader) error {
	return localClient.PushFile(ctx, target, size, name, r)
}

func GetWaitingFile(ctx context.Context, name string) (io.ReadCloser, int64, error) {
	return localClient.GetWaitingFile(ctx, name)
}

func DeleteWaitingFile(ctx context.Context, name string) error {
	return localClient.DeleteWaitingFile(ctx, name)
}

// WaitingFiles polls for any pending incoming files. It returns
// quickly if there are no files currently pending.
func WaitingFiles(ctx context.Context) ([]apitype.WaitingFile, error) {
	// TODO: https://github.com/tailscale/tailscale/issues/8911
	return localClient.AwaitWaitingFiles(ctx, time.Second)
}

func FileTargets(ctx context.Context) ([]apitype.FileTarget, error) {
	return localClient.FileTargets(ctx)
}

func GetProfileStatus(ctx context.Context) (ipn.LoginProfile, []ipn.LoginProfile, error) {
	return localClient.ProfileStatus(ctx)
}

func SwitchProfile(ctx context.Context, id ipn.ProfileID) error {
	return localClient.SwitchProfile(ctx, id)
}

func StartLogin(ctx context.Context) error {
	return localClient.StartLoginInteractive(ctx)
}

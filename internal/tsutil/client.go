package tsutil

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/netip"

	"tailscale.com/client/local"
	"tailscale.com/client/tailscale/apitype"
	"tailscale.com/cmd/tailscale/cli"
	"tailscale.com/ipn"
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

// Prefs returns the options of the local node.
func Prefs(ctx context.Context) (*ipn.Prefs, error) {
	return localClient.GetPrefs(ctx)
}

// Start connects the local peer to the Tailscale network.
func Start(ctx context.Context) error {
	return cli.RunWithContext(ctx, []string{"up"})
}

// Stop disconnects the local peer from the Tailscale network.
func Stop(ctx context.Context) error {
	return cli.RunWithContext(ctx, []string{"down"})
}

// ExitNode uses the specified peer as an exit node, or unsets
// an existing exit node if peer is an empty string.
func ExitNode(ctx context.Context, peer tailcfg.StableNodeID) error {
	if peer == "" {
		var prefs ipn.Prefs
		prefs.ClearExitNode()
		_, err := localClient.EditPrefs(ctx, &ipn.MaskedPrefs{
			Prefs:           prefs,
			ExitNodeIDSet:   true,
			ExitNodeIPSet:   true,
			AutoExitNodeSet: true,
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
		Prefs:           prefs,
		ExitNodeIDSet:   true,
		ExitNodeIPSet:   true,
		AutoExitNodeSet: true,
	})
	if err != nil {
		return fmt.Errorf("edit prefs: %w", err)
	}

	return nil
}

// ExitNodeSelection is how an exit node is chosen when exit node use
// is turned on.
type ExitNodeSelection int

const (
	// SelectPreviousExitNode reuses the most recently used exit node.
	SelectPreviousExitNode ExitNodeSelection = iota

	// SelectRecommendedExitNode lets the daemon pick and keep
	// tracking the recommended exit node.
	SelectRecommendedExitNode
)

// ParseExitNodeSelection parses a setting value as written by
// [ExitNodeSelection.String]. Unknown values select the previous exit
// node.
func ParseExitNodeSelection(str string) ExitNodeSelection {
	if str == "recommended" {
		return SelectRecommendedExitNode
	}
	return SelectPreviousExitNode
}

func (sel ExitNodeSelection) String() string {
	if sel == SelectRecommendedExitNode {
		return "recommended"
	}
	return "previous"
}

// SetUseExitNode turns exit node use on or off. When turning it on,
// sel determines which exit node is used.
func SetUseExitNode(ctx context.Context, use bool, sel ExitNodeSelection) error {
	if use && sel == SelectRecommendedExitNode {
		err := useAutoExitNode(ctx)
		if err == nil {
			return nil
		}
		slog.Error("use automatic exit node", "err", err)
		return errors.Join(err, useSuggestedExitNode(ctx))
	}

	if !use {
		if err := pinAutoExitNode(ctx); err != nil {
			slog.Error("pin automatic exit node before disabling", "err", err)
		}
		return localClient.SetUseExitNode(ctx, false)
	}

	useErr := localClient.SetUseExitNode(ctx, true)
	if useErr != nil {
		suggestErr := useSuggestedExitNode(ctx)
		if suggestErr == nil {
			return nil
		}
		return errors.Join(useErr, suggestErr)
	}

	return keepPreviousExitNode(ctx)
}

// pinAutoExitNode replaces automatic exit node selection with the node
// that it currently resolves to. The daemon remembers automatic mode,
// rather than the node, as the prior exit node when exit node use is
// turned off, so without this, turning it back on with
// [SelectPreviousExitNode] can't get the node that was in use back.
func pinAutoExitNode(ctx context.Context) error {
	prefs, err := Prefs(ctx)
	if err != nil {
		return fmt.Errorf("get prefs: %w", err)
	}
	id := prefs.ExitNodeID
	if !prefs.AutoExitNode.IsSet() || id == "" {
		return nil
	}
	if _, ok := ipn.ParseAutoExitNodeString(id); ok {
		// Not resolved yet, so there's no node to remember.
		return nil
	}

	_, err = localClient.EditPrefs(ctx, &ipn.MaskedPrefs{
		Prefs:           ipn.Prefs{ExitNodeID: id},
		AutoExitNodeSet: true,
		ExitNodeIDSet:   true,
		ExitNodeIPSet:   true,
	})
	if err != nil {
		return fmt.Errorf("edit prefs: %w", err)
	}
	return nil
}

func useAutoExitNode(ctx context.Context) error {
	_, err := localClient.EditPrefs(ctx, &ipn.MaskedPrefs{
		AutoExitNode:    ipn.AnyExitNode,
		AutoExitNodeSet: true,
	})
	if err != nil {
		return fmt.Errorf("edit prefs: %w", err)
	}
	return nil
}

func useSuggestedExitNode(ctx context.Context) error {
	suggested, err := localClient.SuggestExitNode(ctx)
	if err != nil {
		return fmt.Errorf("suggest exit node: %w", err)
	}
	slog.Info("got suggested exit node", "id", suggested.ID, "name", suggested.Name, "location", suggested.Location)
	return ExitNode(ctx, suggested.ID)
}

// keepPreviousExitNode pins the exit node that the daemon just
// restored. Turning exit node use off while automatic selection was on
// makes the daemon remember that, so turning it back on would resume
// automatic selection instead of using the previous node.
func keepPreviousExitNode(ctx context.Context) error {
	prefs, err := Prefs(ctx)
	if err != nil {
		return fmt.Errorf("get prefs: %w", err)
	}
	if !prefs.AutoExitNode.IsSet() {
		return nil
	}

	id := prefs.ExitNodeID
	// The daemon uses this placeholder until it has picked a node.
	if id == "" || id == "auto:any" {
		suggested, err := localClient.SuggestExitNode(ctx)
		if err != nil {
			return fmt.Errorf("suggest exit node: %w", err)
		}
		id = suggested.ID
	}

	_, err = localClient.EditPrefs(ctx, &ipn.MaskedPrefs{
		ExitNodeID:      id,
		AutoExitNodeSet: true,
		ExitNodeIDSet:   true,
		ExitNodeIPSet:   true,
	})
	if err != nil {
		return fmt.Errorf("edit prefs: %w", err)
	}
	return nil
}

// AdvertiseExitNode enables and disables exit node advertisement for
// the current node.
func AdvertiseExitNode(ctx context.Context, enable bool) error {
	prefs, err := Prefs(ctx)
	if err != nil {
		return fmt.Errorf("get prefs: %w", err)
	}
	prefs.SetAdvertiseExitNode(enable)

	_, err = localClient.EditPrefs(ctx, &ipn.MaskedPrefs{
		Prefs:              *prefs,
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

// AcceptDNS sets whether or not the Tailscale DNS config should be
// used.
func AcceptDNS(ctx context.Context, accept bool) error {
	prefs := ipn.Prefs{
		CorpDNS: accept,
	}

	_, err := localClient.EditPrefs(ctx, &ipn.MaskedPrefs{
		Prefs:      prefs,
		CorpDNSSet: true,
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
	return localClient.AwaitWaitingFiles(ctx, 0)
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

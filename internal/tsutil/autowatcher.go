package tsutil

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/godbus/dbus/v5"
	"tailscale.com/ipn"
)

// AutoWatcher monitors Wi-Fi network changes via NetworkManager DBus
// and automatically toggles the Tailscale VPN based on whether the
// current SSID is in the trusted list.
type AutoWatcher struct {
	poller *Poller

	mu         sync.Mutex
	evalMu     sync.Mutex // serialises whole evaluations; never held with mu
	lastAction string     // "enable", "disable", or ""
	debounce   *time.Timer
	conn       *dbus.Conn
	cancel     context.CancelFunc

	// Callbacks
	OnSSIDChange func(ssid string)
	OnVPNToggle  func(enabled bool, ssid string)

	// Settings accessors (set by the UI layer)
	IsEnabled   func() bool
	TrustedList func() []string
}

// NewAutoWatcher creates a new AutoWatcher. The caller must set
// IsEnabled, TrustedList, and optionally the callback fields before
// calling Start.
func NewAutoWatcher(poller *Poller) *AutoWatcher {
	return &AutoWatcher{
		poller: poller,
	}
}

// Start begins watching for network changes. It blocks until the
// context is cancelled or Stop is called.
func (w *AutoWatcher) Start(ctx context.Context) {
	ctx, cancel := context.WithCancel(ctx)
	w.mu.Lock()
	w.cancel = cancel
	w.mu.Unlock()

	defer cancel()

	conn, err := dbus.SystemBus()
	if err != nil {
		slog.Error("auto-vpn: failed to connect to system DBus", "err", err)
		return
	}

	// SSID detection requires NetworkManager. Without it there is
	// nothing to watch, so bail out rather than sit in a loop that can
	// only ever report an unknown network -- which the fail-secure path
	// would treat as untrusted and force the VPN on.
	if !nameHasOwner(conn, "org.freedesktop.NetworkManager") {
		slog.Warn("auto-vpn: NetworkManager unavailable; auto-VPN disabled")
		return
	}
	w.mu.Lock()
	w.conn = conn
	w.mu.Unlock()

	// Subscribe to NetworkManager state changes
	err = conn.AddMatchSignal(
		dbus.WithMatchObjectPath("/org/freedesktop/NetworkManager"),
		dbus.WithMatchInterface("org.freedesktop.NetworkManager"),
		dbus.WithMatchMember("StateChanged"),
	)
	if err != nil {
		slog.Error("auto-vpn: failed to subscribe to NM StateChanged", "err", err)
	}

	// Subscribe to property changes on NetworkManager
	err = conn.AddMatchSignal(
		dbus.WithMatchObjectPath("/org/freedesktop/NetworkManager"),
		dbus.WithMatchInterface("org.freedesktop.DBus.Properties"),
		dbus.WithMatchMember("PropertiesChanged"),
	)
	if err != nil {
		slog.Error("auto-vpn: failed to subscribe to NM PropertiesChanged", "err", err)
	}

	signals := make(chan *dbus.Signal, 16)
	conn.Signal(signals)

	// Periodic re-evaluation to catch state changes (e.g. user manually
	// connects Tailscale, or Tailscale state changes after initial eval)
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	// Initial evaluation
	w.scheduleEvaluation(ctx)

	for {
		select {
		case <-ctx.Done():
			return
		case <-signals:
			w.scheduleEvaluation(ctx)
		case <-ticker.C:
			w.scheduleEvaluation(ctx)
		}
	}
}

// Stop stops the watcher.
func (w *AutoWatcher) Stop() {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.cancel != nil {
		w.cancel()
		w.cancel = nil
	}
	if w.debounce != nil {
		w.debounce.Stop()
		w.debounce = nil
	}
	w.lastAction = ""
}

// Reevaluate resets dedup state and triggers an immediate evaluation.
func (w *AutoWatcher) Reevaluate(ctx context.Context) {
	w.mu.Lock()
	w.lastAction = ""
	w.mu.Unlock()

	w.evaluateCurrent(ctx)
}

func (w *AutoWatcher) scheduleEvaluation(ctx context.Context) {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.debounce != nil {
		w.debounce.Stop()
	}
	w.debounce = time.AfterFunc(2*time.Second, func() {
		w.evaluateCurrent(ctx)
	})
}

func (w *AutoWatcher) evaluateCurrent(ctx context.Context) {
	if w.IsEnabled == nil || !w.IsEnabled() {
		return
	}

	// One evaluation at a time. Settings changes, the debounce timer and
	// the ticker can all land at once; without this each could pass the
	// dedup check while another is blocked reading the poller and issue a
	// duplicate Start/Stop.
	w.evalMu.Lock()
	defer w.evalMu.Unlock()

	// Stop may have been called while waiting for the lock.
	if ctx.Err() != nil {
		return
	}

	ssid := w.CurrentSSID()
	slog.Info("auto-vpn: evaluating network", "ssid", ssid)

	if w.OnSSIDChange != nil {
		w.OnSSIDChange(ssid)
	}

	if ssid != "" && w.isTrusted(ssid) {
		w.disableVPN(ctx, ssid)
	} else {
		w.enableVPN(ctx, ssid)
	}
}

// ipnState reads the current IPN status, giving up if the watcher is
// stopped first. The poller channel may never deliver, and a plain
// receive would leave Stop() unable to unblock the evaluation.
func (w *AutoWatcher) ipnState(ctx context.Context) (*IPNStatus, bool) {
	select {
	case status := <-w.poller.GetIPN():
		return status, true
	case <-ctx.Done():
		return nil, false
	}
}

func (w *AutoWatcher) isTrusted(ssid string) bool {
	if w.TrustedList == nil {
		return false
	}
	for _, s := range w.TrustedList() {
		if s == ssid {
			return true
		}
	}
	return false
}

func (w *AutoWatcher) enableVPN(ctx context.Context, ssid string) {
	// Check current state — if already running, just update tracking
	status, ok := w.ipnState(ctx)
	if !ok {
		return
	}
	if status.State == ipn.Running {
		w.mu.Lock()
		w.lastAction = "enable"
		w.mu.Unlock()
		return
	}

	// Dedup: only skip if we already tried to enable and VPN is still starting
	w.mu.Lock()
	if w.lastAction == "enable" {
		w.mu.Unlock()
		return
	}
	w.mu.Unlock()

	slog.Info("auto-vpn: enabling VPN", "ssid", ssid)
	err := Start(ctx)
	if ctx.Err() != nil {
		// Stopped mid-flight; do not report state the app no longer tracks.
		return
	}
	if err != nil {
		slog.Error("auto-vpn: failed to enable VPN", "err", err)
		return
	}

	w.mu.Lock()
	w.lastAction = "enable"
	w.mu.Unlock()

	if w.OnVPNToggle != nil {
		w.OnVPNToggle(true, ssid)
	}
}

func (w *AutoWatcher) disableVPN(ctx context.Context, ssid string) {
	// Check current state — if already stopped, just update tracking
	status, ok := w.ipnState(ctx)
	if !ok {
		return
	}
	if status.State != ipn.Running {
		w.mu.Lock()
		w.lastAction = "disable"
		w.mu.Unlock()
		return
	}

	// Dedup: only skip if we already tried to disable and VPN is still stopping
	w.mu.Lock()
	if w.lastAction == "disable" {
		w.mu.Unlock()
		return
	}
	w.mu.Unlock()

	slog.Info("auto-vpn: disabling VPN", "ssid", ssid)
	err := Stop(ctx)
	if ctx.Err() != nil {
		// Stopped mid-flight; do not report state the app no longer tracks.
		return
	}
	if err != nil {
		slog.Error("auto-vpn: failed to disable VPN", "err", err)
		return
	}

	w.mu.Lock()
	w.lastAction = "disable"
	w.mu.Unlock()

	if w.OnVPNToggle != nil {
		w.OnVPNToggle(false, ssid)
	}
}

// CurrentSSID returns the SSID of the currently connected Wi-Fi
// network. Returns empty string if not connected to Wi-Fi or if
// detection fails.
func (w *AutoWatcher) CurrentSSID() string {
	return w.currentSSIDDBus()
}

// DetectCurrentSSID returns the current Wi-Fi SSID without requiring
// an AutoWatcher instance. Used by the preferences UI.
func DetectCurrentSSID() string {
	conn, err := dbus.SystemBus()
	if err != nil {
		return ""
	}
	return detectSSIDViaDBus(conn)
}

func (w *AutoWatcher) currentSSIDDBus() string {
	w.mu.Lock()
	conn := w.conn
	w.mu.Unlock()

	if conn == nil {
		var err error
		conn, err = dbus.SystemBus()
		if err != nil {
			return ""
		}
	}

	return detectSSIDViaDBus(conn)
}

// NetworkManagerAvailable reports whether NetworkManager is present on
// the system bus. Auto-VPN needs it to resolve the current Wi-Fi SSID;
// there is no portable alternative, since neither gio.NetworkMonitor nor
// the NetworkMonitor portal expose SSID information. When this returns
// false the UI disables the Auto-VPN controls and explains why.
func NetworkManagerAvailable() bool {
	conn, err := dbus.SystemBus()
	if err != nil {
		slog.Debug("auto-vpn: no system bus", "err", err)
		return false
	}
	return nameHasOwner(conn, "org.freedesktop.NetworkManager")
}

// nameHasOwner asks the bus daemon whether name is currently owned,
// which avoids activating the service just to probe for it.
func nameHasOwner(conn *dbus.Conn, name string) bool {
	bus := conn.Object("org.freedesktop.DBus", "/org/freedesktop/DBus")

	var owned bool
	err := bus.Call("org.freedesktop.DBus.NameHasOwner", 0, name).Store(&owned)
	if err != nil {
		slog.Debug("auto-vpn: NameHasOwner failed", "name", name, "err", err)
		return false
	}
	return owned
}

func detectSSIDViaDBus(conn *dbus.Conn) string {
	nm := conn.Object("org.freedesktop.NetworkManager", "/org/freedesktop/NetworkManager")

	// Get list of devices
	var devicePaths []dbus.ObjectPath
	err := nm.Call("org.freedesktop.NetworkManager.GetDevices", 0).Store(&devicePaths)
	if err != nil {
		slog.Debug("auto-vpn: failed to get NM devices", "err", err)
		return ""
	}

	for _, devPath := range devicePaths {
		dev := conn.Object("org.freedesktop.NetworkManager", devPath)

		// Check device type (2 = Wi-Fi)
		deviceType, err := dev.GetProperty("org.freedesktop.NetworkManager.Device.DeviceType")
		if err != nil {
			continue
		}
		if deviceType.Value().(uint32) != 2 {
			continue
		}

		// Get active access point
		apPathVariant, err := dev.GetProperty("org.freedesktop.NetworkManager.Device.Wireless.ActiveAccessPoint")
		if err != nil {
			continue
		}
		apPath, ok := apPathVariant.Value().(dbus.ObjectPath)
		if !ok || apPath == "/" || apPath == "" {
			continue
		}

		// Get SSID from access point
		ap := conn.Object("org.freedesktop.NetworkManager", apPath)
		ssidVariant, err := ap.GetProperty("org.freedesktop.NetworkManager.AccessPoint.Ssid")
		if err != nil {
			continue
		}
		ssidBytes, ok := ssidVariant.Value().([]byte)
		if !ok || len(ssidBytes) == 0 {
			continue
		}

		return string(ssidBytes)
	}

	return ""
}

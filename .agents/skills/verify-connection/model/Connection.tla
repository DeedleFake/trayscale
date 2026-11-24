------------------------------ MODULE Connection ------------------------------
\* Connection and login lifecycle: Bus reconnect and SetRunning hold,
\* App.online, NeedsAuth, and App.login, against tailscaled at the
\* tailscale.com version pinned in scripts/check-connection.
\*
\* Trayscale paths: bus = internal/tsutil/bus.go, app = internal/ui/app.go,
\* win = internal/ui/mainwindow.go, off = internal/ui/offlinepage.go,
\* tray = internal/tray/tray.go, poll = internal/tsutil/poller.go,
\* tc = internal/tsutil/client.go, files = internal/tsutil/files.go,
\* gutil = internal/gutil/data.go.
\* Tailscale paths, in the module cache: ll = ipn/ipnlocal/local.go,
\* lapi = client/local/local.go.
\*
\* Each LocalAPI or CLI call is one step, because tailscaled serves it
\* under b.mu. Each gutil.IdleAdd callback is one step, in the order
\* they were added, so the model keeps them in one queue, mainQ. The
\* constants other than the bounds are code facts that the check script
\* reads from the code. TRUE means the code has the guard.
EXTENDS Naturals, Sequences

CONSTANTS
  MaxToggles, MaxLogins, MaxDrops, MaxEngine, MaxFiles,
  OfflineClearsEngine,     \* app: showIPN sets a.engine = nil when !online
  OfflineClearsWaiting,    \* app: showIPN sets a.waiting = nil when !online
  OfflineIgnoresEngine,    \* app: EngineStatus case returns if !a.online
  WindowClearsEngine,      \* win: Update IPNStatus sets win.engine = nil when !online
  NeedsAuthPrompts,        \* app: setRunning offers login instead of SetRunning when NeedsAuth
  LoginWaitsAfterStart,    \* app: login calls StartLogin, then a.bus.Next
  SetRunningHolds,         \* bus: SetRunning increments holds before run and decrements after
  SetRunningResendsOnFail, \* bus: SetRunning queues resend when run returns an error
  WatchSendsNoState,       \* bus: watch publishes a synthetic NoState after the connection drops
  FilesGatedOnline,        \* files: Files only pushes when Bus.latest is online
  AtomicUI                 \* chart only: apply UI updates in the same step as publish

STATES == {"NoState", "NeedsLogin", "Stopped", "Running"}

VARIABLES
  daemonUp,     \* tailscaled is reachable
  dState,       \* tailscaled's IPN state, when daemonUp
  dBrowse,      \* tailscaled has a valid auth URL (ll:4708-4714)
  busConn,      \* watchOnce has a live bus connection (bus:165)
  pending,      \* status waiting to be published, or [ok |-> FALSE]
  holds,        \* SetRunning hold count (bus:86, bus:109-110)
  resend,       \* resend request queued (bus:52, bus:307-310)
  latest,       \* last status passed to New (bus:34)
  mainQ,        \* IdleAdd queue (app:823, gutil:62)
  online,       \* App.online (app:42, app:258-260)
  engine,       \* App.engine is set (app:54)
  winEngine,    \* MainWindow.engine is set (win:71, win:306)
  waiting,      \* App.waiting is set (app:59)
  appState,     \* last IPN state shown by showIPN
  switchOn,     \* the window's status switch (win:348-352)
  needsAuthUI,  \* OfflinePage.NeedsAuthGroup (off:39)
  prompt,       \* the "Login Required" confirmation is showing (app:383)
  runPc,        \* SetRunning: idle, held, releasing
  runUp,        \* the SetRunning direction
  runFail,      \* this SetRunning call returns an error
  loginPc,      \* idle, started, waiting, opened, failed
  nextReady,    \* a status was published after Next subscribed (bus:259-272)
  awaitingURL,  \* StartLogin asked control for an auth URL (ll:4728)
  sawURL,       \* this login attempt's URL was published
  opened,       \* this login attempt opened the URL
  lastUpAuth,   \* the last "go up" attempt saw NeedsLogin
  lastUpRan,    \* that attempt called SetRunning
  lastUpPrompt, \* that attempt offered the login confirmation
  lastUpDone,   \* a "go up" attempt has finished
  downNoted,    \* watch has sent the synthetic NoState for this drop
  budget

vars == <<
  daemonUp, dState, dBrowse, busConn, pending, holds, resend, latest,
  mainQ, online, engine, winEngine, waiting, appState, switchOn,
  needsAuthUI, prompt, runPc, runUp, runFail, loginPc, nextReady,
  awaitingURL, sawURL, opened, lastUpAuth, lastUpRan, lastUpPrompt,
  lastUpDone, downNoted, budget
>>

None == [ok |-> FALSE, state |-> "NoState", browse |-> FALSE]
St(state, browse) == [ok |-> TRUE, state |-> state, browse |-> browse]

OnlineOf(state) == state = "Running"
NeedsAuthOf(state) == state = "NeedsLogin"   \* poll:193-194

Init ==
  /\ daemonUp = TRUE
  /\ \/ /\ dState = "NeedsLogin"
        /\ dBrowse \in BOOLEAN
     \/ /\ dState \in {"Stopped", "Running"}
        /\ dBrowse = FALSE
  /\ busConn = TRUE
  /\ pending = None
  /\ holds = 0
  /\ resend = FALSE
  /\ latest = St(dState, dBrowse)
  /\ mainQ = <<>>
  /\ online = OnlineOf(dState)
  /\ engine = FALSE
  /\ winEngine = FALSE
  /\ waiting = FALSE
  /\ appState = dState
  /\ switchOn = OnlineOf(dState)
  /\ needsAuthUI = NeedsAuthOf(dState)
  /\ prompt = FALSE
  /\ runPc = "idle"
  /\ runUp = FALSE
  /\ runFail = FALSE
  /\ loginPc = "idle"
  /\ nextReady = FALSE
  /\ awaitingURL = FALSE
  /\ sawURL = FALSE
  /\ opened = FALSE
  /\ lastUpAuth = FALSE
  /\ lastUpRan = FALSE
  /\ lastUpPrompt = FALSE
  /\ lastUpDone = FALSE
  /\ downNoted = TRUE
  /\ budget = [toggles |-> 0, logins |-> 0, drops |-> 0,
               engine |-> 0, files |-> 0, fails |-> 0]

\* ---------------- App / window (main thread) ----------------

\* app:256-288, win:301-325, off:37-42, win:348-352.
ShowIPN(s) ==
  /\ online' = OnlineOf(s.state)
  /\ appState' = s.state
  /\ switchOn' = OnlineOf(s.state)
  /\ needsAuthUI' = NeedsAuthOf(s.state)
  /\ IF OnlineOf(s.state)
     THEN UNCHANGED <<engine, winEngine, waiting>>
     ELSE /\ engine' = IF OfflineClearsEngine THEN FALSE ELSE engine
          /\ waiting' = IF OfflineClearsWaiting THEN FALSE ELSE waiting
          /\ winEngine' = IF WindowClearsEngine THEN FALSE ELSE winEngine

ApplyEngine ==
  IF OfflineIgnoresEngine /\ ~online
  THEN UNCHANGED <<engine, winEngine>>
  ELSE /\ engine' = TRUE
       /\ winEngine' = TRUE

\* ---------------- Bus ----------------

\* bus:119-126. New does IdleAdd(a.update) (app:823-825). fire(changePublished)
\* happens after New, so Next unblocks on this publish (bus:259-272).
BusPublishes ==
  /\ pending.ok
  /\ holds = 0
  /\ latest' = pending
  /\ pending' = None
  /\ sawURL' = (sawURL \/ (pending.browse /\ loginPc \in {"started", "waiting"}))
  /\ nextReady' = IF loginPc = "waiting" THEN TRUE ELSE nextReady
  /\ IF AtomicUI
     THEN /\ ShowIPN(pending)
          /\ UNCHANGED mainQ
     ELSE /\ mainQ' = Append(mainQ, [kind |-> "ipn", state |-> pending.state, browse |-> pending.browse])
          /\ UNCHANGED <<online, engine, winEngine, waiting, appState, switchOn, needsAuthUI>>
  /\ UNCHANGED <<daemonUp, dState, dBrowse, busConn, holds, resend, prompt,
                 runPc, runUp, runFail, loginPc, awaitingURL, opened,
                 lastUpAuth, lastUpRan, lastUpPrompt, lastUpDone, downNoted, budget>>

\* bus:111-114. Resend calls New(latest) again, and does not fire changePublished.
BusResends ==
  /\ resend
  /\ ~pending.ok
  /\ holds = 0
  /\ latest.ok
  /\ resend' = FALSE
  /\ IF AtomicUI
     THEN /\ ShowIPN(latest)
          /\ UNCHANGED mainQ
     ELSE /\ mainQ' = Append(mainQ, [kind |-> "ipn", state |-> latest.state, browse |-> latest.browse])
          /\ UNCHANGED <<online, engine, winEngine, waiting, appState, switchOn, needsAuthUI>>
  /\ UNCHANGED <<daemonUp, dState, dBrowse, busConn, pending, holds, latest,
                 prompt, runPc, runUp, runFail, loginPc, nextReady,
                 awaitingURL, sawURL, opened, lastUpAuth, lastUpRan,
                 lastUpPrompt, lastUpDone, downNoted, budget>>

\* app:124-192.
AppUpdates ==
  /\ ~AtomicUI
  /\ mainQ # <<>>
  /\ LET ev == Head(mainQ) IN
       /\ mainQ' = Tail(mainQ)
       /\ \/ /\ ev.kind = "ipn"
             /\ ShowIPN(ev)
             /\ UNCHANGED <<daemonUp, dState, dBrowse, busConn, pending, holds,
                            resend, latest, prompt, runPc, runUp, runFail, loginPc,
                            nextReady, awaitingURL, sawURL, opened, lastUpAuth,
                            lastUpRan, lastUpPrompt, lastUpDone, downNoted, budget>>
          \/ /\ ev.kind = "engine"
             /\ ApplyEngine
             /\ UNCHANGED <<daemonUp, dState, dBrowse, busConn, pending, holds,
                            resend, latest, online, waiting, appState, switchOn,
                            needsAuthUI, prompt, runPc, runUp, runFail, loginPc,
                            nextReady, awaitingURL, sawURL, opened, lastUpAuth,
                            lastUpRan, lastUpPrompt, lastUpDone, downNoted, budget>>
          \/ /\ ev.kind = "files"
             /\ waiting' = TRUE
             /\ UNCHANGED <<daemonUp, dState, dBrowse, busConn, pending, holds,
                            resend, latest, online, engine, winEngine, appState,
                            switchOn, needsAuthUI, prompt, runPc, runUp, runFail,
                            loginPc, nextReady, awaitingURL, sawURL, opened,
                            lastUpAuth, lastUpRan, lastUpPrompt, lastUpDone,
                            downNoted, budget>>

\* ---------------- SetRunning and the status switch ----------------

StartHold(up, fail) ==
  /\ runPc' = "held"
  /\ runUp' = up
  /\ runFail' = fail
  /\ holds' = IF SetRunningHolds THEN 1 ELSE 0   \* bus:290-296
  /\ lastUpAuth' = (up /\ NeedsAuthOf(latest.state))
  /\ lastUpRan' = TRUE
  /\ lastUpPrompt' = FALSE
  /\ lastUpDone' = FALSE

\* app:376-405, win:730-753, tray OnConnToggle -> app:419-436.
\* The switch moves first (win:115-120). setRunning reads Latest (app:378).
UserToggles ==
  /\ runPc = "idle"
  /\ loginPc \in {"idle", "opened", "failed"}
  /\ ~prompt
  /\ latest.ok
  /\ budget.toggles < MaxToggles
  /\ LET up == ~OnlineOf(latest.state) IN
       \/ /\ up /\ NeedsAuthOf(latest.state) /\ NeedsAuthPrompts
          /\ prompt' = TRUE            \* app:383-394
          /\ switchOn' = FALSE         \* !sent reverts the switch (win:747-748)
          /\ lastUpAuth' = TRUE
          /\ lastUpRan' = FALSE
          /\ lastUpPrompt' = TRUE
          /\ lastUpDone' = TRUE
          /\ budget' = [budget EXCEPT !.toggles = @ + 1]
          /\ UNCHANGED <<daemonUp, dState, dBrowse, busConn, pending, holds,
                         resend, latest, mainQ, online, engine, winEngine,
                         waiting, appState, needsAuthUI, runPc, runUp, runFail,
                         loginPc, nextReady, awaitingURL, sawURL, opened, downNoted>>
       \/ /\ ~(up /\ NeedsAuthOf(latest.state) /\ NeedsAuthPrompts)
          /\ switchOn' = up
          /\ \/ /\ daemonUp
                /\ StartHold(up, FALSE)
                /\ budget' = [budget EXCEPT !.toggles = @ + 1]
             \/ /\ budget.fails < 1
                /\ StartHold(up, TRUE)
                /\ budget' = [budget EXCEPT !.toggles = @ + 1, !.fails = @ + 1]
          /\ UNCHANGED <<daemonUp, dState, dBrowse, busConn, pending, resend,
                         latest, mainQ, online, engine, winEngine, waiting,
                         appState, needsAuthUI, prompt, loginPc, nextReady,
                         awaitingURL, sawURL, opened, downNoted>>

\* The CLI call, bus:315-326 / tc:48-55, then the unbuffered holds <- -1
\* (bus:301-304), which Run applies before SetRunning queues a resend.
SetRunningFinishes ==
  /\ runPc = "held"
  /\ IF ~daemonUp \/ runFail
     THEN UNCHANGED <<dState, dBrowse, pending>>
     ELSE /\ dState' = IF runUp THEN "Running" ELSE "Stopped"
          /\ dBrowse' = IF runUp THEN FALSE ELSE dBrowse
          /\ pending' = St(dState', dBrowse')   \* last notify while held (bus:102)
          \* Stopped and Running are the states Start/Stop settle on. Starting
          \* notifies during a successful up are overwritten in pending.
  /\ holds' = 0
  /\ runPc' = "releasing"
  /\ UNCHANGED <<daemonUp, busConn, resend, latest, mainQ, online, engine,
                 winEngine, waiting, appState, switchOn, needsAuthUI, prompt,
                 runUp, runFail, loginPc, nextReady, awaitingURL, sawURL, opened,
                 lastUpAuth, lastUpRan, lastUpPrompt, lastUpDone, downNoted, budget>>

\* bus:306-310, after the hold has been released.
SetRunningQueuesResend ==
  /\ runPc = "releasing"
  /\ runPc' = "idle"
  /\ lastUpDone' = TRUE
  /\ resend' = (resend \/ (runFail /\ SetRunningResendsOnFail))
  /\ UNCHANGED <<daemonUp, dState, dBrowse, busConn, pending, holds, latest,
                 mainQ, online, engine, winEngine, waiting, appState, switchOn,
                 needsAuthUI, prompt, runUp, runFail, loginPc, nextReady,
                 awaitingURL, sawURL, opened, lastUpAuth, lastUpRan,
                 lastUpPrompt, downNoted, budget>>

\* ---------------- Login ----------------

\* StartLoginInteractive (ll:4699-4730, tc:440-441, lapi:1071). When a valid
\* auth URL already exists, popBrowserAuthNowLocked sends BrowseToURL under
\* b.mu before the RPC returns (ll:4725-4726, ll:4355-4368).
PopOrRequestURL ==
  IF dBrowse
  THEN /\ awaitingURL' = FALSE
       /\ pending' = St(dState, TRUE)
  ELSE /\ awaitingURL' = TRUE
       /\ UNCHANGED pending

BeginLogin ==
  /\ loginPc \in {"idle", "opened", "failed"}
  /\ runPc = "idle"
  /\ daemonUp
  /\ busConn
  /\ latest.ok
  /\ budget.logins < MaxLogins
  /\ budget' = [budget EXCEPT !.logins = @ + 1]
  /\ sawURL' = FALSE
  /\ opened' = FALSE
  /\ nextReady' = FALSE
  /\ PopOrRequestURL
  /\ loginPc' = IF LoginWaitsAfterStart THEN "started" ELSE "waiting"

\* app:608-639, bus:267-281. WaitBrowseURL takes a cursor, then StartLogin.
LoginStarts ==
  /\ ~prompt
  /\ BeginLogin
  /\ UNCHANGED <<daemonUp, dState, dBrowse, busConn, holds, resend, latest,
                 mainQ, online, engine, winEngine, waiting, appState, switchOn,
                 needsAuthUI, prompt, runPc, runUp, runFail, lastUpAuth,
                 lastUpRan, lastUpPrompt, lastUpDone, downNoted>>

\* app:389-391: the confirmation's Accept calls ActivateAction("login").
UserConfirmsLogin ==
  /\ prompt
  /\ prompt' = FALSE
  /\ BeginLogin
  /\ UNCHANGED <<daemonUp, dState, dBrowse, busConn, holds, resend, latest,
                 mainQ, online, engine, winEngine, waiting, appState, switchOn,
                 needsAuthUI, runPc, runUp, runFail, lastUpAuth, lastUpRan,
                 lastUpPrompt, lastUpDone, downNoted>>

UserCancelsLogin ==
  /\ prompt
  /\ prompt' = FALSE
  /\ UNCHANGED <<daemonUp, dState, dBrowse, busConn, pending, holds, resend,
                 latest, mainQ, online, engine, winEngine, waiting, appState,
                 switchOn, needsAuthUI, runPc, runUp, runFail, loginPc, nextReady,
                 awaitingURL, sawURL, opened, lastUpAuth, lastUpRan,
                 lastUpPrompt, lastUpDone, downNoted, budget>>

\* The subscribe-after-StartLogin path, only when LoginWaitsAfterStart.
LoginSubscribes ==
  /\ LoginWaitsAfterStart
  /\ loginPc = "started"
  /\ loginPc' = "waiting"
  /\ nextReady' = FALSE
  /\ UNCHANGED <<daemonUp, dState, dBrowse, busConn, pending, holds, resend,
                 latest, mainQ, online, engine, winEngine, waiting, appState,
                 switchOn, needsAuthUI, prompt, runPc, runUp, runFail,
                 awaitingURL, sawURL, opened, lastUpAuth, lastUpRan,
                 lastUpPrompt, lastUpDone, downNoted, budget>>

\* bus:259-272, app:643-645. Next returns latest after a publish that
\* happened after it subscribed.
LoginSeesStatus ==
  /\ loginPc = "waiting"
  /\ nextReady
  /\ latest.ok
  /\ nextReady' = FALSE
  /\ IF latest.browse
     THEN /\ loginPc' = "opened"
          /\ opened' = TRUE
     ELSE /\ loginPc' = "waiting"
          /\ UNCHANGED opened
  /\ UNCHANGED <<daemonUp, dState, dBrowse, busConn, pending, holds, resend,
                 latest, mainQ, online, engine, winEngine, waiting, appState,
                 switchOn, needsAuthUI, prompt, runPc, runUp, runFail,
                 awaitingURL, sawURL, lastUpAuth, lastUpRan, lastUpPrompt,
                 lastUpDone, downNoted, budget>>

\* app:608, app:639-641: the 10s context times out waiting for Next.
\* Enabled only when nothing still in flight would unblock Next.
LoginTimesOut ==
  /\ loginPc = "waiting"
  /\ ~nextReady
  /\ ~pending.ok
  /\ ~awaitingURL
  /\ loginPc' = "failed"
  /\ UNCHANGED <<daemonUp, dState, dBrowse, busConn, pending, holds, resend,
                 latest, mainQ, online, engine, winEngine, waiting, appState,
                 switchOn, needsAuthUI, prompt, runPc, runUp, runFail, nextReady,
                 awaitingURL, sawURL, opened, lastUpAuth, lastUpRan,
                 lastUpPrompt, lastUpDone, downNoted, budget>>

\* ll:4728: control returns an auth URL after cc.Login.
DaemonOffersURL ==
  /\ awaitingURL
  /\ daemonUp
  /\ busConn
  /\ dBrowse' = TRUE
  /\ awaitingURL' = FALSE
  /\ pending' = St(dState, TRUE)
  /\ UNCHANGED <<daemonUp, dState, busConn, holds, resend, latest, mainQ,
                 online, engine, winEngine, waiting, appState, switchOn,
                 needsAuthUI, prompt, runPc, runUp, runFail, loginPc, nextReady,
                 sawURL, opened, lastUpAuth, lastUpRan, lastUpPrompt,
                 lastUpDone, downNoted, budget>>

\* ---------------- Daemon drop and reconnect ----------------

\* bus:141-160. A drop is modeled when SetRunning is idle, so the reconnect
\* invariant is checked without mixing it into an in-flight CLI call.
DaemonDrops ==
  /\ daemonUp
  /\ runPc = "idle"
  /\ loginPc \in {"idle", "opened", "failed"}
  /\ ~prompt
  /\ budget.drops < MaxDrops
  /\ daemonUp' = FALSE
  /\ busConn' = FALSE
  /\ downNoted' = FALSE
  /\ awaitingURL' = FALSE
  /\ budget' = [budget EXCEPT !.drops = @ + 1]
  /\ UNCHANGED <<dState, dBrowse, pending, holds, resend, latest, mainQ,
                 online, engine, winEngine, waiting, appState, switchOn,
                 needsAuthUI, prompt, runPc, runUp, runFail, loginPc, nextReady,
                 sawURL, opened, lastUpAuth, lastUpRan, lastUpPrompt, lastUpDone>>

\* bus:147-153: if the connection was up, watch sends a synthetic NoState
\* (empty prefs, so switchHold does not hold it).
WatchReportsDown ==
  /\ ~busConn
  /\ ~downNoted
  /\ downNoted' = TRUE
  /\ pending' = IF WatchSendsNoState THEN St("NoState", FALSE) ELSE pending
  /\ UNCHANGED <<daemonUp, dState, dBrowse, busConn, holds, resend, latest,
                 mainQ, online, engine, winEngine, waiting, appState, switchOn,
                 needsAuthUI, prompt, runPc, runUp, runFail, loginPc, nextReady,
                 awaitingURL, sawURL, opened, lastUpAuth, lastUpRan,
                 lastUpPrompt, lastUpDone, budget>>

DaemonReturns ==
  /\ ~daemonUp
  /\ downNoted
  /\ daemonUp' = TRUE
  /\ UNCHANGED <<dState, dBrowse, busConn, pending, holds, resend, latest,
                 mainQ, online, engine, winEngine, waiting, appState, switchOn,
                 needsAuthUI, prompt, runPc, runUp, runFail, loginPc, nextReady,
                 awaitingURL, sawURL, opened, lastUpAuth, lastUpRan,
                 lastUpPrompt, lastUpDone, downNoted, budget>>

\* bus:165-180, then the initial state notify (bus:184, bus:369-378).
WatchReconnects ==
  /\ daemonUp
  /\ ~busConn
  /\ downNoted
  /\ busConn' = TRUE
  /\ pending' = St(dState, dBrowse)
  /\ UNCHANGED <<daemonUp, dState, dBrowse, holds, resend, latest, mainQ,
                 online, engine, winEngine, waiting, appState, switchOn,
                 needsAuthUI, prompt, runPc, runUp, runFail, loginPc, nextReady,
                 awaitingURL, sawURL, opened, lastUpAuth, lastUpRan,
                 lastUpPrompt, lastUpDone, downNoted, budget>>

\* ---------------- Other bus events ----------------

\* bus:98-100: engine updates are not held.
EngineArrives ==
  /\ busConn
  /\ budget.engine < MaxEngine
  /\ budget' = [budget EXCEPT !.engine = @ + 1]
  /\ IF AtomicUI
     THEN /\ ApplyEngine
          /\ UNCHANGED mainQ
     ELSE /\ mainQ' = Append(mainQ, [kind |-> "engine"])
          /\ UNCHANGED <<engine, winEngine>>
  /\ UNCHANGED <<daemonUp, dState, dBrowse, busConn, pending, holds, resend,
                 latest, online, waiting, appState, switchOn,
                 needsAuthUI, prompt, runPc, runUp, runFail, loginPc, nextReady,
                 awaitingURL, sawURL, opened, lastUpAuth, lastUpRan,
                 lastUpPrompt, lastUpDone, downNoted>>

\* files:266-275: Files pushes only while Bus.latest is online.
FileArrives ==
  /\ busConn
  /\ (~FilesGatedOnline \/ (latest.ok /\ OnlineOf(latest.state)))
  /\ budget.files < MaxFiles
  /\ budget' = [budget EXCEPT !.files = @ + 1]
  /\ IF AtomicUI
     THEN /\ waiting' = TRUE
          /\ UNCHANGED mainQ
     ELSE /\ mainQ' = Append(mainQ, [kind |-> "files"])
          /\ UNCHANGED waiting
  /\ UNCHANGED <<daemonUp, dState, dBrowse, busConn, pending, holds, resend,
                 latest, online, engine, winEngine, appState, switchOn,
                 needsAuthUI, prompt, runPc, runUp, runFail, loginPc, nextReady,
                 awaitingURL, sawURL, opened, lastUpAuth, lastUpRan,
                 lastUpPrompt, lastUpDone, downNoted>>

\* ---------------- Next ----------------

Settled ==
  /\ runPc = "idle"
  /\ loginPc \in {"idle", "opened", "failed"}
  /\ ~prompt
  /\ ~pending.ok
  /\ ~resend
  /\ mainQ = <<>>
  /\ holds = 0
  /\ downNoted
  /\ busConn = daemonUp

Idle ==
  /\ Settled
  /\ UNCHANGED vars

Next ==
  \/ UserToggles
  \/ SetRunningFinishes
  \/ SetRunningQueuesResend
  \/ UserConfirmsLogin
  \/ UserCancelsLogin
  \/ LoginStarts
  \/ LoginSubscribes
  \/ LoginSeesStatus
  \/ LoginTimesOut
  \/ DaemonOffersURL
  \/ BusPublishes
  \/ BusResends
  \/ AppUpdates
  \/ DaemonDrops
  \/ WatchReportsDown
  \/ DaemonReturns
  \/ WatchReconnects
  \/ EngineArrives
  \/ FileArrives
  \/ Idle

Spec == Init /\ [][Next]_vars

\* ---------------- Properties ----------------

TypeOK ==
  /\ daemonUp \in BOOLEAN
  /\ dState \in STATES
  /\ dBrowse \in BOOLEAN
  /\ busConn \in BOOLEAN
  /\ pending.ok \in BOOLEAN
  /\ pending.state \in STATES
  /\ pending.browse \in BOOLEAN
  /\ holds \in {0, 1}
  /\ resend \in BOOLEAN
  /\ latest.ok \in BOOLEAN
  /\ latest.state \in STATES
  /\ online \in BOOLEAN
  /\ engine \in BOOLEAN
  /\ winEngine \in BOOLEAN
  /\ waiting \in BOOLEAN
  /\ appState \in STATES
  /\ switchOn \in BOOLEAN
  /\ needsAuthUI \in BOOLEAN
  /\ prompt \in BOOLEAN
  /\ runPc \in {"idle", "held", "releasing"}
  /\ loginPc \in {"idle", "started", "waiting", "opened", "failed"}
  /\ nextReady \in BOOLEAN
  /\ awaitingURL \in BOOLEAN
  /\ sawURL \in BOOLEAN
  /\ opened \in BOOLEAN
  /\ lastUpAuth \in BOOLEAN
  /\ lastUpRan \in BOOLEAN
  /\ lastUpPrompt \in BOOLEAN
  /\ lastUpDone \in BOOLEAN
  /\ downNoted \in BOOLEAN

\* When offline and the main thread has caught up, engine and waiting are
\* cleared, and the offline page shows the login group only for NeedsLogin.
OfflineIdleAndCleared ==
  (~online /\ mainQ = <<>>) =>
    /\ ~engine
    /\ ~winEngine
    /\ ~waiting
    /\ needsAuthUI = NeedsAuthOf(appState)

\* Toggling connect when Latest is NeedsLogin asks for auth rather than
\* calling SetRunning(true).
NeedsAuthPromptsLogin ==
  (lastUpDone /\ lastUpAuth) => (~lastUpRan /\ lastUpPrompt)

\* Starting App.login reliably observes a BrowseToURL that tailscaled
\* published for that attempt.
LoginSeesBrowseURL ==
  (loginPc \in {"idle", "opened", "failed"} /\ sawURL) => opened

\* A SetRunning hold always releases, and after a failed call has settled
\* the window switch matches the shown state (the last status was resent
\* so the UI can undo what it anticipated).
SetRunningHoldReleases ==
  /\ (runPc = "idle" => holds = 0)
  /\ ((runPc = "idle" /\ ~pending.ok /\ ~resend /\ mainQ = <<>>) =>
        switchOn = online)

\* When tailscaled drops and comes back, Bus reconnects and App.online
\* converges to the daemon's real state.
DaemonReconnectResyncs ==
  Settled =>
    IF daemonUp
    THEN /\ latest.ok
         /\ latest.state = dState
         /\ online = OnlineOf(dState)
         /\ appState = dState
    ELSE /\ ~busConn
         /\ ~online
         /\ WatchSendsNoState => (latest.ok /\ latest.state = "NoState" /\ appState = "NoState")

\* The system is reactive and always has a Next, including Idle in settled
\* states. This just checks that a hold cannot be left behind.
NoDeadlock ==
  runPc = "idle" => holds = 0
=============================================================================

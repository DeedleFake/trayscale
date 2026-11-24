-------------------------- MODULE ProfilesAndFiles --------------------------
\* Trayscale showing tailscaled's login profiles and waiting Taildrop files,
\* with Bus, Profiles, and Files from internal/tsutil and the App and window
\* code that applies what they publish.
\*
\* Trayscale paths: bus = internal/tsutil/bus.go, src = internal/tsutil/sources.go,
\* files = internal/tsutil/files.go, app = internal/ui/app.go,
\* win = internal/ui/mainwindow.go, self = internal/ui/selfpage.go,
\* io = internal/ui/io.go. Tailscale paths, in the module cache:
\* ll = ipn/ipnlocal/local.go, td = feature/taildrop/ext.go.
\*
\* Goroutines: Bus.Run, Profiles.Run (fetchLoop), Files.Run (filesRun.run
\* and its check goroutine), the GTK main thread, and one goroutine per
\* background switch, delete, or SetRunning. gutil.IdleAdd callbacks run
\* on the main thread in the order they were added, so they are the
\* sequence mainQ. Each LocalAPI call is one step, because tailscaled
\* serves it under b.mu.
\*
\* The constants other than the bounds are code facts that the check script
\* reads from the code. TRUE means the code has the guard.
EXTENDS Naturals, Sequences, FiniteSets

CONSTANTS
  Part,                    \* bound: "profiles" or "files", which loop and which user actions run
  MaxSwitches, MaxReceives, MaxOutsideDeletes, MaxListChanges, MaxLooks,
  MaxDeletes, MaxToggles,
  Resends,                 \* bound: a file can arrive under the name of one that waited before
  ProfilesOnState,         \* src: profilesTriggers has changeState
  FilesResetOnState,       \* files: filesResets has changeState
  ResetOnPublished,        \* files: drain resets when the latest published state changed
  FilesGatedOnline,        \* files: run only checks while Bus's latest status is online
  DiscardStaleFetch,       \* files: run drops a list when a reset fired while checking
  PollWhileWaiting,        \* files: run checks again after Interval while files wait
  BaselineQuiet,           \* files: push passes the first list after a reset with no Arrived
  RefreshRepeats,          \* files: a Refresh during a check that isn't waiting repeats it
  RefreshBuffered,         \* files, src: Refresh sends to a 1-buffered channel without blocking
  SwitchRefreshes,         \* src: Profiles.Switch defers p.Refresh()
  DeleteRefreshes,         \* files: Files.Delete defers f.Refresh()
  WindowOpenRefreshes,     \* app: onAppActivate calls a.files.Refresh() and a.profiles.Refresh()
  DropDownRefreshes,       \* win: the profile drop-down's popup calls app.profiles.Refresh
  TrayOpenRefreshes,       \* app: OnAccountsOpen calls a.profiles.Refresh()
  FocusRefreshes,          \* win: the window becoming active calls app.profiles.Refresh
  TraySwitchOffMain,       \* app: OnProfileSwitch calls a.profiles.Switch in a goroutine
  TraySwitchFailureNotifies, \* app: a failed tray switch sends a "Switch account" notification
  DropDownSwitchOffMain,   \* win: the drop-down's selected-item handler calls Switch in a goroutine
  DeleteOffMain,           \* self: the delete confirmation calls a.files.Delete in a goroutine
  SelfPageFillsFiles,      \* self: SelfPage.init fills fileModel from a.waiting
  NoticesFromArrivals,     \* app: a New Incoming File notification names only a file in FileStatus.Arrived
  HoldGatesFiles,          \* app: a FileStatus doesn't reach the window during a hold
  HoldReplaysFiles,        \* app: a hold's end shows a.waiting in the window
  SwitchHold,              \* app: App.update holds NoState and Starting back while online and WantRunning
  HoldDisables,            \* app: a hold's start calls a.setSwitching(true)
  HoldReenables,           \* app: both ends of a hold call a.setSwitching(false)
  HoldFallback             \* app: a hold's start arms the switchTimeout fallback

Profiles == {"A", "B"}
Files == {"f1", "f2"}

NoGen == [f \in Files |-> 0]
StartFiles == [p \in Profiles |-> IF p = "A" THEN {"f1"} ELSE {"f1", "f2"}]
NoFiles == [ok |-> FALSE, p |-> "none", fs |-> {}, gen |-> NoGen]
NoProfiles == [ok |-> FALSE, cur |-> "none", list |-> {}]
NoOp == [k |-> "none", src |-> "none", to |-> "none", f |-> "none", p |-> "none"]
\* A list from waitingFiles: the files, with the profile it was read from
\* and each file's arrival as ghosts. The code only has the names.
NoRes == [p |-> "none", fs |-> {}, gen |-> NoGen]
Msg(k) == [k |-> k, st |-> "none", cur |-> "none", list |-> {}, p |-> "none", fs |-> {}, gen |-> NoGen, to |-> "none", arr |-> {}]
Notify(k, st) == [k |-> k, st |-> st]

VARIABLES
  \* tailscaled
  cur,        \* the current profile
  known,      \* the profiles that exist
  dstate,     \* "Running", "Stopped", or "NoState" then "Starting" during a profile switch
  files,      \* waiting files per profile: td keeps a directory per login
  busQ,       \* notifies tailscaled has sent that Bus hasn't read yet
  \* Bus.Run
  busState,   \* watchState.ipn.State
  pending,    \* the status Run will publish next, or "none"
  busLatest,  \* b.latest's state, "NoState" while it's nil
  hold,       \* holds from SetRunning
  runPhase,   \* the one SetRunning in flight (b.runMu): "idle", "held", "called"
  \* Profiles.Run (fetchLoop, src)
  pPhase, pSig, pRef, pRes,
  \* Files.Run (filesRun, files)
  fPhase,     \* "off": waiting offline; "check": a check in flight; "got": its list in hand; "wait": waiting to poll
  fLong,      \* the check in flight waits for a file to arrive
  fAgain,     \* check's again: a Refresh came during a check that doesn't wait
  fTimer,     \* the Interval timer is armed
  fRef,       \* a Refresh is buffered in f.refresh
  fSig,       \* a signal in filesResets fired that drain hasn't taken
  fState,     \* r.state
  fLast,      \* r.last's names
  fBase,      \* r.baseline
  fBaseP,     \* ghost: the profile r.last's baseline was read from
  fRes,       \* the list the check returned
  \* the main thread
  mainQ,      \* IdleAdd callbacks not run yet
  mainWait,   \* a LocalAPI call the main thread is blocked in, or NoOp
  ops,        \* background switch and delete goroutines in flight
  online,     \* a.online
  waiting,    \* a.waiting, with the profile it was read from as a ghost
  appProf,    \* a.profileStatus, which the tray shows
  winOpen,    \* a.win != nil
  winProf,    \* the window's profiles and activeProfileID
  dropSel,    \* the profile the drop-down shows as selected
  selfPage,   \* the self page's file list, with its profile as a ghost; ok is FALSE when there's no self page
  heldSt,     \* app: a.hold.held's state, or "none" when no switch is held
  timerArmed, \* app: a switchTimeout fallback that hasn't fired yet
  trayOff,    \* the tray's Tailscale items are disabled (Tray.SetSwitching)
  winOff,     \* the window's content and app actions are disabled (MainWindow.SetSwitching, syncActions)
  \* ghosts for the properties
  unseenProf, \* a profile change nothing reports happened since the user last opened a profile list
  fDirty,     \* ghost: the current waiting files changed since Files last read them
  gen,        \* ghost: per profile and file, how many times it has arrived; 0 means it was there from the start
  noticed,    \* ghost: per profile and file, the latest arrival a notification or a first list has accounted for
  owed,       \* ghost: per profile and file, the latest arrival that Trayscale owes a notification
  told,       \* ghost: per profile and file, the latest arrival a New Incoming File notification named
  stray,      \* a New Incoming File notification went out for a file that never arrived in that profile
  dup,        \* a New Incoming File notification went out again for an arrival already accounted for
  wrongDelete,\* a delete removed a file from a different profile than the list it was picked from
  failedTray, \* tray switches that failed
  notices,    \* "Switch account" notifications shown
  stuck,      \* ghost: the switch stalled in Starting, a real failure, so the fallback may fire
  flap,       \* ghost: a switch that didn't stall showed offline, with its disconnect notification
  budget

daemonVars == <<cur, known, dstate, files>>
busVars == <<busQ, busState, pending, busLatest, hold, runPhase>>
pVars == <<pPhase, pSig, pRef, pRes>>
fVars == <<fPhase, fLong, fAgain, fTimer, fRef, fSig, fState, fLast, fBase, fBaseP, fRes>>
holdVars == <<heldSt, timerArmed, trayOff, winOff>>
uiVars == <<online, waiting, appProf, winOpen, winProf, dropSel, selfPage, heldSt, timerArmed, trayOff, winOff>>
noteVars == <<noticed, told, stray, dup>>
ghostVars == <<unseenProf, fDirty, gen, noticed, owed, told, stray, dup, wrongDelete, failedTray, notices, stuck, flap>>
vars == <<daemonVars, busVars, pVars, fVars, mainQ, mainWait, ops, uiVars, ghostVars, budget>>

\* The loop that a Part leaves out gets no signals or refreshes.
PMask == IF Part # "profiles" THEN {}
         ELSE {"connected"} \cup (IF ProfilesOnState THEN {"state"} ELSE {})
FMask == IF Part # "files" THEN {}
         ELSE {"connected"} \cup (IF FilesResetOnState THEN {"state"} ELSE {})

\* kick, src. With an unbuffered channel the send only lands while the
\* loop is blocked in a select on it: await for Profiles, next for Files.
KickP == IF Part = "profiles" /\ (RefreshBuffered \/ pPhase = "idle") THEN TRUE ELSE pRef
KickF == IF Part = "files" /\ (RefreshBuffered \/ fPhase \in {"off", "check", "wait"}) THEN TRUE ELSE fRef

\* The bus's select only runs when nothing is waiting to be published.
BusSelects == pending = "none" \/ hold > 0

\* The profiles Part leaves the self page out.
SelfInit == IF Part # "files" THEN NoFiles
            ELSE IF SelfPageFillsFiles /\ waiting.ok
            THEN [waiting EXCEPT !.ok = TRUE]   \* SelfPage.init
            ELSE [NoFiles EXCEPT !.ok = TRUE]

Init ==
  /\ cur = "A" /\ known = Profiles /\ dstate = "Running"
  /\ files = StartFiles
  \* watchOnce's changeConnected, then the initial state
  /\ busQ = <<Notify("connected", "none"), Notify("state", "Running")>>
  /\ busState = "NoState" /\ pending = "none" /\ busLatest = "NoState" /\ hold = 0 /\ runPhase = "idle"
  \* fetchLoop fetches once at the start, which a pending refresh stands for.
  /\ pPhase = "idle" /\ pSig = FALSE /\ pRef = (Part = "profiles") /\ pRes = NoProfiles
  \* Files.Run starts offline, as b.latest is nil.
  /\ fPhase = "off" /\ fLong = FALSE /\ fAgain = FALSE /\ fTimer = FALSE /\ fRef = FALSE
  /\ fSig = FALSE /\ fState = "NoState" /\ fLast = {} /\ fBase = FALSE /\ fBaseP = "none" /\ fRes = NoRes
  /\ mainQ = <<>> /\ mainWait = NoOp /\ ops = {}
  /\ online = FALSE /\ waiting = NoFiles /\ appProf = NoProfiles
  /\ winOpen = FALSE /\ winProf = NoProfiles /\ dropSel = "none" /\ selfPage = NoFiles
  /\ heldSt = "none" /\ timerArmed = FALSE /\ trayOff = FALSE /\ winOff = FALSE
  /\ unseenProf = FALSE /\ fDirty = (Part = "files")
  /\ gen = [p \in Profiles |-> NoGen] /\ noticed = [p \in Profiles |-> NoGen]
  /\ owed = [p \in Profiles |-> NoGen] /\ told = [p \in Profiles |-> NoGen]
  /\ stray = FALSE /\ dup = FALSE /\ wrongDelete = FALSE /\ failedTray = 0 /\ notices = 0
  /\ stuck = FALSE /\ flap = FALSE
  /\ budget = [switch |-> MaxSwitches, recv |-> MaxReceives, odel |-> MaxOutsideDeletes,
               list |-> MaxListChanges, look |-> MaxLooks, del |-> MaxDeletes,
               toggle |-> MaxToggles]

Spend(k) == budget' = [budget EXCEPT ![k] = @ - 1]

\* ---------------- tailscaled and other clients ----------------

\* SwitchProfile, ll:8383-8398, and resetForProfileChangeLocked, ll:8414-8455:
\* the taildrop manager moves to the new login (td:149-195) and the state
\* becomes NoState under b.mu, then startLocked brings it back up.
\* Returns the error as a boolean.
SwitchFails(p) == p \notin known
DoSwitch(p) ==
  IF SwitchFails(p) \/ p = cur THEN UNCHANGED <<cur, dstate, busQ, fDirty>>
  ELSE /\ cur' = p /\ dstate' = "NoState" /\ fDirty' = TRUE
       /\ busQ' = busQ \o (IF dstate # "NoState" THEN <<Notify("state", "NoState")>> ELSE <<>>)

\* `tailscale switch` from the CLI or another client.
OtherClientSwitches(p) ==
  /\ budget.switch > 0 /\ p # cur /\ p \in known
  /\ DoSwitch(p) /\ Spend("switch")
  /\ UNCHANGED <<known, files, busState, pending, busLatest, hold, runPhase, pVars, fVars, mainQ, mainWait, ops, uiVars,
                 unseenProf, gen, noticed, owed, told, stray, dup, wrongDelete, failedTray, notices, stuck, flap>>

\* startLocked after a switch: Starting with WantRunning still set, then
\* Running once the new profile's control server answers. Seen on the bus
\* as NoState, Starting, Running about 250ms apart with tailscaled
\* v1.104.4. s is TRUE when the switch stalls in Starting, as when the
\* control server doesn't answer: a real failure.
TailscaledStarts(s) ==
  /\ dstate = "NoState" /\ dstate' = "Starting" /\ stuck' = s
  /\ busQ' = Append(busQ, Notify("state", "Starting"))
  /\ UNCHANGED <<cur, known, files, busState, pending, busLatest, hold, runPhase, pVars, fVars, mainQ, mainWait, ops, uiVars,
                 unseenProf, fDirty, gen, noticed, owed, told, stray, dup, wrongDelete, failedTray, notices, flap, budget>>

TailscaledComesBackUp ==
  /\ dstate = "Starting" /\ dstate' = "Running" /\ stuck' = FALSE
  /\ busQ' = Append(busQ, Notify("state", "Running"))
  /\ UNCHANGED <<cur, known, files, busState, pending, busLatest, hold, runPhase, pVars, fVars, mainQ, mainWait, ops, uiVars,
                 unseenProf, fDirty, gen, noticed, owed, told, stray, dup, wrongDelete, failedTray, notices, flap, budget>>

\* A Taildrop transfer finishes: PutFile (feature/taildrop/send.go) renames
\* it into place as f, then wakes AwaitWaitingFiles. Rename keeps the name
\* when no other file has it. Files doesn't follow the bus's deprecated
\* IncomingFiles, so the bus is left out. Trayscale owes a notification
\* for an arrival once Files has passed a first list for this profile to
\* New; a file that arrives before that is listed quietly with it, like
\* the files waiting at launch.
FileArrives(f) ==
  /\ Part = "files" /\ budget.recv > 0 /\ dstate = "Running" /\ f \notin files[cur]
  /\ Resends \/ (f \notin StartFiles[cur] /\ gen[cur][f] = 0)
  /\ files' = [files EXCEPT ![cur] = @ \cup {f}]
  /\ gen' = [gen EXCEPT ![cur][f] = @ + 1]
  /\ fDirty' = TRUE
  /\ owed' = IF fBase /\ fBaseP = cur THEN [owed EXCEPT ![cur][f] = gen'[cur][f]] ELSE owed
  /\ Spend("recv")
  /\ UNCHANGED <<cur, known, dstate, busVars, pVars, fVars, mainQ, mainWait, ops, uiVars,
                 unseenProf, noticed, told, stray, dup, wrongDelete, failedTray, notices, stuck, flap>>

\* `tailscale file get` or another client takes a waiting file. Nothing
\* reports it, so only a check finds it.
OtherClientTakesFile(f) ==
  /\ Part = "files" /\ budget.odel > 0 /\ dstate = "Running" /\ f \in files[cur]
  /\ files' = [files EXCEPT ![cur] = @ \ {f}]
  /\ fDirty' = TRUE
  /\ Spend("odel")
  /\ UNCHANGED <<cur, known, dstate, busVars, pVars, fVars, mainQ, mainWait, ops, uiVars,
                 unseenProf, gen, noticed, owed, told, stray, dup, wrongDelete, failedTray, notices, stuck, flap>>

\* Another client adds or deletes a profile that isn't the current one.
\* Nothing reports it.
OtherClientChangesProfiles(q) ==
  /\ Part = "profiles" /\ budget.list > 0 /\ q # cur
  /\ known' = IF q \in known THEN known \ {q} ELSE known \cup {q}
  /\ unseenProf' = TRUE
  /\ Spend("list")
  /\ UNCHANGED <<cur, dstate, files, busVars, pVars, fVars, mainQ, mainWait, ops, uiVars,
                 fDirty, gen, noticed, owed, told, stray, dup, wrongDelete, failedTray, notices, stuck, flap>>

\* ---------------- Bus.Run ----------------

\* One notify: watchState.apply decides the changes, then Run sets
\* pending and fires the changes before it publishes.
BusReads ==
  /\ busQ # <<>> /\ BusSelects
  /\ LET n == Head(busQ)
         changes == CASE n.k = "connected" -> {"connected"}
                      [] n.k = "state" -> (IF n.st # busState THEN {"state"} ELSE {})
     IN /\ busQ' = Tail(busQ)
        /\ busState' = IF n.k = "state" THEN n.st ELSE busState
        /\ pending' = IF n.k = "state" THEN n.st ELSE pending
        /\ pSig' = (pSig \/ changes \cap PMask # {})
        /\ fSig' = (fSig \/ changes \cap FMask # {})
  /\ UNCHANGED <<daemonVars, busLatest, hold, runPhase, pPhase, pRef, pRes,
                 fPhase, fLong, fAgain, fTimer, fRef, fState, fLast, fBase, fBaseP, fRes,
                 mainQ, mainWait, ops, uiVars, ghostVars, budget>>

\* publish: b.latest.Swap, then Bus.New does gutil.IdleAdd(a.update), then
\* changePublished fires, which wakes Files to compare the state.
BusPublishes ==
  /\ pending # "none" /\ hold = 0
  /\ mainQ' = Append(mainQ, [Msg("ipn") EXCEPT !.st = pending])
  /\ busLatest' = pending
  /\ pending' = "none"
  /\ UNCHANGED <<daemonVars, busQ, busState, hold, runPhase, pVars, fVars, mainWait, ops, uiVars, ghostVars, budget>>

\* SetRunning, from the window's status switch or the tray's connection
\* toggle, both in goroutines. It holds publishing, but not signals, while
\* `tailscale up` or `down` runs.
SetRunningHolds ==
  /\ budget.toggle > 0 /\ runPhase = "idle" /\ dstate \in {"Running", "Stopped"} /\ BusSelects
  /\ ~trayOff \/ (winOpen /\ ~winOff)
  /\ hold' = 1 /\ runPhase' = "held" /\ Spend("toggle")
  /\ UNCHANGED <<daemonVars, busQ, busState, pending, busLatest, pVars, fVars, mainQ, mainWait, ops, uiVars, ghostVars>>

SetRunningCalls ==
  /\ runPhase = "held" /\ runPhase' = "called"
  /\ CASE dstate = "Running" -> dstate' = "Stopped" /\ busQ' = Append(busQ, Notify("state", "Stopped"))
       [] dstate = "Stopped" -> dstate' = "Running" /\ busQ' = Append(busQ, Notify("state", "Running"))
       [] dstate \in {"NoState", "Starting"} -> UNCHANGED <<dstate, busQ>>
  /\ UNCHANGED <<cur, known, files, busState, pending, busLatest, hold, pVars, fVars, mainQ, mainWait, ops, uiVars, ghostVars, budget>>

SetRunningReleases ==
  /\ runPhase = "called" /\ BusSelects
  /\ hold' = 0 /\ runPhase' = "idle"
  /\ UNCHANGED <<daemonVars, busQ, busState, pending, busLatest, pVars, fVars, mainQ, mainWait, ops, uiVars, ghostVars, budget>>

\* ---------------- Profiles.Run, src ----------------

\* await returns on a signal in mask or a refresh and drains the rest,
\* then getProfileStatus is one LocalAPI call. A change between the drain
\* and the call fires a signal that the next await sees, so taking both as
\* one step only drops a fetch that repeats this one.
ProfilesFetch ==
  /\ Part = "profiles" /\ pPhase = "idle" /\ (pSig \/ pRef)
  /\ pPhase' = "deliver" /\ pSig' = FALSE /\ pRef' = FALSE
  /\ pRes' = [ok |-> TRUE, cur |-> cur, list |-> known]
  /\ UNCHANGED <<daemonVars, busVars, fVars, mainQ, mainWait, ops, uiVars, ghostVars, budget>>

\* p.New does gutil.IdleAdd(a.update).
ProfilesPublish ==
  /\ pPhase = "deliver" /\ pPhase' = "idle"
  /\ mainQ' = Append(mainQ, [Msg("profiles") EXCEPT !.cur = pRes.cur, !.list = pRes.list])
  /\ UNCHANGED <<daemonVars, busVars, pSig, pRef, pRes, fVars, mainWait, ops, uiVars, ghostVars, budget>>

\* ---------------- Files.Run, files ----------------

\* drain finds a reset: a signal in filesResets fired, or the latest
\* published state isn't r.state. Each publish fires changePublished, so a
\* changed state always wakes drain.
ResetPending == fSig \/ (ResetOnPublished /\ busLatest # fState)

\* The top of run's loop, with r.last and r.baseline as last and base:
\* offline, reset and wait for a signal or a Refresh; online, start a
\* check, which waits for a file to arrive if the last list passed to New
\* was empty.
Top(last, base) ==
  IF FilesGatedOnline /\ busLatest # "Running"
  THEN fPhase' = "off" /\ fLast' = {} /\ fBase' = FALSE /\ fLong' = FALSE /\ fAgain' = FALSE
  ELSE fPhase' = "check" /\ fLast' = last /\ fBase' = base /\ fLong' = (base /\ last = {}) /\ fAgain' = FALSE

\* next, or check's own loop, wakes on a signal and drain resets: the
\* timer and a check in flight are dropped. In "got", run calls drain
\* right after check returns.
FilesResets ==
  /\ Part = "files" /\ ResetPending
  /\ fPhase \in {"off", "check", "wait"} \/ (fPhase = "got" /\ DiscardStaleFetch)
  /\ fSig' = FALSE /\ fState' = busLatest /\ fTimer' = FALSE /\ fRes' = NoRes
  /\ Top({}, FALSE)
  /\ UNCHANGED <<daemonVars, busVars, pVars, fRef, fBaseP, mainQ, mainWait, ops, uiVars, ghostVars, budget>>

\* next returns filesRefresh while offline or waiting to poll.
FilesRefreshWakes ==
  /\ Part = "files" /\ fRef /\ fPhase \in {"off", "wait"}
  /\ fRef' = FALSE /\ fTimer' = FALSE
  /\ Top(fLast, fBase)
  /\ UNCHANGED <<daemonVars, busVars, pVars, fSig, fState, fBaseP, fRes, mainQ, mainWait, ops, uiVars, ghostVars, budget>>

\* check's loop takes a Refresh: one that doesn't wait for a file is
\* repeated after it returns, as its list may predate the Refresh.
FilesRefreshDuringCheck ==
  /\ Part = "files" /\ fRef /\ fPhase = "check"
  /\ fRef' = FALSE /\ fAgain' = (fAgain \/ (RefreshRepeats /\ ~fLong))
  /\ UNCHANGED <<daemonVars, busVars, pVars, fPhase, fLong, fTimer, fSig, fState, fLast, fBase, fBaseP, fRes,
                 mainQ, mainWait, ops, uiVars, ghostVars, budget>>

\* The Interval timer fires.
FilesTimer ==
  /\ Part = "files" /\ fPhase = "wait" /\ fTimer
  /\ fTimer' = FALSE
  /\ Top(fLast, fBase)
  /\ UNCHANGED <<daemonVars, busVars, pVars, fRef, fSig, fState, fBaseP, fRes, mainQ, mainWait, ops, uiVars, ghostVars, budget>>

\* waitingFiles: AwaitWaitingFiles against the current login's manager.
\* With a wait it answers once any file is waiting. Its deadline passing
\* with none waiting answers an empty list, which push drops as unchanged
\* before the next long poll, so the model leaves it out.
FilesFetch ==
  /\ Part = "files" /\ fPhase = "check" /\ (~fLong \/ files[cur] # {})
  /\ fPhase' = "got"
  /\ fRes' = [p |-> cur, fs |-> files[cur],
              gen |-> [f \in Files |-> IF f \in files[cur] THEN gen[cur][f] ELSE 0]]
  /\ fDirty' = FALSE
  /\ UNCHANGED <<daemonVars, busVars, pVars, fLong, fAgain, fTimer, fRef, fSig, fState, fLast, fBase, fBaseP,
                 mainQ, mainWait, ops, uiVars, unseenProf, gen, noticed, owed, told, stray, dup,
                 wrongDelete, failedTray, notices, stuck, flap, budget>>

\* push, then the rest of run's loop: New with the first list after a
\* reset, or with a changed list and the files in it that weren't in the
\* last one (arrivedFiles, which compares names and sizes; the model has
\* names). An empty list goes straight to a long poll; a list with files
\* arms the Interval timer, unless a Refresh came during the check.
FilesPush ==
  /\ Part = "files" /\ fPhase = "got" /\ (~ResetPending \/ ~DiscardStaleFetch)
  /\ LET fs == fRes.fs
         first == ~fBase
         arr == IF first THEN (IF BaselineQuiet THEN {} ELSE fs) ELSE fs \ fLast
     IN /\ mainQ' = IF first \/ fs # fLast
                    THEN Append(mainQ, [Msg("files") EXCEPT !.p = fRes.p, !.fs = fs, !.gen = fRes.gen, !.arr = arr])
                    ELSE mainQ
        /\ fBaseP' = IF first THEN fRes.p ELSE fBaseP
        /\ IF fs = {} \/ fAgain
           THEN fTimer' = FALSE /\ Top(fs, TRUE)
           ELSE /\ fPhase' = "wait" /\ fTimer' = PollWhileWaiting /\ fLast' = fs /\ fBase' = TRUE
                /\ UNCHANGED <<fLong, fAgain>>
  /\ fRes' = NoRes
  /\ UNCHANGED <<daemonVars, busVars, pVars, fRef, fSig, fState, mainWait, ops, uiVars, ghostVars, budget>>

\* ---------------- Switches and deletes ----------------

\* Profiles.Switch, src, from the tray (OnProfileSwitch) or the drop-down
\* (its selected-item handler), and Files.Delete, files, from the self page
\* (fileRow's delete button) or saveFile (io). Each is one step: the
\* LocalAPI call, then the deferred Refresh, then for the tray the failure
\* notification's IdleAdd.
Finish(op) ==
  IF op.k = "switch" THEN
    /\ DoSwitch(op.to)
    /\ pRef' = IF SwitchRefreshes THEN KickP ELSE pRef
    /\ IF op.src = "tray" /\ SwitchFails(op.to)
       THEN /\ failedTray' = failedTray + 1
            /\ mainQ' = IF TraySwitchFailureNotifies THEN Append(mainQ, Msg("notice")) ELSE mainQ
       ELSE UNCHANGED <<failedTray, mainQ>>
    /\ UNCHANGED <<known, files, fRef, wrongDelete>>
  ELSE
    /\ wrongDelete' = (wrongDelete \/ (op.p # cur /\ op.f \in files[cur]))
    /\ files' = [files EXCEPT ![cur] = @ \ {op.f}]
    /\ fDirty' = (fDirty \/ op.f \in files[cur])
    /\ fRef' = IF DeleteRefreshes THEN KickF ELSE fRef
    /\ UNCHANGED <<cur, known, dstate, busQ, pRef, failedTray, mainQ>>

FinishUnchanged ==
  UNCHANGED <<busState, pending, busLatest, hold, runPhase, pPhase, pSig, pRes,
              fPhase, fLong, fAgain, fTimer, fSig, fState, fLast, fBase, fBaseP, fRes, uiVars,
              unseenProf, gen, noticed, owed, told, stray, dup, notices, stuck, flap, budget>>

BackgroundFinishes(op) ==
  /\ op \in ops /\ ops' = ops \ {op}
  /\ Finish(op)
  /\ FinishUnchanged /\ UNCHANGED mainWait

MainThreadFinishes ==
  /\ mainWait # NoOp /\ mainWait' = NoOp
  /\ Finish(mainWait)
  /\ FinishUnchanged /\ UNCHANGED ops

\* Start op on the main thread or in a goroutine.
Launch(op, offMain) ==
  IF offMain THEN ops' = ops \cup {op} /\ UNCHANGED mainWait
  ELSE mainWait' = op /\ UNCHANGED ops

\* ---------------- The main thread ----------------

MainFree == mainWait = NoOp

\* showIPN and MainWindow.Update for st. Offline, a.waiting goes. The self
\* page goes offline (SelfPage.UpdateIPN returns false and updatePeers
\* removes it) and comes back online (ensureSectionPages). holdSidebar
\* detaches the sidebar around those page changes within this step and
\* changes nothing modeled.
ShowWaiting(st) == IF st = "Running" THEN waiting ELSE NoFiles
ShowPage(st) ==
  IF ~winOpen \/ st # "Running" THEN NoFiles
  ELSE IF selfPage.ok THEN selfPage
  ELSE SelfInit

\* replay is a hold ending: setSwitching(false) then syncWindow shows
\* a.waiting, which update kept from the window during the hold.
Show(st, replay) ==
  /\ online' = (st = "Running")
  /\ waiting' = ShowWaiting(st)
  /\ selfPage' = IF replay /\ ShowPage(st).ok /\ ShowWaiting(st).ok
                 THEN ShowWaiting(st) ELSE ShowPage(st)
  /\ flap' = (flap \/ (online /\ st # "Running" /\ st \in {"NoState", "Starting"} /\ ~stuck))

\* setSwitching: the tray, an open window, and the app actions. A window
\* that opens later gets the hold's state from onAppActivate.
SetSwitching(on) ==
  /\ trayOff' = on
  /\ winOff' = (winOpen /\ on)

\* App.update's IPNStatus case, with switchHold.offer,
\* internal/ui/switchhold.go. In this model WantRunning is set in every
\* NoState and Starting status, because they only come from a switch.
ApplyIPN(m) ==
  /\ IF SwitchHold /\ online /\ m.st \in {"NoState", "Starting"} THEN
       /\ heldSt' = m.st
       /\ waiting' = NoFiles
       /\ IF heldSt = "none"
          THEN /\ timerArmed' = (timerArmed \/ HoldFallback)
               /\ IF HoldDisables THEN SetSwitching(TRUE) ELSE UNCHANGED <<trayOff, winOff>>
          ELSE UNCHANGED <<timerArmed, trayOff, winOff>>
       /\ UNCHANGED <<online, selfPage, flap>>
     ELSE
       /\ heldSt' = "none"
       /\ Show(m.st, heldSt # "none" /\ HoldReenables /\ HoldReplaysFiles)
       /\ IF heldSt # "none" /\ HoldReenables THEN SetSwitching(FALSE) ELSE UNCHANGED <<trayOff, winOff>>
       /\ UNCHANGED timerArmed
  /\ UNCHANGED <<appProf, winOpen, winProf, dropSel, ops, mainWait, noteVars, notices>>

\* The fallback's TimeoutAdd callback: switchTimeout after the hold
\* starts, show the latest held status. A switch that doesn't stall
\* reaches Running well within switchTimeout, so the timer only fires
\* after a stall or after the hold has already ended.
HoldExpires ==
  /\ MainFree /\ timerArmed /\ (stuck \/ heldSt = "none")
  /\ timerArmed' = FALSE
  /\ IF heldSt = "none"
     THEN UNCHANGED <<online, waiting, appProf, winOpen, winProf, dropSel, selfPage, heldSt, trayOff, winOff, flap>>
     ELSE /\ heldSt' = "none"
          /\ Show(heldSt, HoldReenables /\ HoldReplaysFiles)
          /\ IF HoldReenables THEN SetSwitching(FALSE) ELSE UNCHANGED <<trayOff, winOff>>
          /\ UNCHANGED <<appProf, winOpen, winProf, dropSel>>
  /\ UNCHANGED <<daemonVars, busVars, pVars, fVars, mainQ, mainWait, ops,
                 unseenProf, fDirty, gen, noteVars, owed, wrongDelete, failedTray, notices, stuck, budget>>

\* App.update's FileStatus case: a file in the new list whose name is in
\* Arrived gets a notification. With NoticesFromArrivals FALSE, a file in
\* the new list that isn't in a.waiting gets one, unless a.waiting is nil.
\* During a hold the window keeps the old profile's list.
ApplyFiles(m) ==
  LET quiet == ~NoticesFromArrivals /\ ~waiting.ok
      new == IF NoticesFromArrivals THEN m.fs \cap m.arr
             ELSE IF quiet THEN {} ELSE m.fs \ waiting.fs
      \* A list taken quietly accounts for the arrival of each file in it.
      seen == IF quiet THEN m.fs ELSE new
      \* The arrival a notification for f stands for: the one in the
      \* list's profile when it was read. 0 is none, a file that was there
      \* from the start.
      credit(f) == m.gen[f]
  IN
  /\ stray' = (stray \/ \E f \in new : credit(f) = 0)
  /\ dup' = (dup \/ \E f \in new : credit(f) > 0 /\ credit(f) <= noticed[m.p][f])
  /\ noticed' = [noticed EXCEPT ![m.p] = [f \in Files |->
                   IF f \in seen /\ credit(f) > @[f] THEN credit(f) ELSE @[f]]]
  /\ told' = [told EXCEPT ![m.p] = [f \in Files |-> IF f \in new /\ credit(f) > @[f] THEN credit(f) ELSE @[f]]]
  /\ waiting' = [ok |-> TRUE, p |-> m.p, fs |-> m.fs, gen |-> m.gen]
  /\ selfPage' = IF selfPage.ok /\ (heldSt = "none" \/ ~HoldGatesFiles) THEN waiting' ELSE selfPage
  /\ UNCHANGED <<online, appProf, winOpen, winProf, dropSel, holdVars, ops, mainWait, notices, flap>>

\* App.update's ProfileStatus case and updateProfiles.
ApplyProfiles(m) ==
  /\ appProf' = [ok |-> TRUE, cur |-> m.cur, list |-> m.list]
  /\ winProf' = IF winOpen THEN appProf' ELSE winProf
  /\ dropSel' = IF winOpen THEN m.cur ELSE dropSel
  /\ UNCHANGED <<online, waiting, winOpen, selfPage, holdVars, ops, mainWait, noteVars, notices, flap>>

\* OnProfileSwitch's IdleAdd: skip the active profile, else switch.
ApplyTrayClick(m) ==
  /\ IF m.to = appProf.cur THEN UNCHANGED <<ops, mainWait>>
     ELSE Launch([NoOp EXCEPT !.k = "switch", !.src = "tray", !.to = m.to], TraySwitchOffMain)
  /\ UNCHANGED <<uiVars, noteVars, notices, flap>>

MainRuns ==
  /\ MainFree /\ mainQ # <<>>
  /\ mainQ' = Tail(mainQ)
  /\ LET m == Head(mainQ) IN
       CASE m.k = "ipn" -> ApplyIPN(m)
         [] m.k = "files" -> ApplyFiles(m)
         [] m.k = "profiles" -> ApplyProfiles(m)
         [] m.k = "tray" -> ApplyTrayClick(m)
         [] m.k = "notice" -> notices' = notices + 1 /\ UNCHANGED <<uiVars, ops, mainWait, noteVars, flap>>
  /\ UNCHANGED <<daemonVars, busVars, pVars, fVars,
                 unseenProf, fDirty, gen, owed, wrongDelete, failedTray, stuck, budget>>

\* ---------------- The user ----------------

\* onAppActivate when no window is open: refresh both, then showIPN(a.ipn)
\* builds the pages, and the window takes the hold's state. The window
\* shows no profiles until the next ProfileStatus. The window opens at
\* most once: closing it drops everything, so a second open goes the same
\* way as the first.
UserOpensWindow ==
  /\ MainFree /\ ~winOpen
  /\ winOpen' = TRUE /\ winProf' = NoProfiles /\ dropSel' = "none"
  /\ selfPage' = IF online THEN SelfInit ELSE NoFiles
  /\ winOff' = (heldSt # "none")
  /\ pRef' = IF WindowOpenRefreshes THEN KickP ELSE pRef
  /\ fRef' = IF WindowOpenRefreshes THEN KickF ELSE fRef
  /\ unseenProf' = FALSE
  /\ UNCHANGED <<daemonVars, busVars, pPhase, pSig, pRes,
                 fPhase, fLong, fAgain, fTimer, fSig, fState, fLast, fBase, fBaseP, fRes,
                 mainQ, mainWait, ops, online, waiting, appProf, heldSt, timerArmed, trayOff,
                 fDirty, gen, noticed, owed, told, stray, dup, wrongDelete, failedTray, notices, stuck, flap, budget>>

\* Looking at a profile list: opening the drop-down (onDropDownPopup),
\* the tray's Switch account submenu on hosts that report it
\* (OnAccountsOpen), or coming back to the window (onWindowActive).
UserOpensProfileList(where) ==
  /\ Part = "profiles" /\ MainFree /\ budget.look > 0
  /\ CASE where = "dropdown" -> winOpen
       [] where = "focus" -> winOpen
       [] where = "tray" -> appProf.ok /\ Cardinality(appProf.list) > 1
  /\ pRef' = IF \/ (where = "dropdown" /\ DropDownRefreshes)
                \/ (where = "tray" /\ TrayOpenRefreshes)
                \/ (where = "focus" /\ FocusRefreshes)
             THEN KickP ELSE pRef
  /\ unseenProf' = FALSE
  /\ Spend("look")
  /\ UNCHANGED <<daemonVars, busVars, pPhase, pSig, pRes, fVars,
                 mainQ, mainWait, ops, uiVars, fDirty, gen, noticed, owed, told, stray, dup,
                 wrongDelete, failedTray, notices, stuck, flap>>

\* The drop-down's selected-item handler.
UserPicksInDropDown(p) ==
  /\ Part = "profiles" /\ MainFree /\ winOpen /\ ~winOff /\ winProf.ok /\ budget.switch > 0
  /\ p \in winProf.list /\ p # winProf.cur
  /\ dropSel' = p
  /\ Launch([NoOp EXCEPT !.k = "switch", !.src = "dropdown", !.to = p], DropDownSwitchOffMain)
  /\ Spend("switch")
  /\ UNCHANGED <<daemonVars, busVars, pVars, fVars, mainQ,
                 online, waiting, appProf, winOpen, winProf, selfPage, holdVars, ghostVars>>

\* A tray radio item's handler runs on the D-Bus goroutine and IdleAdds.
UserPicksInTray(p) ==
  /\ Part = "profiles" /\ ~trayOff /\ appProf.ok /\ Cardinality(appProf.list) > 1 /\ budget.switch > 0
  /\ p \in appProf.list /\ p # appProf.cur
  /\ mainQ' = Append(mainQ, [Msg("tray") EXCEPT !.to = p])
  /\ Spend("switch")
  /\ UNCHANGED <<daemonVars, busVars, pVars, fVars, mainWait, ops, uiVars, ghostVars>>

\* The self page's delete confirmation.
UserDeletes(f) ==
  /\ Part = "files" /\ MainFree /\ ~winOff /\ selfPage.ok /\ f \in selfPage.fs /\ budget.del > 0
  /\ Launch([NoOp EXCEPT !.k = "delete", !.f = f, !.p = selfPage.p], DeleteOffMain)
  /\ Spend("del")
  /\ UNCHANGED <<daemonVars, busVars, pVars, fVars, mainQ, uiVars, ghostVars>>

Next ==
  \/ \E p \in Profiles : OtherClientSwitches(p) \/ OtherClientChangesProfiles(p)
                         \/ UserPicksInDropDown(p) \/ UserPicksInTray(p)
  \/ \E s \in BOOLEAN : TailscaledStarts(s)
  \/ TailscaledComesBackUp
  \/ \E f \in Files : FileArrives(f) \/ OtherClientTakesFile(f) \/ UserDeletes(f)
  \/ BusReads \/ BusPublishes
  \/ SetRunningHolds \/ SetRunningCalls \/ SetRunningReleases
  \/ ProfilesFetch \/ ProfilesPublish
  \/ FilesResets \/ FilesRefreshWakes \/ FilesRefreshDuringCheck \/ FilesTimer
  \/ FilesFetch \/ FilesPush
  \/ \E op \in ops : BackgroundFinishes(op)
  \/ MainThreadFinishes
  \/ MainRuns
  \/ HoldExpires
  \/ UserOpensWindow
  \/ \E w \in {"dropdown", "tray", "focus"} : UserOpensProfileList(w)

Spec == Init /\ [][Next]_vars

\* ---------------- Properties ----------------

TypeOK ==
  /\ cur \in Profiles /\ known \subseteq Profiles /\ cur \in known
  /\ dstate \in {"Running", "Stopped", "NoState", "Starting"}
  /\ heldSt \in {"none", "NoState", "Starting"}
  /\ hold \in {0, 1} /\ runPhase \in {"idle", "held", "called"}
  /\ pPhase \in {"deliver", "idle"} /\ fPhase \in {"off", "check", "got", "wait"}
  /\ fLast \subseteq Files

\* Files is blocked with nothing to do: offline, waiting to poll, or in a
\* check that waits for a file while none is waiting.
FilesIdle ==
  /\ fPhase \in {"off", "wait"} \/ (fPhase = "check" /\ fLong /\ files[cur] = {})
  /\ ~fRef /\ ~ResetPending

\* Nothing is in flight: every notify read and published, both loops
\* blocked with nothing pending, every callback run, and tailscaled not
\* mid-switch. Files' Interval timer can still be armed, as it is for as
\* long as files wait.
Settled ==
  /\ busQ = <<>> /\ pending = "none" /\ runPhase = "idle"
  /\ (Part = "profiles") => (pPhase = "idle" /\ ~pSig /\ ~pRef)
  /\ (Part = "files") => FilesIdle
  /\ mainQ = <<>> /\ mainWait = NoOp /\ ops = {}
  /\ dstate \in {"Running", "Stopped"} /\ ~timerArmed

ProfilesSettle ==
  (Settled /\ ~unseenProf) =>
    /\ appProf = [ok |-> TRUE, cur |-> cur, list |-> known]
    /\ winOpen => (winProf = appProf /\ dropSel = cur)

\* Once Files has read the current list and everything has landed, an open
\* window's self page lists exactly the files waiting for the current
\* profile. Files reads it again within Interval of any change, which
\* FilesWatched checks.
FilesSettle ==
  (Settled /\ ~fDirty /\ dstate = "Running" /\ winOpen) =>
    (selfPage.ok /\ selfPage.fs = files[cur])

\* Files never blocks where a change to the list could go unseen: while
\* online it either waits for a file to arrive, with none waiting, or has
\* a poll armed.
FilesWatched ==
  (Part = "files" /\ FilesIdle) =>
    /\ fPhase = "wait" => fTimer
    /\ fPhase = "off" => busLatest # "Running"

NoStrayFileNotice == ~stray

NoDuplicateFileNotice == ~dup

NoMissedFileNotice ==
  (Settled /\ ~fDirty) => \A f \in files[cur] : owed[cur][f] <= told[cur][f]

NoWrongDelete == ~wrongDelete

FailedTraySwitchNotifies == Settled => notices = failedTray

MainThreadNeverWaits == mainWait = NoOp

\* A switch that doesn't stall never shows offline, so it sends neither
\* Tailscale Status notification.
SwitchStaysConnected == ~flap

\* Once a switch has settled on Running, or stalled and the fallback has
\* fired, nothing is held, the tray and the window are enabled, and they
\* show tailscaled's state.
SwitchSettled ==
  /\ busQ = <<>> /\ pending = "none" /\ runPhase = "idle"
  /\ mainQ = <<>> /\ mainWait = NoOp /\ ~timerArmed
  /\ (dstate \in {"Running", "Stopped"} \/ stuck)

HoldEnds ==
  SwitchSettled =>
    /\ heldSt = "none" /\ ~trayOff /\ ~winOff
    /\ online = (dstate = "Running")

\* While a switch is held, the window, the tray, and the app actions show
\* or act on the old profile, so none of them take a click.
HeldStateDisabled == heldSt # "none" => (trayOff /\ (winOpen => winOff))

=============================================================================

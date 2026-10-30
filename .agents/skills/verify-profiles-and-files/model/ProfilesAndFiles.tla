-------------------------- MODULE ProfilesAndFiles --------------------------
\* Trayscale showing tailscaled's login profiles and waiting Taildrop files,
\* with Bus, Profiles, and Files from internal/tsutil and the App and window
\* code that applies what they publish.
\*
\* Trayscale paths: bus = internal/tsutil/bus.go, src = internal/tsutil/sources.go,
\* app = internal/ui/app.go, win = internal/ui/mainwindow.go,
\* self = internal/ui/selfpage.go, io = internal/ui/io.go.
\* Tailscale paths, in the module cache: ll = ipn/ipnlocal/local.go,
\* td = feature/taildrop/ext.go.
\*
\* Goroutines: Bus.Run, Profiles.Run and Files.Run (each fetchLoop), the
\* GTK main thread, and one goroutine per background switch, delete, or
\* SetRunning. gutil.IdleAdd callbacks run on the main thread in the order
\* they were added, so they are the sequence mainQ. Each LocalAPI call is
\* one step, because tailscaled serves it under b.mu.
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
  FilesOnState,            \* src: filesTriggers has changeState
  FilesOnFiles,            \* src: filesTriggers has changeFiles
  RefreshBuffered,         \* src: Refresh sends to a 1-buffered channel without blocking
  SwitchRefreshes,         \* src: Profiles.Switch defers p.Refresh()
  DeleteRefreshes,         \* src: Files.Delete defers f.Refresh()
  WindowOpenRefreshes,     \* app: onAppActivate calls a.files.Refresh() and a.profiles.Refresh()
  DropDownRefreshes,       \* win: the profile drop-down's popup calls app.profiles.Refresh
  TrayOpenRefreshes,       \* app: OnAccountsOpen calls a.profiles.Refresh()
  TraySwitchOffMain,       \* app: OnProfileSwitch calls a.profiles.Switch in a goroutine
  TraySwitchFailureNotifies, \* app: a failed tray switch sends a "Switch account" notification
  DropDownSwitchOffMain,   \* win: the drop-down's selected-item handler calls Switch in a goroutine
  DeleteOffMain,           \* self: the delete confirmation calls a.files.Delete in a goroutine
  SelfPageFillsFiles,      \* self: SelfPage.init fills fileModel from a.waiting
  FirstListQuiet           \* app: no New Incoming File notifications while a.waiting is nil

Profiles == {"A", "B"}
Files == {"f1", "f2"}

NoGen == [f \in Files |-> 0]
StartFiles == [p \in Profiles |-> IF p = "A" THEN {"f1"} ELSE {"f1", "f2"}]
NoFiles == [ok |-> FALSE, p |-> "none", fs |-> {}, gen |-> NoGen]
NoProfiles == [ok |-> FALSE, cur |-> "none", list |-> {}]
NoOp == [k |-> "none", src |-> "none", to |-> "none", f |-> "none", p |-> "none"]
Msg(k) == [k |-> k, st |-> "none", cur |-> "none", list |-> {}, p |-> "none", fs |-> {}, gen |-> NoGen, to |-> "none"]
Notify(k, st) == [k |-> k, st |-> st]

VARIABLES
  \* tailscaled
  cur,        \* the current profile
  known,      \* the profiles that exist
  dstate,     \* "Running", "Stopped", or "NoState" during a profile switch
  files,      \* waiting files per profile: td:204-223 keeps a directory per login
  busQ,       \* notifies tailscaled has sent that Bus hasn't read yet
  \* Bus.Run
  busState,   \* watchState.ipn.State, bus:357-367
  pending,    \* the status Run will publish next, bus:86, or "none"
  hold,       \* holds from SetRunning, bus:87
  runPhase,   \* the one SetRunning in flight (b.runMu): "idle", "held", "called"
  \* Profiles.Run and Files.Run (fetchLoop, src:131-191)
  pPhase, pSig, pRef, pRes,
  fPhase, fSig, fRef, fRes,
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
  \* ghosts for the properties
  unseenProf, \* a profile change nothing reports happened since the user last opened a profile list
  unseenFiles,\* a file deletion nothing reports happened since the user last opened the window
  gen,        \* ghost: per profile and file, how many times it has arrived; 0 means it was there from the start
  noticed,    \* ghost: per profile and file, the latest arrival a notification or a first list has accounted for
  owed,       \* ghost: per profile and file, the latest arrival that landed while Trayscale held that profile's list
  told,       \* ghost: per profile and file, the latest arrival a New Incoming File notification named
  stray,      \* a New Incoming File notification went out for a file that didn't just arrive
  wrongDelete,\* a delete removed a file from a different profile than the list it was picked from
  failedTray, \* tray switches that failed
  notices,    \* "Switch account" notifications shown
  budget

vars == <<cur, known, dstate, files, busQ, busState, pending, hold, runPhase,
          pPhase, pSig, pRef, pRes, fPhase, fSig, fRef, fRes,
          mainQ, mainWait, ops, online, waiting, appProf, winOpen, winProf,
          dropSel, selfPage, unseenProf, unseenFiles, gen, noticed, owed, told, stray,
          wrongDelete, failedTray, notices, budget>>
daemonVars == <<cur, known, dstate, files>>
loopVars == <<pPhase, pSig, pRef, pRes, fPhase, fSig, fRef, fRes>>
uiVars == <<online, waiting, appProf, winOpen, winProf, dropSel, selfPage>>
ghostVars == <<unseenProf, unseenFiles, gen, noticed, owed, told, stray, wrongDelete, failedTray, notices>>

\* The loop that a Part leaves out gets no signals or refreshes.
PMask == IF Part # "profiles" THEN {}
         ELSE {"connected"} \cup (IF ProfilesOnState THEN {"state"} ELSE {})
FMask == IF Part # "files" THEN {}
         ELSE {"connected"} \cup (IF FilesOnState THEN {"state"} ELSE {})
                            \cup (IF FilesOnFiles THEN {"files"} ELSE {})

\* kick, src:121-126. With an unbuffered channel the send only lands while
\* the loop is blocked in await.
Kick(ref, phase) == IF RefreshBuffered \/ phase = "idle" THEN TRUE ELSE ref
KickP == IF Part = "profiles" THEN Kick(pRef, pPhase) ELSE pRef
KickF == IF Part = "files" THEN Kick(fRef, fPhase) ELSE fRef

\* The bus's select only runs when nothing is waiting to be published,
\* bus:89-95.
BusSelects == pending = "none" \/ hold > 0

\* tailscaled loses FilesWaiting on its next notify once no file waits;
\* engine updates send one about every 2 seconds, bus:415-444.
FilesGoneNotify(fs) == IF fs = {} THEN <<Notify("files", "none")>> ELSE <<>>

\* The profiles Part leaves the self page out.
SelfInit == IF Part # "files" THEN NoFiles
            ELSE IF SelfPageFillsFiles /\ waiting.ok
            THEN [waiting EXCEPT !.ok = TRUE]
            ELSE [NoFiles EXCEPT !.ok = TRUE]   \* self:95-121 sets only incomingFiles

Init ==
  /\ cur = "A" /\ known = Profiles /\ dstate = "Running"
  /\ files = StartFiles
  \* bus:179 changeConnected, then the initial state, bus:167-172
  /\ busQ = <<Notify("connected", "none"), Notify("state", "Running")>>
  /\ busState = "NoState" /\ pending = "none" /\ hold = 0 /\ runPhase = "idle"
  \* fetchLoop fetches once at the start, src:134-138, which a pending
  \* refresh stands for.
  /\ pPhase = "idle" /\ pSig = FALSE /\ pRef = (Part = "profiles") /\ pRes = NoProfiles
  /\ fPhase = "idle" /\ fSig = FALSE /\ fRef = (Part = "files") /\ fRes = NoFiles
  /\ mainQ = <<>> /\ mainWait = NoOp /\ ops = {}
  /\ online = FALSE /\ waiting = NoFiles /\ appProf = NoProfiles
  /\ winOpen = FALSE /\ winProf = NoProfiles /\ dropSel = "none" /\ selfPage = NoFiles
  /\ unseenProf = FALSE /\ unseenFiles = FALSE
  /\ gen = [p \in Profiles |-> NoGen] /\ noticed = [p \in Profiles |-> NoGen]
  /\ owed = [p \in Profiles |-> NoGen] /\ told = [p \in Profiles |-> NoGen]
  /\ stray = FALSE /\ wrongDelete = FALSE /\ failedTray = 0 /\ notices = 0
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
  IF SwitchFails(p) \/ p = cur THEN UNCHANGED <<cur, dstate, busQ>>
  ELSE /\ cur' = p /\ dstate' = "NoState"
       /\ busQ' = busQ \o (IF dstate # "NoState" THEN <<Notify("state", "NoState")>> ELSE <<>>)

\* `tailscale switch` from the CLI or another client.
OtherClientSwitches(p) ==
  /\ budget.switch > 0 /\ p # cur /\ p \in known
  /\ DoSwitch(p) /\ Spend("switch")
  /\ UNCHANGED <<known, files, busState, pending, hold, runPhase, loopVars, mainQ, mainWait, ops, uiVars, ghostVars>>

\* startLocked after a switch.
TailscaledComesBackUp ==
  /\ dstate = "NoState" /\ dstate' = "Running"
  /\ busQ' = Append(busQ, Notify("state", "Running"))
  /\ UNCHANGED <<cur, known, files, busState, pending, hold, runPhase, loopVars, mainQ, mainWait, ops, uiVars, ghostVars, budget>>

\* A Taildrop transfer finishes: IncomingFiles with Done, bus:421-437.
FileArrives(f) ==
  /\ Part = "files" /\ budget.recv > 0 /\ dstate = "Running" /\ f \notin files[cur]
  /\ Resends \/ (f \notin StartFiles[cur] /\ gen[cur][f] = 0)
  /\ files' = [files EXCEPT ![cur] = @ \cup {f}]
  /\ busQ' = Append(busQ, Notify("files", "none"))
  /\ gen' = [gen EXCEPT ![cur][f] = @ + 1]
  \* A file that lands before Trayscale holds a list for this profile is
  \* one the first list takes quietly, by design.
  /\ owed' = IF waiting.ok /\ waiting.p = cur THEN [owed EXCEPT ![cur][f] = gen'[cur][f]] ELSE owed
  /\ Spend("recv")
  /\ UNCHANGED <<cur, known, dstate, busState, pending, hold, runPhase, loopVars, mainQ, mainWait, ops, uiVars,
                 unseenProf, unseenFiles, noticed, told, stray, wrongDelete, failedTray, notices>>

\* `tailscale file get` or another client takes a waiting file. Nothing
\* reports it unless it was the last one.
OtherClientTakesFile(f) ==
  /\ Part = "files" /\ budget.odel > 0 /\ dstate = "Running" /\ f \in files[cur]
  /\ files' = [files EXCEPT ![cur] = @ \ {f}]
  /\ busQ' = busQ \o FilesGoneNotify(files'[cur])
  /\ unseenFiles' = (unseenFiles \/ files'[cur] # {})
  /\ Spend("odel")
  /\ UNCHANGED <<cur, known, dstate, busState, pending, hold, runPhase, loopVars, mainQ, mainWait, ops, uiVars,
                 unseenProf, gen, noticed, owed, told, stray, wrongDelete, failedTray, notices>>

\* Another client adds or deletes a profile that isn't the current one.
\* Nothing reports it, src:67-71.
OtherClientChangesProfiles(q) ==
  /\ Part = "profiles" /\ budget.list > 0 /\ q # cur
  /\ known' = IF q \in known THEN known \ {q} ELSE known \cup {q}
  /\ unseenProf' = TRUE
  /\ Spend("list")
  /\ UNCHANGED <<cur, dstate, files, busQ, busState, pending, hold, runPhase, loopVars, mainQ, mainWait, ops, uiVars,
                 unseenFiles, gen, noticed, owed, told, stray, wrongDelete, failedTray, notices>>

\* ---------------- Bus.Run, bus:76-118 ----------------

\* One notify: watchState.apply (bus:354-410), then Run sets pending and
\* fires the changes before it publishes, bus:98-109.
BusReads ==
  /\ busQ # <<>> /\ BusSelects
  /\ LET n == Head(busQ)
         changes == CASE n.k = "connected" -> {"connected"}
                      [] n.k = "state" -> (IF n.st # busState THEN {"state"} ELSE {})
                      [] n.k = "files" -> {"files"}
     IN /\ busQ' = Tail(busQ)
        /\ busState' = IF n.k = "state" THEN n.st ELSE busState
        /\ pending' = IF n.k = "state" THEN n.st ELSE pending
        /\ pSig' = (pSig \/ changes \cap PMask # {})
        /\ fSig' = (fSig \/ changes \cap FMask # {})
  /\ UNCHANGED <<daemonVars, hold, runPhase, pPhase, pRef, pRes, fPhase, fRef, fRes,
                 mainQ, mainWait, ops, uiVars, ghostVars, budget>>

\* publish, bus:89-92 and 120-127: Bus.New does gutil.IdleAdd(a.update), app:695-701.
BusPublishes ==
  /\ pending # "none" /\ hold = 0
  /\ mainQ' = Append(mainQ, [Msg("ipn") EXCEPT !.st = pending])
  /\ pending' = "none"
  /\ UNCHANGED <<daemonVars, busQ, busState, hold, runPhase, loopVars, mainWait, ops, uiVars, ghostVars, budget>>

\* SetRunning, bus:261-290, from the window's status switch or the tray's
\* connection toggle, both in goroutines. It holds publishing, but not
\* signals, while `tailscale up` or `down` runs.
SetRunningHolds ==
  /\ budget.toggle > 0 /\ runPhase = "idle" /\ dstate # "NoState" /\ BusSelects
  /\ hold' = 1 /\ runPhase' = "held" /\ Spend("toggle")
  /\ UNCHANGED <<daemonVars, busQ, busState, pending, loopVars, mainQ, mainWait, ops, uiVars, ghostVars>>

SetRunningCalls ==
  /\ runPhase = "held" /\ runPhase' = "called"
  /\ CASE dstate = "Running" -> dstate' = "Stopped" /\ busQ' = Append(busQ, Notify("state", "Stopped"))
       [] dstate = "Stopped" -> dstate' = "Running" /\ busQ' = Append(busQ, Notify("state", "Running"))
       [] dstate = "NoState" -> UNCHANGED <<dstate, busQ>>
  /\ UNCHANGED <<cur, known, files, busState, pending, hold, loopVars, mainQ, mainWait, ops, uiVars, ghostVars, budget>>

SetRunningReleases ==
  /\ runPhase = "called" /\ BusSelects
  /\ hold' = 0 /\ runPhase' = "idle"
  /\ UNCHANGED <<daemonVars, busQ, busState, pending, loopVars, mainQ, mainWait, ops, uiVars, ghostVars, budget>>

\* ---------------- Profiles.Run and Files.Run, src:131-191 ----------------

\* await returns on a signal in mask or a refresh and drains the rest
\* (src:165-191), then getProfileStatus is one LocalAPI call. A change
\* between the drain and the call fires a signal that the next await sees,
\* so taking both as one step only drops a fetch that repeats this one.
ProfilesFetch ==
  /\ Part = "profiles" /\ pPhase = "idle" /\ (pSig \/ pRef)
  /\ pPhase' = "deliver" /\ pSig' = FALSE /\ pRef' = FALSE
  /\ pRes' = [ok |-> TRUE, cur |-> cur, list |-> known]
  /\ UNCHANGED <<daemonVars, busQ, busState, pending, hold, runPhase, fPhase, fSig, fRef, fRes,
                 mainQ, mainWait, ops, uiVars, ghostVars, budget>>

\* p.New, src:102, does gutil.IdleAdd(a.update).
ProfilesPublish ==
  /\ pPhase = "deliver" /\ pPhase' = "idle"
  /\ mainQ' = Append(mainQ, [Msg("profiles") EXCEPT !.cur = pRes.cur, !.list = pRes.list])
  /\ UNCHANGED <<daemonVars, busQ, busState, pending, hold, runPhase, pSig, pRef, pRes, fPhase, fSig, fRef, fRes,
                 mainWait, ops, uiVars, ghostVars, budget>>

\* Like ProfilesFetch: await, then waitingFiles against the current
\* login's manager.
FilesFetch ==
  /\ Part = "files" /\ fPhase = "idle" /\ (fSig \/ fRef)
  /\ fPhase' = "deliver" /\ fSig' = FALSE /\ fRef' = FALSE
  /\ fRes' = [ok |-> TRUE, p |-> cur, fs |-> files[cur],
              gen |-> [f \in Files |-> IF f \in files[cur] THEN gen[cur][f] ELSE 0]]
  /\ UNCHANGED <<daemonVars, busQ, busState, pending, hold, runPhase, pPhase, pSig, pRef, pRes,
                 mainQ, mainWait, ops, uiVars, ghostVars, budget>>

\* f.New, src:49.
FilesPublish ==
  /\ fPhase = "deliver" /\ fPhase' = "idle"
  /\ mainQ' = Append(mainQ, [Msg("files") EXCEPT !.p = fRes.p, !.fs = fRes.fs, !.gen = fRes.gen])
  /\ UNCHANGED <<daemonVars, busQ, busState, pending, hold, runPhase, pPhase, pSig, pRef, pRes, fSig, fRef, fRes,
                 mainWait, ops, uiVars, ghostVars, budget>>

\* ---------------- Switches and deletes ----------------

\* Profiles.Switch, src:116-119, from the tray (app:610-628) or the
\* drop-down (win:121-157), and Files.Delete, src:62-65, from the self
\* page (self:305-318) or saveFile (io:73). Each is one step: the LocalAPI
\* call, then the deferred Refresh, then for the tray the failure
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
    /\ busQ' = busQ \o (IF op.f \in files[cur] THEN FilesGoneNotify(files'[cur]) ELSE <<>>)
    /\ fRef' = IF DeleteRefreshes THEN KickF ELSE fRef
    /\ UNCHANGED <<cur, known, dstate, pRef, failedTray, mainQ>>

BackgroundFinishes(op) ==
  /\ op \in ops /\ ops' = ops \ {op}
  /\ Finish(op)
  /\ UNCHANGED <<busState, pending, hold, runPhase, pPhase, pSig, pRes, fPhase, fSig, fRes, mainWait, uiVars,
                 unseenProf, unseenFiles, gen, noticed, owed, told, stray, notices, budget>>

MainThreadFinishes ==
  /\ mainWait # NoOp /\ mainWait' = NoOp
  /\ Finish(mainWait)
  /\ UNCHANGED <<busState, pending, hold, runPhase, pPhase, pSig, pRes, fPhase, fSig, fRes, ops, uiVars,
                 unseenProf, unseenFiles, gen, noticed, owed, told, stray, notices, budget>>

\* Start op on the main thread or in a goroutine.
Launch(op, offMain) ==
  IF offMain THEN ops' = ops \cup {op} /\ UNCHANGED mainWait
  ELSE mainWait' = op /\ UNCHANGED ops

\* ---------------- The main thread ----------------

MainFree == mainWait = NoOp

\* App.update, app:118-200, and MainWindow.Update, win:271-292.
ApplyIPN(m) ==
  LET on == m.st = "Running" IN
  /\ online' = on
  /\ waiting' = IF on THEN waiting ELSE NoFiles                     \* app:149-151
  \* updatePeers: SelfPage.UpdateIPN returns false offline and the page
  \* goes (self:360-363, win:385-397); online, ensureSectionPages adds it
  \* (win:375-383).
  /\ selfPage' = IF ~winOpen THEN NoFiles
                 ELSE IF ~on THEN NoFiles
                 ELSE IF selfPage.ok THEN selfPage
                 ELSE SelfInit
  /\ UNCHANGED <<appProf, winOpen, winProf, dropSel, ops, mainWait, stray, noticed, told, notices>>

\* app:166-188: a file in the new list that isn't in a.waiting gets a
\* notification, unless a.waiting is nil.
ApplyFiles(m) ==
  LET quiet == ~waiting.ok /\ FirstListQuiet
      new == IF quiet THEN {} ELSE IF waiting.ok THEN m.fs \ waiting.fs ELSE m.fs
      \* A list taken quietly accounts for the arrival of each file in it.
      seen == IF quiet THEN m.fs ELSE new
  IN
  /\ stray' = (stray \/ \E f \in new : m.gen[f] = 0 \/ m.gen[f] <= noticed[m.p][f])
  /\ noticed' = [noticed EXCEPT ![m.p] = [f \in Files |->
                   IF f \in seen /\ m.gen[f] > @[f] THEN m.gen[f] ELSE @[f]]]
  /\ told' = [told EXCEPT ![m.p] = [f \in Files |-> IF f \in new THEN m.gen[f] ELSE @[f]]]
  /\ waiting' = [ok |-> TRUE, p |-> m.p, fs |-> m.fs, gen |-> m.gen]
  /\ selfPage' = IF selfPage.ok THEN waiting' ELSE selfPage   \* self:398-403
  /\ UNCHANGED <<online, appProf, winOpen, winProf, dropSel, ops, mainWait, notices>>

\* app:189-200 and updateProfiles, win:605-630.
ApplyProfiles(m) ==
  /\ appProf' = [ok |-> TRUE, cur |-> m.cur, list |-> m.list]
  /\ winProf' = IF winOpen THEN appProf' ELSE winProf
  /\ dropSel' = IF winOpen THEN m.cur ELSE dropSel
  /\ UNCHANGED <<online, waiting, winOpen, selfPage, ops, mainWait, stray, noticed, told, notices>>

\* OnProfileSwitch's IdleAdd, app:610-628: skip the active profile, else
\* switch.
ApplyTrayClick(m) ==
  /\ IF m.to = appProf.cur THEN UNCHANGED <<ops, mainWait>>
     ELSE Launch([NoOp EXCEPT !.k = "switch", !.src = "tray", !.to = m.to], TraySwitchOffMain)
  /\ UNCHANGED <<uiVars, stray, noticed, told, notices>>

MainRuns ==
  /\ MainFree /\ mainQ # <<>>
  /\ mainQ' = Tail(mainQ)
  /\ LET m == Head(mainQ) IN
       CASE m.k = "ipn" -> ApplyIPN(m)
         [] m.k = "files" -> ApplyFiles(m)
         [] m.k = "profiles" -> ApplyProfiles(m)
         [] m.k = "tray" -> ApplyTrayClick(m)
         [] m.k = "notice" -> notices' = notices + 1 /\ UNCHANGED <<uiVars, ops, mainWait, stray, noticed, told>>
  /\ UNCHANGED <<daemonVars, busQ, busState, pending, hold, runPhase, loopVars,
                 unseenProf, unseenFiles, gen, owed, wrongDelete, failedTray, budget>>

\* ---------------- The user ----------------

\* onAppActivate, app:410-486, when no window is open: refresh both, then
\* a.update(a.ipn) builds the pages. The window shows no profiles until
\* the next ProfileStatus. The window opens at most once: closing it drops
\* everything, so a second open goes the same way as the first.
UserOpensWindow ==
  /\ MainFree /\ ~winOpen
  /\ winOpen' = TRUE /\ winProf' = NoProfiles /\ dropSel' = "none"
  /\ selfPage' = IF online THEN SelfInit ELSE NoFiles
  /\ pRef' = IF WindowOpenRefreshes THEN KickP ELSE pRef
  /\ fRef' = IF WindowOpenRefreshes THEN KickF ELSE fRef
  /\ unseenProf' = FALSE /\ unseenFiles' = FALSE
  /\ UNCHANGED <<daemonVars, busQ, busState, pending, hold, runPhase, pPhase, pSig, pRes, fPhase, fSig, fRes,
                 mainQ, mainWait, ops, online, waiting, appProf, gen, noticed, owed, told, stray, wrongDelete, failedTray, notices, budget>>

\* win:160 and app:631: opening the drop-down or the tray's Switch account
\* submenu, on hosts that report it.
UserOpensProfileList(where) ==
  /\ Part = "profiles" /\ MainFree /\ budget.look > 0
  /\ IF where = "dropdown" THEN winOpen ELSE appProf.ok /\ Cardinality(appProf.list) > 1
  /\ pRef' = IF (where = "dropdown" /\ DropDownRefreshes) \/ (where = "tray" /\ TrayOpenRefreshes)
             THEN KickP ELSE pRef
  /\ unseenProf' = FALSE
  /\ Spend("look")
  /\ UNCHANGED <<daemonVars, busQ, busState, pending, hold, runPhase, pPhase, pSig, pRes, fPhase, fSig, fRef, fRes,
                 mainQ, mainWait, ops, uiVars, unseenFiles, gen, noticed, owed, told, stray, wrongDelete, failedTray, notices>>

\* The drop-down's selected-item handler, win:121-157.
UserPicksInDropDown(p) ==
  /\ Part = "profiles" /\ MainFree /\ winOpen /\ winProf.ok /\ budget.switch > 0
  /\ p \in winProf.list /\ p # winProf.cur
  /\ dropSel' = p
  /\ Launch([NoOp EXCEPT !.k = "switch", !.src = "dropdown", !.to = p], DropDownSwitchOffMain)
  /\ Spend("switch")
  /\ UNCHANGED <<daemonVars, busQ, busState, pending, hold, runPhase, loopVars, mainQ,
                 online, waiting, appProf, winOpen, winProf, selfPage, ghostVars>>

\* A tray radio item's handler runs on the D-Bus goroutine and IdleAdds,
\* app:610-611.
UserPicksInTray(p) ==
  /\ Part = "profiles" /\ appProf.ok /\ Cardinality(appProf.list) > 1 /\ budget.switch > 0
  /\ p \in appProf.list /\ p # appProf.cur
  /\ mainQ' = Append(mainQ, [Msg("tray") EXCEPT !.to = p])
  /\ Spend("switch")
  /\ UNCHANGED <<daemonVars, busQ, busState, pending, hold, runPhase, loopVars, mainWait, ops, uiVars, ghostVars>>

\* The self page's delete confirmation, self:305-318.
UserDeletes(f) ==
  /\ Part = "files" /\ MainFree /\ selfPage.ok /\ f \in selfPage.fs /\ budget.del > 0
  /\ Launch([NoOp EXCEPT !.k = "delete", !.f = f, !.p = selfPage.p], DeleteOffMain)
  /\ Spend("del")
  /\ UNCHANGED <<daemonVars, busQ, busState, pending, hold, runPhase, loopVars, mainQ, uiVars, ghostVars>>

Next ==
  \/ \E p \in Profiles : OtherClientSwitches(p) \/ OtherClientChangesProfiles(p)
                         \/ UserPicksInDropDown(p) \/ UserPicksInTray(p)
  \/ TailscaledComesBackUp
  \/ \E f \in Files : FileArrives(f) \/ OtherClientTakesFile(f) \/ UserDeletes(f)
  \/ BusReads \/ BusPublishes
  \/ SetRunningHolds \/ SetRunningCalls \/ SetRunningReleases
  \/ ProfilesFetch \/ ProfilesPublish
  \/ FilesFetch \/ FilesPublish
  \/ \E op \in ops : BackgroundFinishes(op)
  \/ MainThreadFinishes
  \/ MainRuns
  \/ UserOpensWindow
  \/ \E w \in {"dropdown", "tray"} : UserOpensProfileList(w)

Spec == Init /\ [][Next]_vars

\* ---------------- Properties ----------------

TypeOK ==
  /\ cur \in Profiles /\ known \subseteq Profiles /\ cur \in known
  /\ dstate \in {"Running", "Stopped", "NoState"}
  /\ hold \in {0, 1} /\ runPhase \in {"idle", "held", "called"}
  /\ pPhase \in {"deliver", "idle"} /\ fPhase \in {"deliver", "idle"}

\* Nothing is in flight: every notify read and published, both loops
\* waiting with nothing pending, every callback run, and tailscaled not
\* mid-switch.
Settled ==
  /\ busQ = <<>> /\ pending = "none" /\ runPhase = "idle"
  /\ (Part = "profiles") => (pPhase = "idle" /\ ~pSig /\ ~pRef)
  /\ (Part = "files") => (fPhase = "idle" /\ ~fSig /\ ~fRef)
  /\ mainQ = <<>> /\ mainWait = NoOp /\ ops = {}
  /\ dstate # "NoState"

ProfilesSettle ==
  (Settled /\ ~unseenProf) =>
    /\ appProf = [ok |-> TRUE, cur |-> cur, list |-> known]
    /\ winOpen => (winProf = appProf /\ dropSel = cur)

FilesSettle ==
  (Settled /\ ~unseenFiles /\ dstate = "Running" /\ winOpen) =>
    (selfPage.ok /\ selfPage.fs = files[cur])

NoStrayFileNotice == ~stray

NoMissedFileNotice ==
  Settled => \A f \in files[cur] : owed[cur][f] <= told[cur][f]

NoWrongDelete == ~wrongDelete

FailedTraySwitchNotifies == Settled => notices = failedTray

MainThreadNeverWaits == mainWait = NoOp

=============================================================================

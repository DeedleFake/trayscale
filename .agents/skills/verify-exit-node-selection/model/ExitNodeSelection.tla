------------------------- MODULE ExitNodeSelection -------------------------
\* Turning exit node use on and off from Trayscale with the exit-node-selection
\* setting, against tailscaled at the tailscale.com version pinned in
\* scripts/check-exit-node-selection.
\*
\* Trayscale paths: tc = internal/tsutil/client.go, app = internal/ui/app.go,
\* set = internal/ui/settings.go, peer = internal/ui/peerpage.go,
\* mv = internal/ui/mullvadpage.go, self = internal/ui/selfpage.go,
\* poll = internal/tsutil/poller.go.
\* Tailscale paths, in the module cache: ll = ipn/ipnlocal/local.go,
\* api = ipn/localapi/localapi.go.
\*
\* Each LocalAPI call is one step, because tailscaled serves it under b.mu.
\* The constants other than the bounds are code facts that the check script
\* reads from the code. TRUE means the code has the guard.
EXTENDS Naturals

CONSTANTS
  MaxClicks, MaxOther, MaxNetmap, MaxSetting,
  OffWhileWaiting,          \* bound, not a fact: allow turning exit node use off while Recommended has no node
  SettingReadBack,          \* set: the setting round-trips through the "exit-node-selection" key
  MenuUsesSetting,          \* app: the use_exit_node action passes a.exitNodeSelection()
  TrayUsesSetting,          \* app: OnExitToggle passes a.exitNodeSelection()
  RecommendedUsesAuto,      \* tc: SetUseExitNode calls useAutoExitNode for Recommended
  PreviousPinsRestoredAuto, \* tc: SetUseExitNode ends the Previous path with keepPreviousExitNode
  OffPinsAutoNode,          \* tc: DisableExitNode calls pinAutoExitNode first
  SuggestRejectsEmpty,      \* tc: suggestExitNode fails on an empty suggestion ID
  KeepUndoesFailedPin,      \* tc: keepPreviousExitNode turns exit node use back off when it has nothing to pin
  PeerPageOffKeepsPrior,    \* peer: the Use as exit node switch turns off with DisableExitNode
  MullvadPageOffKeepsPrior, \* mv: a Mullvad node's switch turns off with DisableExitNode
  AdvertiseKeepsPrior       \* self: Advertise exit node turns exit node use off with DisableExitNode

Nodes == {"X", "Y"}
Pages == {"peer", "mullvad", "self"}

VARIABLES
  auto,       \* prefs.AutoExitNode is set ("any")
  eid,        \* prefs.ExitNodeID: "", a node, or "U" for unresolvedExitNodeID "auto:any" (ll:9177)
  prior,      \* prefs.InternalExitNodePrior: "", a node, or "A" for "auto:any"
  sugg,       \* b.lastSuggestedExitNode, "none" when zero
  setting,    \* the exit-node-selection GSettings key
  lastUsed,   \* ghost: the last node this device used as its exit node
  req,        \* the latest toggle from the menu or the tray
  clicks,     \* toggles so far
  otherSince, \* ghost: something other than a toggle changed exit node prefs since the last toggle
  budget      \* bounds on page actions, netmaps, and setting changes

vars == <<auto, eid, prior, sugg, setting, lastUsed, req, clicks, otherSince, budget>>

Used(e) == IF e \in Nodes THEN e ELSE lastUsed

\* resolveAutoExitNodeLocked, ll:2831-2870
Resolve(e) ==
  IF sugg # "none" THEN sugg                      \* ll:2852-2854
  ELSE IF e \in Nodes THEN e                      \* ll:2855-2858 isAllowedAutoExitNodeID
  ELSE "U"                                        \* ll:2859-2864

Init ==
  /\ auto = FALSE /\ eid = "" /\ prior = ""
  /\ sugg = "none"
  /\ setting \in {"previous", "recommended"}
  /\ lastUsed = "none"
  /\ req = [use |-> FALSE, want |-> "previous", pc |-> "done", id |-> "",
            prev |-> "none", failed |-> FALSE]
  /\ clicks = 0
  /\ otherSince = FALSE
  /\ budget = [other |-> MaxOther, netmap |-> MaxNetmap, setting |-> MaxSetting]

Go(pc) == req' = [req EXCEPT !.pc = pc]
GoId(pc, id) == req' = [req EXCEPT !.pc = pc, !.id = id]
Fail == req' = [req EXCEPT !.pc = "done", !.failed = TRUE]

\* SetUseExitNodeEnabled(false), ll:5137-5184
DisableInTailscale ==
  IF eid = "" THEN UNCHANGED <<auto, eid, prior>>               \* ll:5149-5152
  ELSE /\ prior' = IF auto THEN "A" ELSE eid                    \* ll:5176-5181
       /\ eid' = "" /\ auto' = FALSE

\* ---------------- The user and the network ----------------

\* The menu's use_exit_node action (app:408-420) or the tray's OnExitToggle
\* (app:548-570) asks for the opposite of ExitNodeActive (poll:595-597).
\* want is the setting the user chose, sel what the code passes on.
Click ==
  /\ clicks < MaxClicks
  /\ req.pc = "done"
  /\ OffWhileWaiting \/ eid # "U"
  /\ \E entry \in {"menu", "tray"} :
       LET on == eid = ""
           follows == SettingReadBack /\ IF entry = "menu" THEN MenuUsesSetting ELSE TrayUsesSetting
       IN \E sel \in IF follows THEN {setting} ELSE {"previous", "recommended"} :
            req' = [use |-> on, want |-> setting,
                    pc |-> IF ~on THEN "oRead"
                           ELSE IF sel = "recommended" /\ RecommendedUsesAuto THEN "rAuto"
                           ELSE "pEnable",
                    id |-> "", prev |-> lastUsed, failed |-> FALSE]
  /\ clicks' = clicks + 1
  /\ otherSince' = FALSE
  /\ UNCHANGED <<auto, eid, prior, sugg, setting, lastUsed, budget>>

\* The Preferences combo row writes the key (set:145-149).
ChangeSetting ==
  /\ budget.setting > 0
  /\ setting' = IF setting = "previous" THEN "recommended" ELSE "previous"
  /\ budget' = [budget EXCEPT !.setting = @ - 1]
  /\ UNCHANGED <<auto, eid, prior, sugg, lastUsed, req, clicks, otherSince>>

\* A peer or Mullvad page's switch turned on: ExitNode(n) (tc:59-89) with
\* AutoExitNodeSet, so the prior stays. adjustEditPrefsLocked empties the
\* pick while the daemon still holds the unresolved placeholder (ll:5327-5331).
PickOnPage ==
  /\ budget.other > 0
  /\ \E n \in Nodes :
       IF eid = "U"
         THEN eid' = "" /\ UNCHANGED lastUsed
         ELSE eid' = n /\ lastUsed' = n
  /\ auto' = FALSE
  /\ budget' = [budget EXCEPT !.other = @ - 1]
  /\ otherSince' = TRUE
  /\ UNCHANGED <<prior, sugg, setting, req, clicks>>

\* A peer or Mullvad page's switch turned off (peer:147-172, mv:385-409), only
\* shown on for the node in use, or Advertise exit node turned on (self:130-138).
\* With the fact, the page calls DisableExitNode: pinAutoExitNode then
\* SetUseExitNodeEnabled(false), composed into one step because the main
\* thread runs the handler and auto is off between the two calls. Without it,
\* the page calls ExitNode(""), and an empty ExitNodeID makes
\* adjustEditPrefsLocked zero the prior (ll:5321-5325) even when no exit node
\* was in use.
OffOnPage ==
  /\ budget.other > 0
  /\ OffWhileWaiting \/ eid # "U"
  /\ \E page \in Pages :
       /\ page # "self" => eid \in Nodes
       /\ IF CASE page = "peer" -> PeerPageOffKeepsPrior
               [] page = "mullvad" -> MullvadPageOffKeepsPrior
               [] page = "self" -> AdvertiseKeepsPrior
          THEN IF eid = "" THEN UNCHANGED <<auto, eid, prior>>
               ELSE /\ prior' = IF auto /\ eid \in Nodes /\ OffPinsAutoNode THEN eid
                                ELSE IF auto THEN "A" ELSE eid
                    /\ eid' = "" /\ auto' = FALSE
          ELSE eid' = "" /\ auto' = FALSE /\ prior' = ""
  /\ budget' = [budget EXCEPT !.other = @ - 1]
  /\ otherSince' = TRUE
  /\ UNCHANGED <<sugg, setting, lastUsed, req, clicks>>

\* A netmap brings a new suggestion: suggestExitNodeLocked, then
\* refreshExitNodeLocked under the same lock (ll:2130-2140).
\* Without OffWhileWaiting, no suggestion lands between a toggle's decision
\* not to pin and its disable, since that is the same turning off while
\* Recommended has no node.
NewSuggestion ==
  /\ budget.netmap > 0
  /\ OffWhileWaiting \/ ~(req.pc = "oOff" /\ auto)
  /\ \E n \in Nodes \ {sugg} :
       /\ sugg' = n
       /\ LET e2 == IF auto THEN n ELSE eid IN
            /\ eid' = e2
            /\ lastUsed' = Used(e2)
  /\ budget' = [budget EXCEPT !.netmap = @ - 1]
  /\ UNCHANGED <<auto, prior, setting, req, clicks, otherSince>>

\* ---------------- One toggle, one LocalAPI call per step ----------------

\* Recommended: useAutoExitNode (tc:190-199), then setPrefsLocked resolves it.
UseAutoExitNode ==
  /\ req.pc = "rAuto"
  /\ auto' = TRUE
  /\ eid' = Resolve(eid)
  /\ lastUsed' = Used(Resolve(eid))
  /\ Go("done")
  /\ UNCHANGED <<prior, sugg, setting, otherSince, budget, clicks>>

\* Previous: localClient.SetUseExitNode(true) (tc:137) -> SetUseExitNodeEnabled.
EnableExitNode ==
  /\ req.pc = "pEnable"
  /\ LET next == IF PreviousPinsRestoredAuto THEN "kRead" ELSE "done" IN
     IF eid # "" THEN                                           \* ll:5145-5148 already on
        /\ Go(next)
        /\ UNCHANGED <<auto, eid, lastUsed>>
     ELSE IF prior = "" THEN                                    \* ll:5155-5160 error
        /\ Go("fSuggest")                                       \* tc:138-143
        /\ UNCHANGED <<auto, eid, lastUsed>>
     ELSE IF prior = "A" THEN                                   \* ll:5163-5170
        /\ auto' = TRUE
        /\ eid' = Resolve("U")
        /\ lastUsed' = Used(Resolve("U"))
        /\ Go(next)
     ELSE                                                       \* ll:5164-5165, adjust ll:5333-5338
        /\ auto' = FALSE /\ eid' = prior
        /\ lastUsed' = prior
        /\ Go(next)
  /\ UNCHANGED <<prior, sugg, setting, otherSince, budget, clicks>>

\* useSuggestedExitNode -> suggestExitNode (tc:206-224). tailscaled answers an
\* empty ID, not an error, when it has nothing to recommend (api:1826-1881).
FallbackSuggest ==
  /\ req.pc = "fSuggest"
  /\ IF sugg # "none" THEN GoId("fPin", sugg)
     ELSE IF SuggestRejectsEmpty THEN Fail
     ELSE GoId("fPin", "")
  /\ UNCHANGED <<auto, eid, prior, sugg, setting, lastUsed, otherSince, budget, clicks>>

\* ExitNode(id) (tc:59-89). An empty ID clears the exit node, and
\* adjustEditPrefsLocked zeroes the prior with it (ll:5321-5325).
FallbackPin ==
  /\ req.pc = "fPin"
  /\ eid' = (IF eid = "U" THEN "" ELSE req.id) /\ auto' = FALSE \* ll:5327-5331
  /\ prior' = IF req.id = "" THEN "" ELSE prior
  /\ lastUsed' = Used(IF eid = "U" THEN "" ELSE req.id)
  /\ Go("done")
  /\ UNCHANGED <<sugg, setting, otherSince, budget, clicks>>

\* keepPreviousExitNode reads prefs (tc:231-234).
KeepReadPrefs ==
  /\ req.pc = "kRead"
  /\ IF ~auto THEN Go("done")                                   \* tc:235-237
     ELSE IF eid \in {"", "U"} THEN Go("kSuggest")              \* tc:241
     ELSE GoId("kPin", eid)
  /\ UNCHANGED <<auto, eid, prior, sugg, setting, lastUsed, otherSince, budget, clicks>>

\* suggestExitNode inside keepPreviousExitNode (tc:242-247).
KeepSuggest ==
  /\ req.pc = "kSuggest"
  /\ IF sugg # "none" THEN GoId("kPin", sugg)
     ELSE IF ~SuggestRejectsEmpty THEN GoId("kPin", "")
     ELSE IF KeepUndoesFailedPin THEN Go("kUndo")
     ELSE Fail
  /\ UNCHANGED <<auto, eid, prior, sugg, setting, lastUsed, otherSince, budget, clicks>>

\* The pin's EditPrefs (tc:250-255). adjustEditPrefsLocked empties it while
\* the daemon still holds the placeholder (ll:5327-5331), and an empty
\* ExitNodeID zeroes the prior (ll:5321-5325).
KeepPin ==
  /\ req.pc = "kPin"
  /\ LET e2 == IF eid = "U" THEN "" ELSE req.id IN
       /\ eid' = e2
       /\ lastUsed' = Used(e2)
  /\ prior' = IF req.id = "" THEN "" ELSE prior
  /\ auto' = FALSE
  /\ Go("done")
  /\ UNCHANGED <<sugg, setting, otherSince, budget, clicks>>

\* localClient.SetUseExitNode(false) after a failed pin (tc:246).
KeepUndo ==
  /\ req.pc = "kUndo"
  /\ DisableInTailscale
  /\ Fail
  /\ UNCHANGED <<sugg, setting, lastUsed, otherSince, budget, clicks>>

\* Off: DisableExitNode (tc:152-157) -> pinAutoExitNode reads prefs (tc:164-176).
OffReadPrefs ==
  /\ req.pc = "oRead"
  /\ IF OffPinsAutoNode /\ auto /\ eid \in Nodes
       THEN GoId("oPin", eid)
       ELSE Go("oOff")
  /\ UNCHANGED <<auto, eid, prior, sugg, setting, lastUsed, otherSince, budget, clicks>>

\* pinAutoExitNode's EditPrefs (tc:178-186).
OffPin ==
  /\ req.pc = "oPin"
  /\ LET e2 == IF eid = "U" THEN "" ELSE req.id IN              \* ll:5327-5331
       /\ eid' = e2
       /\ lastUsed' = Used(e2)
  /\ auto' = FALSE
  /\ Go("oOff")
  /\ UNCHANGED <<prior, sugg, setting, otherSince, budget, clicks>>

\* localClient.SetUseExitNode(false) (tc:156).
OffDisable ==
  /\ req.pc = "oOff"
  /\ DisableInTailscale
  /\ Go("done")
  /\ UNCHANGED <<sugg, setting, lastUsed, otherSince, budget, clicks>>

Toggle ==
    \/ UseAutoExitNode \/ EnableExitNode \/ FallbackSuggest \/ FallbackPin
    \/ KeepReadPrefs \/ KeepSuggest \/ KeepPin \/ KeepUndo
    \/ OffReadPrefs \/ OffPin \/ OffDisable

User == Click \/ ChangeSetting \/ PickOnPage \/ OffOnPage
Network == NewSuggestion

Next == Toggle \/ User \/ Network
Spec == Init /\ [][Next]_vars

\* ---------------- Properties ----------------

Settled == ~ENABLED Toggle
\* The last toggle turned exit node use on, has finished, and nothing else
\* has changed exit node prefs since.
TurnedOn == Settled /\ clicks > 0 /\ req.use /\ ~otherSince
Succeeded == TurnedOn /\ ~req.failed

TypeOK ==
  /\ auto \in BOOLEAN /\ eid \in {"", "U"} \cup Nodes
  /\ prior \in {"", "A"} \cup Nodes /\ sugg \in {"none"} \cup Nodes
  /\ setting \in {"previous", "recommended"}

\* The selection type chosen in the setting is always the one that takes effect.
SelectionTakesEffect ==
  Succeeded => /\ (req.want = "recommended" => auto)
               /\ (req.want = "previous" => ~auto)

\* Previous gets back the node last used, even if Recommended picked it.
PreviousIsLastUsed ==
  (Succeeded /\ req.want = "previous" /\ req.prev \in Nodes) => eid = req.prev

\* Turning exit node use on either leaves it on or reports that it failed.
TurningOnTurnsOn ==
  Succeeded => eid # ""

\* A turn on that reports failure leaves exit node use off.
FailedTurnOnLeavesItOff ==
  (TurnedOn /\ req.failed) => (eid = "" /\ ~auto)
=============================================================================

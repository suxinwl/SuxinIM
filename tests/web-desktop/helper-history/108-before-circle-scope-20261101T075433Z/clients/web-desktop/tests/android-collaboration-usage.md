# Android original-UI collaboration helper

Test tooling only. Do not start until the Android owner confirms that its two Web101 call windows are finished. This helper does not connect to, reload, close, or reauthenticate Web101 (`127.0.0.1:17787`). No group, message, call or meeting is started on launch.

## Fixed isolated identities

| Role | IM user ID | Login fixture |
|---|---|---|
| Web102, default host/control peer | `upgrade38_u102` | `u102@openim38.test` |
| Web103, second participant | `upgrade38_u103` | `u103@openim38.test` |
| Android104 original UI | `upgrade38_u104` | `u104@openim38.test` / `im38u104` |

The script reads the existing synthetic password from the private fixture. Do not put credentials or JWTs in the public report. The target is the independent TLS test stack at `https://192.168.10.11:29880`, with normal certificate validation.

## Start only after coordination

Run from `clients/web-desktop`, using the project Node and the existing Chromium. This authenticates only Web102/103 in two fresh contexts of one dedicated headless browser, with synthetic camera/microphone/display sources.

```powershell
$suiteRoot = 'E:\PUTY_AI\OpenIM3.4-3.8\OpenIM3.8.3patch.18_1.8.4patch.5'
Set-Location -LiteralPath "$suiteRoot\clients\web-desktop"
$env:PLAYWRIGHT_CHROMIUM_EXECUTABLE = 'C:\Users\Administrator\AppData\Local\ms-playwright\chromium-1208\chrome-win64\chrome.exe'
& "$suiteRoot\tools\node-v22.23.4-win-x64\node.exe" '.\tests\android-collaboration.mjs' --start-confirmed
```

The existing Vite page at `http://127.0.0.1:7777` must be available. The helper uses E-drive temporary data, listens only on `127.0.0.1:17788`, rejects browser-origin commands, and has no arbitrary-evaluate endpoint. Do not start another copy on that port.

The108 helper waits for the original UI's asynchronous SDK initialization before
opening its control endpoint. Each `/state` peer now includes `health`: real SDK
login status3, the expected synthetic userID, a successful one-row SQLite
conversation query, WASM callback readiness and actual observer installation.
Opening the chat route alone is not treated as SDK/database readiness. This
startup check never performs a second direct SDK login.

In a separate control shell:

```powershell
function TestPost($route, $payload) {
  Invoke-RestMethod -Uri ("http://127.0.0.1:17788" + $route) -Method Post `
    -ContentType 'application/json' -Body ($payload | ConvertTo-Json -Depth 6 -Compress)
}
Invoke-RestMethod 'http://127.0.0.1:17788/state'
```

## Typing through the actual Web102 composer

After startup is coordinated and no call/dialog is active, Android104 opens its original single conversation with Web102. Open the corresponding Android104 conversation in Web102 and type a short test draft:

```powershell
TestPost '/open-android' @{ peer = '102' }
TestPost '/composer' @{ peer = '102'; action = 'type'; text = 'typing-probe Web102' }
# Observe Android's actual original UI typing indication, then clear the draft:
TestPost '/composer' @{ peer = '102'; action = 'clear' }
TestPost '/composer' @{ peer = '102'; action = 'blur' }
```

`type` clicks `#editable-div` and performs real sequential keyboard input. `clear` uses select-all/backspace within that input; `blur` uses Tab. These commands do not call SDK typing methods, replace the product event handler, press Send/Enter, or send a message. Only Web102's current conversation with the fixed Android104 account is allowed, only short `typing-probe` ASCII text is accepted, and an active dialog/call prevents the action. Calling `type` again appends to the current draft; clear first if a fresh draft is needed.

The frozen Web UI emits active typing for nonempty input changes; clearing/blur does not directly emit `focus=false`. Wait for actual SDK/UI expiry and verify the Android stop indication separately. Do not describe the clear command as an immediate inactive event. Android-to-Web101 typing can continue using the existing101 observer without restarting/relogging101.

## Group-call setup and original-UI acceptance

After explicit coordination, create the three-member synthetic fixture, announce it to Android104, and open it in both Web peers:

```powershell
$group = TestPost '/create-group' @{}
TestPost '/announce-group' @{ groupID = $group.groupID }
TestPost '/open-group' @{ peer = '102'; groupID = $group.groupID }
TestPost '/open-group' @{ peer = '103'; groupID = $group.groupID }
```

Group creation/announcement is setup through the real SDK, not Android UI coverage. Android104 must open that original group conversation, select the audio/video-call action and choose Web102 and Web103 in its original member picker. The helper does not inject an Android call or signaling request.

After each Web peer shows the actual incoming invitation:

```powershell
TestPost '/accept' @{ peer = '102' }
TestPost '/accept' @{ peer = '103' }
Invoke-RestMethod 'http://127.0.0.1:17788/state'
```

`/state` records each original Web call UI and each video's source, size, `currentTime`, decoded/playback frame counters and `requestVideoFrameCallback` counters. Sample twice or over time; a nonzero size alone does not establish continuous decoding. Android hangup, return to chat, immediate text entry/send, native cleanup and ANR checks remain Android-side actions/evidence. A follow-on audio call can use the same fixture without re-creating it.

The Android107 helper additionally reports `audios` and `transport.peers[].rtp`.
It observes the actual native browser peer connections through a transparent
test-page constructor proxy and weak references; it does not replace their
methods, media sources or product SDK. One native `getStats` request per peer
remains in flight until it actually settles. Read deadlines only label a pending
sample. At most 32 weak peer records are retained, and dropped records are counted.

For received audio, correlate inbound `kind: "audio"` RTP `trackIdentifier` with
`audios[].audioTracks[].trackID` (the helper also reports `audioElementIDs`).
Compare two samples: received packets/bytes and total samples or sample duration
should increase, while the original audio element has a live track, advancing
`currentTime`, `paused: false`, `muted: false`, nonzero volume and a ready state.
Energy or audio level can distinguish sound from silence when reported; a silent
source is not evidence of audible sound. This records browser transport/playback,
not physical speaker output. No ICE addresses, SDP, credentials or signed media
URLs are returned by this sampler.

`meetingControls` reads original meeting buttons and switches, including their
disabled/checked attributes. Host permission tests must still click the original
controls, inspect actual feedback and correlate enabled/disabled publication
with transport counters; DOM labels alone do not prove server enforcement.

## Android-hosted meeting and invitation-card path

For Android coverage, Android104 must use its original quick-meeting menu and original invite/share picker to send meeting cards to Web102/103. Read the received IDs:

```powershell
Invoke-RestMethod 'http://127.0.0.1:17788/messages'
```

The safe response includes `clientMsgID`, sender, custom subtype and `meetingRoomID` for subtype905; it never exposes room JWTs. Join by clicking the received original card:

```powershell
TestPost '/join-meeting-card' @{ peer = '102'; clientMsgID = '<card-for-Web102>' }
TestPost '/join-meeting-card' @{ peer = '103'; clientMsgID = '<card-for-Web103>' }
```

Only subtype905 cards actually received from the fixed Android104 account are accepted. Alternatively `/join-meeting` opens the original meeting-ID form for an already invited synthetic participant:

```powershell
TestPost '/join-meeting' @{ peer = '103'; roomID = '<32-hex-room-ID>' }
```

For the reverse direction, Web102 can create a quick meeting through its original UI and send real authorized SDK invitation cards. Android104 then joins by tapping its original received card:

```powershell
$meeting = TestPost '/create-meeting' @{ peer = '102' }
TestPost '/invite-meeting-card' @{ roomID = $meeting.roomID }
TestPost '/join-meeting' @{ peer = '103'; roomID = $meeting.roomID }
```

Do not run both directions simultaneously. Android scheduled-meeting create/reopen/persist, host microphone/camera/screen policy toggles, member list, invitation sharing, ending/leaving and post-hangup input must each be performed in the Android original UI if claimed as Android coverage.

## Media controls and screen-share observation

Web original controls are explicit commands:

```powershell
TestPost '/meeting-control' @{ peer = '102'; action = 'cameraOn' }
TestPost '/meeting-control' @{ peer = '102'; action = 'micOn' }
TestPost '/meeting-control' @{ peer = '103'; action = 'cameraOn' }
TestPost '/meeting-control' @{ peer = '103'; action = 'micOn' }
```

Allowed actions are `cameraOn`, `cameraOff`, `micOn`, `micOff`, `share`, `stopShare`, `members`, `leave`, `finish`. `finish` requires the actual host UI permission. `/meeting-setting` supports only the three original Boolean controls `participantCanUnmuteSelf`, `participantCanEnableVideo`, `onlyHostShareScreen` for a Web-hosted fixture; Android-host policy tests must use Android's UI.

For Android-to-Web sharing, Android104 must click its original share-screen control and accept Android's actual MediaProjection permission dialog. Web `/state` must then show a remote `source: "screen_share"`, `localParticipant: "false"`, a decodable size and increasing frame counters; it must disappear when Android stops sharing. This is stronger than merely seeing a published-track event.

Where available, inbound video RTP `trackIdentifier` also correlates with
`videos[].videoTracks[].trackID` (`videoElementIDs` is precomputed), allowing
received bytes/decoded frames to be paired with that exact original screen-share
element rather than an unrelated camera.

For Web-to-Android sharing, explicitly call `/meeting-control` with `action: "share"`; the source is a Chromium synthetic display, and Android rendering must be demonstrated on the Android side. This does not prove physical desktop or physical-device capture.

## Capture/stop only the dedicated peers

```powershell
TestPost '/screenshot' @{ peer = '102' }
TestPost '/screenshot' @{ peer = '103' }
# Group-call hangup on the indicated Web peer only:
TestPost '/hangup' @{ peer = '102' }
# After all coordinated cases have finished:
TestPost '/close' @{}
```

Reports and screenshots stay in `clients/web-desktop/test-results/android-collaboration`. Copy accepted evidence to the root formal test directory when the run is complete. The operation journal separates original Web UI controls, real-SDK fixture setup and synthetic-media limitations. This preparation alone is not execution evidence or Android acceptance.

## Fixed Android108 original-UI circle fixture

The test-only `/circle` command is restricted to the Android104 post whose exact
text is `android108-circle-select102`. It opens or refreshes the original Web
circle UI and observes that UI's list response and rendered DOM. It does not call
the enterprise API directly. Only Web102 may click the original Like/Comment
controls, and the only submitted comment is `android108-web102-comment`.

```powershell
TestPost '/circle' @{ peer = '102'; action = 'inspect' }
TestPost '/circle' @{ peer = '103'; action = 'inspect' }
TestPost '/circle' @{ peer = '102'; action = 'like' }
TestPost '/circle' @{ peer = '102'; action = 'comment' }
TestPost '/circle' @{ peer = '102'; action = 'inspect' }
TestPost '/circle' @{ peer = '102'; action = 'close' }
TestPost '/circle' @{ peer = '103'; action = 'close' }
```

The helper refresh can restore a previously confirmed synthetic group from its
saved `fixture-resume-108.json` only when `--resume-confirmed-fixtures` is supplied.
It validates the fixed102/103/104 member set and fixture-name prefix. Restoring
this local control record neither opens the group nor marks any message read.
Group SDK setup remains separate from Android original-UI coverage.

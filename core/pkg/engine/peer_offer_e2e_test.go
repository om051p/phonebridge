package engine

import (
	"context"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/pion/ice/v4"
	"github.com/pion/interceptor"
	pion "github.com/pion/webrtc/v4"

	"github.com/om051p/phonebridge/core/pkg/crypto"
)

// End-to-end coverage for the phone→PC direction (DEC-022 peer-offer), the path
// the comms audit found broken: the Android app POSTed to /offer (a route of the
// standalone receiver binary) and got 404, so a phone could never bring up a
// production session on the daemon. These tests drive the exact exchange the
// phone now performs — signed POST, real SDP offer, real answer, real ICE —
// against a daemon-shaped test node, and then check the lifecycle defects the
// audit listed around it (SESSION_BUSY wedges, peer disappearance).

// newPhoneSidePC builds a PeerConnection the way the phone's Go transport does:
// the phone is the OFFERER, loopback is allowed (both ends run on one host in
// tests), and mDNS obfuscation is off (LAN-only, real candidates).
func newPhoneSidePC(t *testing.T) *pion.PeerConnection {
	t.Helper()
	se := pion.SettingEngine{}
	se.SetICEMulticastDNSMode(ice.MulticastDNSModeDisabled)
	se.SetIncludeLoopbackCandidate(true)
	api := pion.NewAPI(
		pion.WithSettingEngine(se),
		pion.WithInterceptorRegistry(&interceptor.Registry{}),
	)
	pc, err := api.NewPeerConnection(pion.Configuration{ICEServers: []pion.ICEServer{}})
	if err != nil {
		t.Fatalf("phone peer connection: %v", err)
	}
	t.Cleanup(func() { _ = pc.Close() })
	return pc
}

// newPairedPhoneIdentity creates a phone identity that has already completed
// pairing with the desktop — i.e. exactly the trust state a shipped phone is in.
func newPairedPhoneIdentity(t *testing.T, desktop *testLinuxNode, name string) *crypto.DeviceIdentity {
	t.Helper()
	ident, err := crypto.LoadOrGenerateIdentity(filepath.Join(t.TempDir(), name+".json"), "POCO F5", "android")
	if err != nil {
		t.Fatalf("phone identity: %v", err)
	}
	if err := desktop.trustStore.AddTrusted(crypto.TrustEntry{
		DeviceID:    ident.DeviceID,
		DisplayName: ident.DisplayName,
		Platform:    ident.Platform,
		PublicKey:   ident.PublicKey,
		PairedAt:    time.Now(),
		LastSeen:    time.Now(),
	}); err != nil {
		t.Fatalf("trust the phone: %v", err)
	}
	return ident
}

// offerControlChannel opens a control DataChannel the way the phone does
// (ordered, unreliable-until-retransmit — unchanged reliability semantics).
func offerControlChannel(t *testing.T, pc *pion.PeerConnection) (*pion.DataChannel, *atomic.Bool) {
	t.Helper()
	ordered := true
	maxRetransmits := uint16(0)
	dc, err := pc.CreateDataChannel("control", &pion.DataChannelInit{
		Ordered:        &ordered,
		MaxRetransmits: &maxRetransmits,
	})
	if err != nil {
		t.Fatalf("create control channel: %v", err)
	}
	opened := &atomic.Bool{}
	dc.OnOpen(func() { opened.Store(true) })
	return dc, opened
}

// establishPhoneSession performs the phone's full handshake against the desktop
// and waits for the transport to come up on both ends. The caller opens the
// control channel first, exactly as the phone's transport does.
func establishPhoneSession(t *testing.T, desktop *testLinuxNode, phone *crypto.DeviceIdentity, pc *pion.PeerConnection) PeerOfferResponse {
	t.Helper()

	offer, err := pc.CreateOffer(nil)
	if err != nil {
		t.Fatalf("phone CreateOffer: %v", err)
	}
	if err := pc.SetLocalDescription(offer); err != nil {
		t.Fatalf("phone SetLocalDescription: %v", err)
	}

	select {
	case <-pion.GatheringCompletePromise(pc):
	case <-time.After(5 * time.Second):
		t.Fatalf("phone ICE gathering did not complete")
	}

	local := pc.LocalDescription()
	if local == nil || local.SDP == "" {
		t.Fatal("phone produced no local offer")
	}

	client := NewSignalingClient(5 * time.Second)
	client.SetIdentity(phone)

	res, err := client.SendPeerOffer(context.Background(), desktop.endpoint, *local, NegotiationRequest{
		Requested: MediaParams{Width: 1080, Height: 2400, FPS: 60},
	}, 7804)
	if err != nil {
		t.Fatalf("SendPeerOffer: %v", err)
	}
	if !res.Accepted || res.Code != CodeOK {
		t.Fatalf("desktop refused the session: %+v", res)
	}
	if res.Answer == "" {
		t.Fatal("desktop accepted but returned no answer")
	}
	if err := pc.SetRemoteDescription(pion.SessionDescription{Type: pion.SDPTypeAnswer, SDP: res.Answer}); err != nil {
		t.Fatalf("phone SetRemoteDescription: %v", err)
	}
	return res
}

// The phone→PC path, end to end: signed peer-offer → real answer → ICE up on
// both sides → a DataChannel open in both directions.
func TestPeerOffer_EndToEndSessionComesUp(t *testing.T) {
	desktop := createTestNode(t, t.TempDir(), "desktop")
	phone := newPairedPhoneIdentity(t, desktop, "phone")
	pc := newPhoneSidePC(t)
	_, controlOpened := offerControlChannel(t, pc)

	res := establishPhoneSession(t, desktop, phone, pc)

	if res.SessionID == "" {
		t.Fatal("the desktop did not report the session it created")
	}

	// The phone's own channel must open: that is what proves ICE/DTLS/SCTP
	// completed and the phone can start sending control frames.
	waitFor(t, 15*time.Second, "the phone's control channel to open", func() bool {
		return controlOpened.Load()
	})

	// The desktop's session must be past CONNECTING: the manager's slot is only
	// worth anything if the session it published is the one that came up.
	waitFor(t, 15*time.Second, "the desktop session to report a live transport", func() bool {
		desktop.manager.mu.RLock()
		defer desktop.manager.mu.RUnlock()
		if desktop.manager.activeSess == nil {
			return false
		}
		switch desktop.manager.activeSess.State() {
		case StateConnected, StateStreaming:
			return true
		}
		return false
	})
}

// A second peer-offer while a session is live must be refused with a typed
// SESSION_BUSY that names the live session, and must not disturb it.
func TestPeerOffer_BusyRefusalNamesLiveSession(t *testing.T) {
	desktop := createTestNode(t, t.TempDir(), "desktop")
	phone := newPairedPhoneIdentity(t, desktop, "phone")
	pc := newPhoneSidePC(t)
	offerControlChannel(t, pc)
	establishPhoneSession(t, desktop, phone, pc)

	// A different, equally trusted phone tries to start its own session.
	otherPhone := newPairedPhoneIdentity(t, desktop, "otherphone")
	otherPC := newPhoneSidePC(t)
	offerControlChannel(t, otherPC)
	offer, err := otherPC.CreateOffer(nil)
	if err != nil {
		t.Fatalf("second CreateOffer: %v", err)
	}
	if err := otherPC.SetLocalDescription(offer); err != nil {
		t.Fatalf("second SetLocalDescription: %v", err)
	}
	select {
	case <-pion.GatheringCompletePromise(otherPC):
	case <-time.After(5 * time.Second):
		t.Fatal("second phone ICE gathering did not complete")
	}

	client := NewSignalingClient(5 * time.Second)
	client.SetIdentity(otherPhone)
	res, err := client.SendPeerOffer(context.Background(), desktop.endpoint, *otherPC.LocalDescription(),
		NegotiationRequest{}, 7804)
	if err != nil {
		t.Fatalf("second SendPeerOffer must be a typed refusal, not an error: %v", err)
	}
	if res.Accepted {
		t.Fatal("a second peer-offer was accepted while a session was live")
	}
	if res.Code != CodeSessionBusy {
		t.Fatalf("code = %s, want SESSION_BUSY", res.Code)
	}
	if res.SessionID == "" {
		t.Fatal("a busy refusal must name the live session so the caller can tell 'someone else is connected' from 'I am still connected'")
	}

	// The original session is untouched.
	desktop.manager.mu.RLock()
	active := desktop.manager.activeSess
	desktop.manager.mu.RUnlock()
	if active == nil {
		t.Fatal("the live session was dropped by the refused offer")
	}
	if st := active.State(); st == StateStopped || st == StateFailed || st == StateDisconnected {
		t.Fatalf("live session went terminal (%s) after a refused offer", st)
	}
}

// The phone stopping must release the session immediately: without that, the
// next offer arriving before the transport timeout is refused as SESSION_BUSY.
func TestPeerOffer_PhoneStopReleasesTheSession(t *testing.T) {
	desktop := createTestNode(t, t.TempDir(), "desktop")
	phone := newPairedPhoneIdentity(t, desktop, "phone")
	pc := newPhoneSidePC(t)
	offerControlChannel(t, pc)
	establishPhoneSession(t, desktop, phone, pc)

	client := NewSignalingClient(5 * time.Second)
	client.SetIdentity(phone)
	if err := client.StopSession(context.Background(), desktop.endpoint, "user stopped sharing", CodeOK); err != nil {
		t.Fatalf("StopSession: %v", err)
	}

	waitFor(t, 10*time.Second, "the desktop to release the peer's session", func() bool {
		desktop.manager.mu.RLock()
		defer desktop.manager.mu.RUnlock()
		return desktop.manager.activeSess == nil || sessionTerminal(desktop.manager.activeSess)
	})

	// And a fresh offer is accepted right away.
	otherPC := newPhoneSidePC(t)
	offerControlChannel(t, otherPC)
	establishPhoneSession(t, desktop, phone, otherPC)
}

// Peer death: the phone answers and then disappears before ICE completes. The
// session must fail and free the slot instead of answering SESSION_BUSY forever.
func TestPeerOffer_PeerDeathFreesTheSlot(t *testing.T) {
	desktop := createTestNode(t, t.TempDir(), "desktop")
	phone := newPairedPhoneIdentity(t, desktop, "phone")
	pc := newPhoneSidePC(t)
	offerControlChannel(t, pc)

	offer, err := pc.CreateOffer(nil)
	if err != nil {
		t.Fatalf("CreateOffer: %v", err)
	}
	if err := pc.SetLocalDescription(offer); err != nil {
		t.Fatalf("SetLocalDescription: %v", err)
	}
	select {
	case <-pion.GatheringCompletePromise(pc):
	case <-time.After(5 * time.Second):
		t.Fatal("ICE gathering did not complete")
	}

	client := NewSignalingClient(5 * time.Second)
	client.SetIdentity(phone)
	res, err := client.SendPeerOffer(context.Background(), desktop.endpoint, *pc.LocalDescription(),
		NegotiationRequest{}, 7804)
	if err != nil {
		t.Fatalf("SendPeerOffer: %v", err)
	}
	if !res.Accepted {
		t.Fatalf("first offer refused: %+v", res)
	}
	if err := pc.SetRemoteDescription(pion.SessionDescription{Type: pion.SDPTypeAnswer, SDP: res.Answer}); err != nil {
		t.Fatalf("SetRemoteDescription: %v", err)
	}

	// The peer vanishes mid-handshake — no /session/stop, exactly the case the
	// audit called out. Closing drops ICE candidates before DTLS can finish, so
	// the desktop's connect watchdog is what has to reclaim the slot.
	_ = pc.Close()

	waitFor(t, 15*time.Second, "the desktop to fail the dead peer's session", func() bool {
		desktop.manager.mu.RLock()
		defer desktop.manager.mu.RUnlock()
		return desktop.manager.activeSess == nil || sessionTerminal(desktop.manager.activeSess)
	})

	// A later peer gets a session instead of a permanent SESSION_BUSY.
	otherPC := newPhoneSidePC(t)
	offerControlChannel(t, otherPC)
	establishPhoneSession(t, desktop, phone, otherPC)
}

func sessionTerminal(s *Session) bool {
	switch s.State() {
	case StateStopped, StateFailed, StateDisconnected:
		return true
	}
	return false
}

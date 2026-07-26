package peer

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"

	"github.com/LosFurina/tmuxatlas/pkg/identity"
	"github.com/LosFurina/tmuxatlas/pkg/toolevents"
)

func TestPeerFlowsThroughReverseProxy(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	hubIdentity, err := identity.Generate("hub")
	if err != nil {
		t.Fatal(err)
	}
	remoteIdentity, err := identity.Generate("peer")
	if err != nil {
		t.Fatal(err)
	}
	peerStore, err := identity.NewPeerStore()
	if err != nil {
		t.Fatal(err)
	}
	pairing := identity.NewPairingManager()
	relay := NewPTYRelay()
	manager := NewManager(hubIdentity, peerStore, nil)
	handler := NewHandler(manager, peerStore, toolevents.NewTracker(), pairing, relay)

	backendMux := http.NewServeMux()
	backendMux.HandleFunc("/api/pair/complete", handler.HandlePairing)
	backendMux.HandleFunc("/ws/peer", handler.HandlePeer)
	backendMux.HandleFunc("/ws/peer-pty", relay.HandlePeerPTY)
	backend := httptest.NewServer(backendMux)
	defer backend.Close()

	backendURL, err := url.Parse(backend.URL)
	if err != nil {
		t.Fatal(err)
	}
	proxy := httptest.NewServer(httputil.NewSingleHostReverseProxy(backendURL))
	defer proxy.Close()

	code, err := pairing.Generate()
	if err != nil {
		t.Fatal(err)
	}
	pairBody, err := json.Marshal(map[string]string{
		"code":       code.Code,
		"name":       remoteIdentity.Name,
		"public_key": remoteIdentity.PublicKey,
	})
	if err != nil {
		t.Fatal(err)
	}
	resp, err := http.Post(proxy.URL+"/api/pair/complete", "application/json", bytes.NewReader(pairBody))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("pairing returned %s", resp.Status)
	}
	var pairResponse map[string]string
	if err := json.NewDecoder(resp.Body).Decode(&pairResponse); err != nil {
		t.Fatal(err)
	}
	if _, exists := pairResponse["ca_cert_pem"]; exists {
		t.Fatal("pairing response exposed legacy CA certificate material")
	}

	proxyWS := "ws" + strings.TrimPrefix(proxy.URL, "http")
	control, _, err := websocket.DefaultDialer.Dial(proxyWS+"/ws/peer", nil)
	if err != nil {
		t.Fatal(err)
	}

	var challenge Message
	if err := control.ReadJSON(&challenge); err != nil {
		t.Fatal(err)
	}
	if challenge.Type != MsgChallenge {
		t.Fatalf("first control message type = %q, want %q", challenge.Type, MsgChallenge)
	}
	var challengePayload ChallengePayload
	if err := json.Unmarshal(challenge.Payload, &challengePayload); err != nil {
		t.Fatal(err)
	}
	challengeBytes, err := base64.StdEncoding.DecodeString(challengePayload.Challenge)
	if err != nil {
		t.Fatal(err)
	}
	signature, err := remoteIdentity.Sign(challengeBytes)
	if err != nil {
		t.Fatal(err)
	}
	auth, err := NewMessage(MsgAuth, AuthPayload{
		PublicKey: remoteIdentity.PublicKey,
		Signature: base64.StdEncoding.EncodeToString(signature),
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := control.WriteJSON(auth); err != nil {
		t.Fatal(err)
	}
	var authResult Message
	if err := control.ReadJSON(&authResult); err != nil {
		t.Fatal(err)
	}
	if authResult.Type != MsgAuthOK {
		t.Fatalf("authentication result = %q, want %q", authResult.Type, MsgAuthOK)
	}
	if err := control.Close(); err != nil {
		t.Fatal(err)
	}

	pending := relay.RegisterPending("proxy-stream", remoteIdentity.Fingerprint(), nil)
	ptyClient, _, err := websocket.DefaultDialer.Dial(proxyWS+"/ws/peer-pty?stream=proxy-stream", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer ptyClient.Close()
	select {
	case ptyServer := <-pending.Ready:
		if ptyServer == nil {
			t.Fatal("PTY relay received a nil server connection")
		}
		defer ptyServer.Close()
	case <-time.After(2 * time.Second):
		t.Fatal("PTY WebSocket did not reach the relay through the reverse proxy")
	}
}

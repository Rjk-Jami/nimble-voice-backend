package server

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/shishir1290/gsocketio"
)

// Helper to extract session ID from Engine.IO v4 handshake response: 0{"sid":"...","upgrades":[...],...}
func extractSID(body string) string {
	if len(body) < 2 || body[0] != '0' {
		return ""
	}
	var data struct {
		SID string `json:"sid"`
	}
	if err := json.Unmarshal([]byte(body[1:]), &data); err != nil {
		return ""
	}
	return data.SID
}

func setupTestServer(t *testing.T) (*SocketServer, *httptest.Server) {
	opts := &gsocketio.Options{
		PingInterval: 5 * time.Second,
		PingTimeout:  3 * time.Second,
		MaxPayload:   512 * 1024,
	}

	sockServer, err := NewSocketServer(opts)
	if err != nil {
		t.Fatalf("Failed to create SocketServer: %v", err)
	}

	go func() {
		_ = sockServer.Serve()
	}()

	ts := httptest.NewServer(sockServer)
	return sockServer, ts
}

func TestSocketServer_HandshakeAndCORS(t *testing.T) {
	sockServer, ts := setupTestServer(t)
	defer ts.Close()
	defer sockServer.Close()

	client := &http.Client{Timeout: 5 * time.Second}

	// 1. Test CORS preflight on /socket.io/
	req, err := http.NewRequest(http.MethodOptions, ts.URL+"/socket.io/?EIO=4&transport=polling", nil)
	if err != nil {
		t.Fatalf("Failed to create OPTIONS request: %v", err)
	}
	req.Header.Set("Origin", "http://localhost:3000")
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("OPTIONS request failed: %v", err)
	}
	_ = resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status 204 or 200 for OPTIONS, got: %d", resp.StatusCode)
	}
	if origin := resp.Header.Get("Access-Control-Allow-Origin"); origin != "http://localhost:3000" {
		t.Errorf("Expected Allow-Origin 'http://localhost:3000', got: %s", origin)
	}
	if creds := resp.Header.Get("Access-Control-Allow-Credentials"); creds != "true" {
		t.Errorf("Expected Allow-Credentials 'true', got: %s", creds)
	}

	// 2. Test Engine.IO v4 Handshake
	handshakeURL := ts.URL + "/socket.io/?EIO=4&transport=polling"
	req, _ = http.NewRequest(http.MethodGet, handshakeURL, nil)
	req.Header.Set("Origin", "http://localhost:3000")
	resp, err = client.Do(req)
	if err != nil {
		t.Fatalf("GET handshake request failed: %v", err)
	}
	defer resp.Body.Close()

	bodyBytes, _ := io.ReadAll(resp.Body)
	bodyStr := string(bodyBytes)

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("Expected 200 OK for handshake, got %d: %s", resp.StatusCode, bodyStr)
	}

	sid := extractSID(bodyStr)
	if sid == "" {
		t.Fatalf("Expected valid sid from Engine.IO handshake, got body: %s", bodyStr)
	}

	if !strings.Contains(bodyStr, "pingInterval") || !strings.Contains(bodyStr, "pingTimeout") {
		t.Errorf("Expected pingInterval and pingTimeout in handshake, got: %s", bodyStr)
	}
}

func TestSocketServer_RoomJoinAndIsolation(t *testing.T) {
	sockServer, ts := setupTestServer(t)
	defer ts.Close()
	defer sockServer.Close()

	client := &http.Client{Timeout: 5 * time.Second}

	// Connect Client A
	respA, err := client.Get(ts.URL + "/socket.io/?EIO=4&transport=polling")
	if err != nil {
		t.Fatalf("Client A handshake failed: %v", err)
	}
	bodyA, _ := io.ReadAll(respA.Body)
	_ = respA.Body.Close()
	sidA := extractSID(string(bodyA))

	// Connect Client B
	respB, err := client.Get(ts.URL + "/socket.io/?EIO=4&transport=polling")
	if err != nil {
		t.Fatalf("Client B handshake failed: %v", err)
	}
	bodyB, _ := io.ReadAll(respB.Body)
	_ = respB.Body.Close()
	sidB := extractSID(string(bodyB))

	// Send Connect packet for Client A
	connectPacket := bytes.NewBufferString(`40{"user_id":"user-a","token":"valid-token"}`)
	resp, err := client.Post(ts.URL+"/socket.io/?EIO=4&transport=polling&sid="+sidA, "text/plain", connectPacket)
	if err != nil {
		t.Fatalf("Client A connect post failed: %v", err)
	}
	_ = resp.Body.Close()

	// Send Connect packet for Client B
	connectPacketB := bytes.NewBufferString(`40{"user_id":"user-b","token":"valid-token"}`)
	resp, err = client.Post(ts.URL+"/socket.io/?EIO=4&transport=polling&sid="+sidB, "text/plain", connectPacketB)
	if err != nil {
		t.Fatalf("Client B connect post failed: %v", err)
	}
	_ = resp.Body.Close()

	// Wait briefly for handshake registration
	time.Sleep(50 * time.Millisecond)

	roomID := "voice-room-party-101"

	// Client A joins room
	joinEventA := bytes.NewBufferString(`42["room:join",{"room_id":"` + roomID + `","user_id":"user-a","role":"host"}]`)
	resp, err = client.Post(ts.URL+"/socket.io/?EIO=4&transport=polling&sid="+sidA, "text/plain", joinEventA)
	if err != nil {
		t.Fatalf("Client A join post failed: %v", err)
	}
	_ = resp.Body.Close()

	time.Sleep(50 * time.Millisecond)
	if count := sockServer.Server.RoomLen("/", roomID); count != 1 {
		t.Errorf("Expected 1 member in room after Client A joined, got: %d", count)
	}

	// Client B joins same room
	joinEventB := bytes.NewBufferString(`42["room:join",{"room_id":"` + roomID + `","user_id":"user-b","role":"listener"}]`)
	resp, err = client.Post(ts.URL+"/socket.io/?EIO=4&transport=polling&sid="+sidB, "text/plain", joinEventB)
	if err != nil {
		t.Fatalf("Client B join post failed: %v", err)
	}
	_ = resp.Body.Close()

	time.Sleep(50 * time.Millisecond)
	if count := sockServer.Server.RoomLen("/", roomID); count != 2 {
		t.Errorf("Expected 2 members in room after Client B joined, got: %d", count)
	}

	// Client A toggles mute
	muteEventA := bytes.NewBufferString(`42["room:mute",{"room_id":"` + roomID + `","user_id":"user-a","is_muted":true}]`)
	resp, err = client.Post(ts.URL+"/socket.io/?EIO=4&transport=polling&sid="+sidA, "text/plain", muteEventA)
	if err != nil {
		t.Fatalf("Client A mute post failed: %v", err)
	}
	_ = resp.Body.Close()

	// Client A leaves room
	leaveEventA := bytes.NewBufferString(`42["room:leave",{"room_id":"` + roomID + `","user_id":"user-a"}]`)
	resp, err = client.Post(ts.URL+"/socket.io/?EIO=4&transport=polling&sid="+sidA, "text/plain", leaveEventA)
	if err != nil {
		t.Fatalf("Client A leave post failed: %v", err)
	}
	_ = resp.Body.Close()

	time.Sleep(50 * time.Millisecond)
	if count := sockServer.Server.RoomLen("/", roomID); count != 1 {
		t.Errorf("Expected 1 member in room after Client A left, got: %d", count)
	}
}

func TestSocketServer_ConcurrentAccess(t *testing.T) {
	sockServer, ts := setupTestServer(t)
	defer ts.Close()
	defer sockServer.Close()

	client := &http.Client{Timeout: 5 * time.Second}

	var wg sync.WaitGroup
	numClients := 10

	for i := 0; i < numClients; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			resp, err := client.Get(ts.URL + "/socket.io/?EIO=4&transport=polling")
			if err != nil {
				return
			}
			body, _ := io.ReadAll(resp.Body)
			_ = resp.Body.Close()
			sid := extractSID(string(body))
			if sid == "" {
				return
			}

			// Connect
			connBody := bytes.NewBufferString(`40{"user_id":"user-idx"}`)
			r, err := client.Post(ts.URL+"/socket.io/?EIO=4&transport=polling&sid="+sid, "text/plain", connBody)
			if err == nil {
				_ = r.Body.Close()
			}

			// Join room
			joinBody := bytes.NewBufferString(`42["room:join",{"room_id":"concurrency-room"}]`)
			r, err = client.Post(ts.URL+"/socket.io/?EIO=4&transport=polling&sid="+sid, "text/plain", joinBody)
			if err == nil {
				_ = r.Body.Close()
			}
		}(i)
	}

	wg.Wait()
}

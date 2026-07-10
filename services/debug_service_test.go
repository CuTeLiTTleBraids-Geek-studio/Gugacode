package services

import (
	"bufio"
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func TestWriteAndReadDAPMessage(t *testing.T) {
	var buf bytes.Buffer
	payload := map[string]interface{}{
		"seq": 1, "type": "request", "command": "initialize",
		"arguments": map[string]interface{}{"clientID": "gugacode"},
	}
	if err := writeDAPMessage(&buf, payload); err != nil {
		t.Fatal(err)
	}
	raw := buf.String()
	if !strings.Contains(raw, "Content-Length:") {
		t.Fatalf("missing header: %q", raw)
	}
	r := bufio.NewReader(&buf)
	msg, err := readDAPMessage(r)
	if err != nil {
		t.Fatal(err)
	}
	if msg.Type != "request" || msg.Command != "initialize" || msg.Seq != 1 {
		t.Fatalf("parsed %+v", msg)
	}
}

func TestDAPMessage_EventJSON(t *testing.T) {
	body := json.RawMessage(`{"reason":"breakpoint","threadId":1}`)
	raw, _ := json.Marshal(dapMessage{
		Seq: 2, Type: "event", Event: "stopped", Body: body,
	})
	var msg dapMessage
	if err := json.Unmarshal(raw, &msg); err != nil {
		t.Fatal(err)
	}
	if msg.Event != "stopped" {
		t.Fatalf("event=%q", msg.Event)
	}
}

func TestDebugService_ToggleBreakpointOffline(t *testing.T) {
	d := NewDebugService()
	bps, err := d.ToggleBreakpoint("/tmp/main.go", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(bps) != 1 || bps[0].Line != 10 {
		t.Fatalf("got %+v", bps)
	}
	bps, err = d.ToggleBreakpoint("/tmp/main.go", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(bps) != 0 {
		t.Fatalf("expected empty after toggle off, got %+v", bps)
	}
}

func TestDebugService_StatusMessage_NoDlv(t *testing.T) {
	d := NewDebugService()
	msg := d.StatusMessage()
	if msg == "" {
		t.Fatal("expected status message")
	}
}

func TestDebugService_GetState_Empty(t *testing.T) {
	d := NewDebugService()
	st := d.GetState()
	if st.Session.Running {
		t.Fatal("expected not running")
	}
}

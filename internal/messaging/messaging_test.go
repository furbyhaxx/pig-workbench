package messaging

import (
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type recorder struct {
	info     Info
	messages chan Envelope
}

func (r *recorder) Info() Info { return r.info }
func (r *recorder) Message(e Envelope) Receipt {
	r.messages <- e
	return Receipt{Status: Accepted, Delivery: Steered, MessageID: e.ID, ConversationID: e.ConversationID}
}

func shortSocket(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("", "wbm")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	return filepath.Join(dir, "e.sock")
}

func serve(t *testing.T) (string, *recorder) {
	t.Helper()
	path := shortSocket(t)
	r := &recorder{info: Info{SessionID: "target", State: "idle"}, messages: make(chan Envelope, 4)}
	server, err := Listen(path, "secret-token", r)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(server.Close)
	return path, r
}

func message(body string) Envelope {
	return Envelope{ID: "m1", ConversationID: "c1", ToSessionID: "target", From: Peer{SessionID: "sender"}, Body: body}
}

func TestAuthenticatedPeerReachesTheHandler(t *testing.T) {
	path, r := serve(t)
	reply, err := Call(path, "secret-token", "ep", Request{Type: "message", Message: ptr(message("hello"))})
	if err != nil || reply.Receipt == nil || reply.Receipt.Status != Accepted || reply.Receipt.Delivery != Steered {
		t.Fatalf("reply = %+v, err = %v", reply, err)
	}
	if got := <-r.messages; got.Body != "hello" {
		t.Fatalf("handler saw %q", got.Body)
	}
	info, err := Call(path, "secret-token", "ep", Request{Type: "info"})
	if err != nil || info.Info == nil || info.Info.State != "idle" {
		t.Fatalf("info = %+v, err = %v", info, err)
	}
}

func TestWrongTokenGetsNoReplyAndNothingIsDelivered(t *testing.T) {
	path, r := serve(t)
	if _, err := Call(path, "not-the-token", "ep", Request{Type: "message", Message: ptr(message("intrusion"))}); err == nil {
		t.Fatal("a wrong token must not receive a reply")
	}
	select {
	case got := <-r.messages:
		t.Fatalf("the handler saw %q", got.Body)
	case <-time.After(100 * time.Millisecond):
	}
}

func TestRawConnectionWithoutHelloIsClosed(t *testing.T) {
	path, r := serve(t)
	conn, err := net.Dial("unix", path)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	if err := writeFrame(conn, Request{V: 1, Type: "message", Message: ptr(message("skip hello"))}); err != nil {
		t.Fatal(err)
	}
	conn.SetReadDeadline(time.Now().Add(time.Second))
	var reply Reply
	if err := readFrame(conn, &reply); err == nil {
		t.Fatalf("got a reply without hello: %+v", reply)
	}
	if len(r.messages) != 0 {
		t.Fatal("an unauthenticated request reached the handler")
	}
}

func TestOversizeAndSelfMessagesAreRejected(t *testing.T) {
	path, _ := serve(t)
	reply, err := Call(path, "secret-token", "ep", Request{Type: "message", Message: ptr(message(strings.Repeat("x", MaxBody+1)))})
	if err != nil || reply.Receipt.Reason != TooLarge {
		t.Fatalf("oversize reply = %+v, err = %v", reply, err)
	}
	self := message("hi")
	self.From.SessionID = "target"
	if _, err := Call(path, "secret-token", "ep", Request{Type: "message", Message: &self}); err == nil {
		t.Fatal("a message from the target to itself must be refused")
	}
}

func TestOversizedFrameClosesTheConnection(t *testing.T) {
	path, _ := serve(t)
	conn, err := net.Dial("unix", path)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	if _, err := conn.Write([]byte{0x7f, 0xff, 0xff, 0xff}); err != nil {
		t.Fatal(err)
	}
	conn.SetReadDeadline(time.Now().Add(time.Second))
	if err := readFrame(conn, &Reply{}); err == nil {
		t.Fatal("an oversized frame must close the connection without a reply")
	}
}

func TestListenReplacesAStaleSocketButNotALiveOne(t *testing.T) {
	path, _ := serve(t)
	if _, err := Listen(path, "other", &recorder{}); err == nil {
		t.Fatal("a live endpoint must not be taken over")
	}
	stale := shortSocket(t)
	listener, err := net.Listen("unix", stale)
	if err != nil {
		t.Fatal(err)
	}
	listener.(*net.UnixListener).SetUnlinkOnClose(false)
	listener.Close()
	server, err := Listen(stale, "t", &recorder{messages: make(chan Envelope, 1)})
	if err != nil {
		t.Fatalf("a stale socket file must be replaced: %v", err)
	}
	server.Close()
}

func TestLimiterWindowsSlide(t *testing.T) {
	l := NewLimiter()
	now := time.Now()
	for i := 0; i < 3; i++ {
		if !l.AllowMessage("p", now, 3) {
			t.Fatalf("message %d refused", i)
		}
	}
	if l.AllowMessage("p", now, 3) {
		t.Fatal("the fourth message in a minute must be refused")
	}
	if !l.AllowMessage("p", now.Add(61*time.Second), 3) || !l.AllowMessage("q", now, 3) {
		t.Fatal("the window must slide and peers must be independent")
	}
	if !l.AllowWake("p", now, 1) || l.AllowWake("p", now.Add(time.Minute), 1) || !l.AllowWake("p", now.Add(61*time.Minute), 1) {
		t.Fatal("the wake budget is per hour")
	}
}

func ptr[T any](v T) *T { return &v }

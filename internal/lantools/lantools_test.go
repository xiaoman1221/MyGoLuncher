package lantools

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func testTarget(server *Server) Target {
	return Target{Address: "127.0.0.1", Port: server.Port(), Token: server.Token()}
}

func TestServerReceivesChatVoiceAndFile(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	inbox := t.TempDir()
	received := make(chan Message, 3)
	server, err := Start(ctx, "self", "Steve", inbox, func(message Message) {
		received <- message
	})
	if err != nil {
		t.Fatal(err)
	}
	defer server.Stop()
	target := testTarget(server)

	if err := SendChat(ctx, target, "peer", "Alex", "hello LAN"); err != nil {
		t.Fatal(err)
	}
	chat := waitMessage(t, received)
	if chat.Kind != KindChat || chat.Text != "hello LAN" || chat.FromName != "Alex" || !chat.Incoming {
		t.Fatalf("unexpected chat: %#v", chat)
	}

	if err := SendVoice(ctx, target, "peer", "Alex", "audio/webm", []byte("voice-bytes"), 1200); err != nil {
		t.Fatal(err)
	}
	voice := waitMessage(t, received)
	if voice.Kind != KindVoice || voice.DurationMS != 1200 {
		t.Fatalf("unexpected voice: %#v", voice)
	}
	data, mime, ok := server.Voice(voice.ID)
	if !ok || string(data) != "voice-bytes" || mime != "audio/webm" {
		t.Fatalf("voice cache = %q %q %v", data, mime, ok)
	}

	source := filepath.Join(t.TempDir(), "hello world.txt")
	if err := os.WriteFile(source, []byte("file-bytes"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := SendFile(ctx, target, "peer", "Alex", source); err != nil {
		t.Fatal(err)
	}
	file := waitMessage(t, received)
	if file.Kind != KindFile || file.FileName != "hello world.txt" || file.Size != int64(len("file-bytes")) {
		t.Fatalf("unexpected file: %#v", file)
	}
	if stored, err := os.ReadFile(file.Path); err != nil || string(stored) != "file-bytes" {
		t.Fatalf("received file = %q %v", stored, err)
	}
	if state := server.State(); len(state.Messages) != 3 {
		t.Fatalf("history = %#v", state.Messages)
	}
}

func TestServerRejectsBadTokenAndUnsafeNames(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	server, err := Start(ctx, "self", "Steve", t.TempDir(), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer server.Stop()
	target := testTarget(server)
	target.Token = "wrong"
	if err := SendChat(ctx, target, "peer", "Alex", "hello"); err == nil {
		t.Fatal("expected rejected token")
	}
	if name := safeName("../../etc/passwd"); strings.Contains(name, "..") || strings.Contains(name, "/") {
		t.Fatalf("unsafe name = %q", name)
	}
}

func TestAddOutgoingMessages(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	server, err := Start(ctx, "self", "Steve", t.TempDir(), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer server.Stop()
	chat := server.AddOutgoingChat("hi")
	voice := server.AddOutgoingVoice([]byte("x"), "audio/webm", 10)
	file := server.AddOutgoingFile(FileInfo{Path: "C:\\tmp\\a.jar", Name: "a.jar", Size: 5})
	if chat.Incoming || voice.Incoming || file.Incoming {
		t.Fatal("local messages must not be marked incoming")
	}
	if _, _, ok := server.Voice(voice.ID); !ok {
		t.Fatal("outgoing voice was not cached")
	}
	if state := server.State(); state.SelfName != "Steve" || len(state.Messages) != 3 {
		t.Fatalf("state = %#v", state)
	}
}

func waitMessage(t *testing.T, channel <-chan Message) Message {
	t.Helper()
	select {
	case message := <-channel:
		return message
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for LAN tool message")
		return Message{}
	}
}

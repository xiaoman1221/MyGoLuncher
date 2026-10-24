package launcher

import (
	"strings"
	"testing"

	"MyMinecraftGoLuncher/internal/mojang"
	"MyMinecraftGoLuncher/internal/store"
)

func TestLogParserFormatsLog4jXML(t *testing.T) {
	parser := &logParser{}
	input := []string{
		`<log4j:Event logger="net.minecraft.client.Minecraft" timestamp="1792828412778" level="INFO" thread="Render thread">`,
		`<log4j:Message><![CDATA[Setting user: Steve]]></log4j:Message>`,
		`</log4j:Event>`,
		`plain console line`,
	}
	var output []string
	for _, line := range input {
		output = append(output, parser.Feed(line)...)
	}
	if len(output) != 2 {
		t.Fatalf("expected 2 output lines, got %#v", output)
	}
	if !strings.Contains(output[0], "[Render thread/INFO] Setting user: Steve") {
		t.Fatalf("event was not formatted: %q", output[0])
	}
	if output[1] != "plain console line" {
		t.Fatalf("plain line changed: %q", output[1])
	}
}

func TestLogParserFormatsThrowable(t *testing.T) {
	parser := &logParser{}
	input := []string{
		`<log4j:Event logger="auth" timestamp="1792828412778" level="ERROR" thread="Download-2">`,
		`<log4j:Message><![CDATA[Failed to fetch user properties]]></log4j:Message>`,
		`<log4j:Throwable><![CDATA[com.mojang.authlib.exceptions.InvalidCredentialsException: Status: 401`,
		`  at com.example.Thing(Thing.java:12)]]></log4j:Throwable>`,
		`</log4j:Event>`,
	}
	var output []string
	for _, line := range input {
		output = append(output, parser.Feed(line)...)
	}
	if len(output) < 3 {
		t.Fatalf("expected message and stack trace, got %#v", output)
	}
	if !strings.Contains(output[0], "Failed to fetch user properties") {
		t.Fatalf("message missing: %#v", output)
	}
	joined := strings.Join(output, "\n")
	if !strings.Contains(joined, "InvalidCredentialsException") || !strings.Contains(joined, "at com.example.Thing") {
		t.Fatalf("stack trace missing: %s", joined)
	}
}

func TestLogParserFlushesIncompleteEvent(t *testing.T) {
	parser := &logParser{}
	if out := parser.Feed(`<log4j:Event logger="x" timestamp="1" level="INFO" thread="main">`); len(out) != 0 {
		t.Fatalf("partial event should be buffered: %#v", out)
	}
	output := parser.Flush()
	if len(output) == 0 || !strings.Contains(strings.Join(output, "\n"), "log4j:Event") {
		t.Fatalf("incomplete event was lost: %#v", output)
	}
}

func TestBuildArgumentsUsesAccountAndInjector(t *testing.T) {
	l := &Launcher{Paths: store.PathsFor(t.TempDir())}
	merged := mojang.Version{
		Type: "release",
		Arguments: mojang.Arguments{
			Game: []mojang.Argument{{Value: []string{
				"--username", "${auth_player_name}",
				"--uuid", "${auth_uuid}",
				"--accessToken", "${auth_access_token}",
				"--xuid", "${auth_xuid}",
				"--userType", "${user_type}",
			}}},
		},
	}
	opts := Options{
		Instance:   store.Instance{MemoryMB: 4096},
		Settings:   store.Settings{PlayerName: "Fallback"},
		PlayerName: "Alex", PlayerUUID: "069a79f444e94726a5befca90e38aaf5",
		AccessToken: "secret-token", UserType: "msa", XUID: "12345",
		AuthlibJar: "/tmp/authlib-injector.jar", AuthlibServer: "https://example.com/api/yggdrasil",
	}
	jvm, game, err := l.buildArguments(merged, opts, "/tmp/game", "/tmp/natives", "/tmp/natives", "/tmp/classes", "/tmp/assets", "1.21.1")
	if err != nil {
		t.Fatal(err)
	}
	joinedJVM := strings.Join(jvm, " ")
	if !strings.Contains(joinedJVM, "-javaagent:/tmp/authlib-injector.jar=https://example.com/api/yggdrasil") {
		t.Fatalf("authlib agent missing: %s", joinedJVM)
	}
	joinedGame := strings.Join(game, " ")
	for _, want := range []string{"Alex", "069a79f444e94726a5befca90e38aaf5", "secret-token", "12345", "msa"} {
		if !strings.Contains(joinedGame, want) {
			t.Fatalf("game arguments missing %q: %v", want, game)
		}
	}
	redacted := redactCommand(append(jvm, game...))
	if strings.Contains(strings.Join(redacted, " "), "secret-token") {
		t.Fatal("access token was not redacted from the command log")
	}
}

func TestRedactCommandHandlesEveryTokenForm(t *testing.T) {
	args := []string{
		"java", "--accessToken", "separate-token",
		"--authSession=joined-token", "--session", "session-token",
		"--token=equals-token", "--username", "Alex",
	}
	redacted := strings.Join(redactCommand(args), " ")
	for _, secret := range []string{"separate-token", "joined-token", "session-token", "equals-token"} {
		if strings.Contains(redacted, secret) {
			t.Fatalf("%q leaked into the command log: %s", secret, redacted)
		}
	}
	if !strings.Contains(redacted, "--username Alex") {
		t.Fatalf("non-secret arguments were changed: %s", redacted)
	}
}

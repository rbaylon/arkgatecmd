package arkgatecmd

import (
	"encoding/json"
	"io"
	"net"
	"testing"
)

// fakeArkgated stands in for arkgated's connection handler: it reads one
// JSON command and writes back a fixed raw response, then closes - matching
// how arkgated answers "ActiveHealthChecks"/"ClearHealthChecks" (a bare JSON
// object, not the {ok,output,error} shape SendCmdOutput expects).
func fakeArkgated(t *testing.T, respond func(cmd Arkcmd) []byte) net.Conn {
	t.Helper()
	client, server := net.Pipe()
	go func() {
		defer server.Close()
		buf := make([]byte, 4096)
		n, err := server.Read(buf)
		if err != nil {
			return
		}
		var cmd Arkcmd
		if err := json.Unmarshal(buf[:n], &cmd); err != nil {
			return
		}
		server.Write(respond(cmd))
	}()
	return client
}

func TestGetActiveHealthChecksParsesArkgatedResponse(t *testing.T) {
	var gotCmd Arkcmd
	conn := fakeArkgated(t, func(cmd Arkcmd) []byte {
		gotCmd = cmd
		// Exactly the shape arkgated's Arkcommand.Snapshot/ActiveSnapshot
		// marshals - see arkgated's arkcommand/registry.go.
		return []byte(`{"count":2,"total_count":5,"commands":[
			{"id":1,"name":"HealthPing","cmd":"/sbin/ping","remote":"10.0.0.1:5555","running_ms":1500},
			{"id":2,"name":"HealthPing","cmd":"/sbin/ping","remote":"10.0.0.2:5556","running_ms":300}
		]}`)
	})

	snap, err := GetActiveHealthChecks(conn)
	if err != nil {
		t.Fatal(err)
	}
	if gotCmd.Name != "ActiveHealthChecks" {
		t.Errorf("sent command name = %q, want ActiveHealthChecks", gotCmd.Name)
	}
	if snap.Count != 2 || snap.TotalCount != 5 {
		t.Errorf("snap = %+v, want count=2 total=5", snap)
	}
	if len(snap.Commands) != 2 || snap.Commands[0].Remote != "10.0.0.1:5555" {
		t.Errorf("commands = %+v", snap.Commands)
	}
}

func TestClearActiveHealthChecksParsesArkgatedResponse(t *testing.T) {
	var gotCmd Arkcmd
	conn := fakeArkgated(t, func(cmd Arkcmd) []byte {
		gotCmd = cmd
		return []byte(`{"cleared":3}`)
	})

	n, err := ClearActiveHealthChecks(conn)
	if err != nil {
		t.Fatal(err)
	}
	if gotCmd.Name != "ClearHealthChecks" {
		t.Errorf("sent command name = %q, want ClearHealthChecks", gotCmd.Name)
	}
	if n != 3 {
		t.Errorf("cleared = %d, want 3", n)
	}
}

// A nil conn (the documented "dial to arkgated failed" case - see
// utils.GetArkConn in srvcman) must be an ordinary error, never a panic; a
// caller that forgot to check for a dial failure and passed the nil straight
// through must not crash the whole process.
func TestNilConnIsAnErrorNotAPanic(t *testing.T) {
	if _, err := GetActiveHealthChecks(nil); err == nil {
		t.Error("GetActiveHealthChecks(nil) should return an error")
	}
	if _, err := ClearActiveHealthChecks(nil); err == nil {
		t.Error("ClearActiveHealthChecks(nil) should return an error")
	}
}

// A response that isn't the expected shape (arkgated down mid-protocol
// change, a proxy mangling bytes, whatever) must be a clear error, not a
// silently-zeroed result that looks like "nothing is stuck" when the truth is
// "we don't actually know".
func TestMalformedResponseIsAnError(t *testing.T) {
	conn := fakeArkgated(t, func(cmd Arkcmd) []byte { return []byte("not json at all") })
	if _, err := GetActiveHealthChecks(conn); err == nil {
		t.Error("want an error for a malformed ActiveHealthChecks response")
	}

	conn2 := fakeArkgated(t, func(cmd Arkcmd) []byte { return []byte("not json at all") })
	if _, err := ClearActiveHealthChecks(conn2); err == nil {
		t.Error("want an error for a malformed ClearHealthChecks response")
	}
}

// Both functions must close the connection - arkgated closes its end right
// after writing, and a leaked client-side fd on every dashboard poll would
// be its own slow resource leak.
func TestGetActiveHealthChecksClosesConnection(t *testing.T) {
	conn := fakeArkgated(t, func(cmd Arkcmd) []byte { return []byte(`{"count":0,"total_count":0}`) })
	if _, err := GetActiveHealthChecks(conn); err != nil {
		t.Fatal(err)
	}
	// A closed net.Pipe conn errors on any further I/O.
	if _, err := conn.Write([]byte("x")); err != io.ErrClosedPipe {
		t.Errorf("connection was not closed after use: %v", err)
	}
}

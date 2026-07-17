// Package ipc defines the local control protocol between the menu-bar app and
// the daemon, spoken as line-delimited JSON over a Unix domain socket.
package ipc

import (
	"bufio"
	"encoding/json"
	"net"
	"os"
	"path/filepath"
)

// SocketPath is the Unix socket the daemon listens on.
func SocketPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "lanfirst", "daemon.sock")
}

// Command names.
const (
	CmdStatus      = "status"
	CmdEnable      = "enable"
	CmdDisable     = "disable"
	CmdReload      = "reload"
	CmdAddEntry     = "add_entry"
	CmdRemoveEntry  = "remove_entry"
	CmdAddExcept    = "add_except"
	CmdRemoveExcept = "remove_except"
)

// Request is sent by the controller to the daemon. Pattern/Target/Port carry the
// entry data for CmdAddEntry and CmdRemoveEntry (RemoveEntry uses Pattern only).
// Pattern+Except carry the exception data for CmdAddExcept and CmdRemoveExcept.
type Request struct {
	Command string `json:"command"`
	Pattern string `json:"pattern,omitempty"`
	Target  string `json:"target,omitempty"`
	Port    int    `json:"port,omitempty"`
	Except  string `json:"except,omitempty"`
}

// EntryStatus mirrors one resolver entry's live routing decision.
type EntryStatus struct {
	Pattern string   `json:"pattern"`
	Target  string   `json:"target"`
	LAN     bool     `json:"lan"`
	Except  []string `json:"except,omitempty"`
}

// Response is returned by the daemon.
type Response struct {
	OK      bool          `json:"ok"`
	Error   string        `json:"error,omitempty"`
	Enabled bool          `json:"enabled"`
	Version string        `json:"version,omitempty"` // daemon build identity
	Entries []EntryStatus `json:"entries,omitempty"`
}

// Handler processes a request and returns a response.
type Handler func(Request) Response

// Serve listens on the Unix socket and dispatches requests to h until stop is
// closed. It removes any stale socket file first.
func Serve(path string, h Handler, stop <-chan struct{}) error {
	_ = os.Remove(path)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	ln, err := net.Listen("unix", path)
	if err != nil {
		return err
	}
	go func() {
		<-stop
		_ = ln.Close()
		_ = os.Remove(path)
	}()
	for {
		conn, err := ln.Accept()
		if err != nil {
			select {
			case <-stop:
				return nil
			default:
				return err
			}
		}
		go handleConn(conn, h)
	}
}

func handleConn(conn net.Conn, h Handler) {
	defer conn.Close()
	dec := json.NewDecoder(bufio.NewReader(conn))
	enc := json.NewEncoder(conn)
	var req Request
	if err := dec.Decode(&req); err != nil {
		_ = enc.Encode(Response{OK: false, Error: "bad request"})
		return
	}
	_ = enc.Encode(h(req))
}

// Call sends a single bare command to the daemon and returns its response.
func Call(path string, cmd string) (Response, error) {
	return CallRequest(path, Request{Command: cmd})
}

// CallRequest sends a full request (used by commands that carry entry data, like
// CmdAddEntry/CmdRemoveEntry) and returns the daemon's response.
func CallRequest(path string, req Request) (Response, error) {
	conn, err := net.Dial("unix", path)
	if err != nil {
		return Response{}, err
	}
	defer conn.Close()
	if err := json.NewEncoder(conn).Encode(req); err != nil {
		return Response{}, err
	}
	var resp Response
	if err := json.NewDecoder(bufio.NewReader(conn)).Decode(&resp); err != nil {
		return Response{}, err
	}
	return resp, nil
}

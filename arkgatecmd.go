package arkgatecmd

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
)

type Arkcmd struct {
	Name string   `json:"name"`
	Cmd  string   `json:"cmd"`
	Opts []string `json:"opts"`
}

// arkcmdWithOutput is the wire shape SendCmdOutput sends: c's own fields
// plus want_output=true (Go's encoding/json flattens an embedded struct's
// fields into the same JSON object), telling arkgated's worker to reply
// with the command's captured output instead of a bare "OK"/"NOK". Arkcmd
// itself doesn't carry this field, so every existing positional
// Arkcmd{name, cmd, opts} literal in this file (GetPFcmds et al.) stays
// valid - only SendCmdOutput needs it, and only at send time.
type arkcmdWithOutput struct {
	Arkcmd
	WantOutput bool `json:"want_output"`
}

// outputResponse mirrors arkgated's own outputResponse type (main.go):
// {ok, output, error} - what SendCmdOutput unmarshals arkgated's reply
// into.
type outputResponse struct {
	OK     bool   `json:"ok"`
	Output string `json:"output"`
	Error  string `json:"error"`
}

// SendCmd sends c over conn and waits for arkgated's "OK" acknowledgement.
// conn is nil whenever the caller's dial/handshake to arkgated failed (see
// e.g. srvcman's utils.GetArkConn, which logs the failure and returns nil
// rather than an error) - callers pass that straight through as
// c.SendCmd(GetArkConn()), so this has to treat nil as an ordinary failure
// (return an error) rather than dereferencing it, or every caller crashes
// the whole process any time arkgated is briefly unreachable.
func (c *Arkcmd) SendCmd(conn net.Conn) error {
	if conn == nil {
		return errors.New("arkgated: no connection")
	}
	defer conn.Close()
	bufc, _ := json.Marshal(c)
	_, err := conn.Write(bufc)
	if err != nil {
		return err
	}
	buf := make([]byte, 8)
	n, err := conn.Read(buf[:])
	if err != nil {
		return err
	}
	ret := string(buf[0:n])
	if ret != "OK" {
		return errors.New(ret)
	}
	return nil
}

// SendCmdOutput behaves like SendCmd but also returns the command's
// captured output text - use this for commands whose whole point is the
// output (e.g. ping/traceroute diagnostics, ifconfig/netstat interface
// stats) rather than a bare success/failure. It reads the full response via
// io.ReadAll (arkgated closes the connection right after writing its
// reply, so this terminates on EOF) instead of a fixed-size buffer, since
// output can be arbitrarily long - unlike SendCmd, which only ever expects
// a short "OK"/"NOK". As with SendCmd, conn may be nil if the caller's
// dial/handshake to arkgated failed, and that's handled the same way.
func (c *Arkcmd) SendCmdOutput(conn net.Conn) (string, error) {
	if conn == nil {
		return "", errors.New("arkgated: no connection")
	}
	defer conn.Close()
	wire := arkcmdWithOutput{Arkcmd: *c, WantOutput: true}
	bufc, err := json.Marshal(wire)
	if err != nil {
		return "", err
	}
	if _, err := conn.Write(bufc); err != nil {
		return "", err
	}
	raw, err := io.ReadAll(conn)
	if err != nil {
		return "", err
	}
	var resp outputResponse
	if err := json.Unmarshal(raw, &resp); err != nil {
		return "", fmt.Errorf("arkgated: malformed response: %w", err)
	}
	if !resp.OK {
		if resp.Error != "" {
			return resp.Output, errors.New(resp.Error)
		}
		return resp.Output, errors.New("arkgated: command failed")
	}
	return resp.Output, nil
}

func GetPFcmds(run_dir string) map[string]*Arkcmd {
	pfcmds := make(map[string]*Arkcmd)
	pfcmds["check"] = &Arkcmd{"CheckPF", "/sbin/pfctl", []string{"-nf", run_dir + "pf.conf"}}
	pfcmds["backup"] = &Arkcmd{"BackupPF", "/bin/mv", []string{"/etc/pf.conf", "/etc/pf.conf.prev"}}
	pfcmds["move"] = &Arkcmd{"MovePF", "/bin/mv", []string{run_dir + "pf.conf", "/etc/"}}
	pfcmds["apply"] = &Arkcmd{"ApplyPF", "/sbin/pfctl", []string{"-f", "/etc/pf.conf"}}
	pfcmds["revert"] = &Arkcmd{"RevertPF", "/bin/mv", []string{"/etc/pf.conf.prev", "/etc/pf.conf"}}
	return pfcmds
}

func GetServiceCmds(service string, binpath string, confdir string, configfile string) map[string]*Arkcmd {
	cmds := make(map[string]*Arkcmd)
	cmds["check"] = &Arkcmd{"CheckConf_" + service, binpath, []string{"-nf", configfile}}
	// check_unbound is for unbound-checkconf, which takes the config file
	// as a plain positional argument and doesn't accept/need "-nf".
	cmds["check_unbound"] = &Arkcmd{"CheckConf_" + service, binpath, []string{configfile}}
	cmds["backup"] = &Arkcmd{"BackupConf_" + service, "/bin/mv", []string{confdir + service + ".conf", confdir + service + ".conf.prev"}}
	cmds["stage"] = &Arkcmd{"StageConf_" + service, "/bin/mv", []string{configfile, confdir + service + ".conf"}}
	cmds["revert"] = &Arkcmd{"RevertConf_" + service, "/bin/mv", []string{confdir + service + ".conf.prev", confdir + service + ".conf"}}
	return cmds
}

func GetRcCmds(service string) map[string]*Arkcmd {
	cmds := make(map[string]*Arkcmd)
	cmds["reload"] = &Arkcmd{"Reload_" + service, "/usr/sbin/rcctl", []string{"reload", service}}
	cmds["restart"] = &Arkcmd{"Restart_" + service, "/usr/sbin/rcctl", []string{"restart", service}}
	cmds["stop"] = &Arkcmd{"Stop_" + service, "/usr/sbin/rcctl", []string{"stop", service}}
	cmds["start"] = &Arkcmd{"Start_" + service, "/usr/sbin/rcctl", []string{"start", service}}
	cmds["enable"] = &Arkcmd{"Enable_" + service, "/usr/sbin/rcctl", []string{"enable", service}}
	cmds["disable"] = &Arkcmd{"Disable_" + service, "/usr/sbin/rcctl", []string{"disable", service}}
	return cmds
}

func UpdatePFtableCmd(tname string, ip string, action string) *Arkcmd {
	cmd := &Arkcmd{"UpdateTable", "/sbin/pfctl", []string{"-t", tname, "-T", action, ip}}
	return cmd
}

func PrepareCmd(name string, binpath string, opts []string) *Arkcmd {
	cmd := &Arkcmd{name, binpath, opts}
	return cmd
}

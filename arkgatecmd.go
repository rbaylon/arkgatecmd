package arkgatecmd

import (
	"encoding/json"
	"errors"
	"net"
)

type Arkcmd struct {
	Name string   `json:"name"`
	Cmd  string   `json:"cmd"`
	Opts []string `json:"opts"`
}

func (c *Arkcmd) SendCmd(conn net.Conn) error {
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

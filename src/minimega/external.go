// Copyright (2012) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.

package main

import (
	"bytes"
	"errors"
	"fmt"
	"minicli"
	log "minilog"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	MIN_QEMU    = 1.6
	MIN_OVS     = 1.4
	MIN_DNSMASQ = 2.73
)

// externalProcessesLock mediates access to customExternalProcesses.
var externalProcessesLock sync.Mutex

// defaultExternalProcesses is the default mapping between a command and the
// actual binary name. This should *never* be modified. If the user needs to
// update customExternalProcesses.
var defaultExternalProcesses = map[string]string{
	"qemu":     "kvm",
	"ip":       "ip",
	"ovs":      "ovs-vsctl",
	"dnsmasq":  "dnsmasq",
	"kill":     "kill",
	"dhcp":     "dhclient",
	"openflow": "ovs-ofctl",
	"mount":    "mount",
	"umount":   "umount",
	"mkdosfs":  "mkdosfs",
	"qemu-nbd": "qemu-nbd",
	"rm":       "rm",
	"qemu-img": "qemu-img",
	"cp":       "cp",
	"taskset":  "taskset",
	"lsmod":    "lsmod",
	"ntfs-3g":  "ntfs-3g",
	"scp":      "scp",
	"ssh":      "ssh",
	"hostname": "hostname",
	"tc":       "tc",
}

// customExternalProcesses contains user-specified mappings between command
// names. This mapping is checked first before using defaultExternalProcesses
// to resolve a command.
var customExternalProcesses = map[string]string{}

var externalCLIHandlers = []minicli.Handler{
	{ // check
		HelpShort: "check that all external executables dependencies exist",
		HelpLong: `
minimega maintains a list of external packages that it depends on, such as
qemu. Calling check will attempt to find each of these executables in the
avaiable path and check to make sure they meet the minimum version
requirements. Returns errors for all missing executables and all minimum
versions not met.`,
		Patterns: []string{
			"check",
		},
		Call: wrapSimpleCLI(cliCheckExternal),
	},
}

func cliCheckExternal(c *minicli.Command, resp *minicli.Response) error {
	if err := checkExternal(); err != nil {
		return err
	}

	// TODO: Remove? This goes against the unix philosophy
	resp.Response = "all external dependencies met"
	return nil
}

// checkExternal checks for the presence of each of the external processes we
// may call, and error if any aren't in our path.
func checkExternal() error {
	// make sure we have all binaries first
	if err := checkProcesses(); err != nil {
		return err
	}

	// everything we want exists, but we have a few minimum versions to check
	version, err := qemuVersion()
	if err != nil {
		return err
	}

	log.Debug("got kvm version %v", version)
	if version < MIN_QEMU {
		return fmt.Errorf("kvm version %v does not meet minimum version %v", version, MIN_QEMU)
	}

	version, err = ovsVersion()
	if err != nil {
		return err
	}

	log.Debug("got ovs version %v", version)
	if version < MIN_OVS {
		return fmt.Errorf("ovs version %v does not meet minimum version %v", version, MIN_OVS)
	}

	version, err = dnsmasqVersion()
	if err != nil {
		return err
	}

	log.Debug("got dnsmasq version %v", version)
	if version < MIN_DNSMASQ {
		return fmt.Errorf("dnsmasq version %v does not meet minimum version %v", version, MIN_DNSMASQ)
	}

	return nil
}

// checkProcesses checks each of the processes in defaultExternalProcesses exists
func checkProcesses() error {
	externalProcessesLock.Lock()
	defer externalProcessesLock.Unlock()

	var errs []string
	for name, proc := range defaultExternalProcesses {
		if alt, ok := customExternalProcesses[name]; ok {
			proc = alt
		}

		path, err := exec.LookPath(proc)
		if err != nil {
			errs = append(errs, err.Error())
		} else {
			log.Info("%v found at: %v", proc, path)
		}
	}

	if len(errs) > 0 {
		return errors.New(strings.Join(errs, "\n"))
	}

	return nil
}

// processWrapper executes the given arg list and returns a combined
// stdout/stderr and any errors. processWrapper blocks until the process exits.
// Users that need runtime control of processes should use os/exec directly.
func processWrapper(args ...string) (string, error) {
	a := append([]string{}, args...)
	if len(a) == 0 {
		return "", fmt.Errorf("empty argument list")
	}
	p := process(a[0])
	if p == "" {
		return "", fmt.Errorf("cannot find process %v", args[0])
	}

	a[0] = p
	var ea []string
	if len(a) > 1 {
		ea = a[1:]
	}

	start := time.Now()
	out, err := exec.Command(p, ea...).CombinedOutput()
	stop := time.Now()
	log.Debug("cmd %v completed in %v", p, stop.Sub(start))
	return string(out), err
}

func process(p string) string {
	externalProcessesLock.Lock()
	defer externalProcessesLock.Unlock()

	name, ok := customExternalProcesses[p]
	if !ok {
		name = defaultExternalProcesses[p]
	}

	path, err := exec.LookPath(name)
	if err != nil {
		log.Error("process: %v", err)
		return ""
	}
	return path
}

func dnsmasqVersion() (float64, error) {
	var sOut bytes.Buffer
	var sErr bytes.Buffer
	p := process("dnsmasq")
	cmd := &exec.Cmd{
		Path: p,
		Args: []string{
			p,
			"-v",
		},
		Env:    nil,
		Dir:    "",
		Stdout: &sOut,
		Stderr: &sErr,
	}

	log.Debug("checking dnsmasq version with cmd: %v", cmd)
	if err := cmd.Run(); err != nil {
		return 0.0, fmt.Errorf("error checking dnsmasq version: %v %v", err, sErr.String())
	}

	f := strings.Fields(sOut.String())
	if len(f) < 3 {
		return 0.0, fmt.Errorf("cannot parse dnsmasq version: %v", sOut.String())
	}

	dnsmasqVersionFields := strings.Split(f[2], ".")
	if len(dnsmasqVersionFields) < 2 {
		return 0.0, fmt.Errorf("cannot parse dnsmasq version: %v", sOut.String())
	}

	log.Debugln(dnsmasqVersionFields)
	dnsmasqVersion, err := strconv.ParseFloat(strings.Join(dnsmasqVersionFields[:2], "."), 64)
	if err != nil {
		return 0.0, fmt.Errorf("cannot parse dnsmasq version: %v %v", sOut.String(), err)
	}

	return dnsmasqVersion, nil
}

func qemuVersion() (float64, error) {
	var sOut bytes.Buffer
	var sErr bytes.Buffer
	p := process("qemu")
	cmd := &exec.Cmd{
		Path: p,
		Args: []string{
			p,
			"-version",
		},
		Env:    nil,
		Dir:    "",
		Stdout: &sOut,
		Stderr: &sErr,
	}

	log.Debug("checking qemu version with cmd: %v", cmd)
	if err := cmd.Run(); err != nil {
		return 0.0, fmt.Errorf("error checking kvm version: %v %v", err, sErr.String())
	}

	f := strings.Fields(sOut.String())
	if len(f) < 4 {
		return 0.0, fmt.Errorf("cannot parse kvm version: %v", sOut.String())
	}

	qemuVersionFields := strings.Split(f[3], ".")
	if len(qemuVersionFields) < 2 {
		return 0.0, fmt.Errorf("cannot parse kvm version: %v", sOut.String())
	}

	log.Debugln(qemuVersionFields)
	qemuVersion, err := strconv.ParseFloat(strings.Join(qemuVersionFields[:2], "."), 64)
	if err != nil {
		return 0.0, fmt.Errorf("cannot parse kvm version: %v %v", sOut.String(), err)
	}

	return qemuVersion, nil
}

func ovsVersion() (float64, error) {
	var sOut bytes.Buffer
	var sErr bytes.Buffer
	p := process("ovs")
	cmd := &exec.Cmd{
		Path: p,
		Args: []string{
			p,
			"-V",
		},
		Env:    nil,
		Dir:    "",
		Stdout: &sOut,
		Stderr: &sErr,
	}

	log.Debug("checking ovs version with cmd: %v", cmd)
	if err := cmd.Run(); err != nil {
		return 0.0, fmt.Errorf("checking ovs version: %v %v", err, sErr.String())
	}

	f := strings.Fields(sOut.String())
	if len(f) < 4 {
		return 0.0, fmt.Errorf("cannot parse ovs version: %v", sOut.String())
	}

	ovsVersionFields := strings.Split(f[3], ".")
	if len(ovsVersionFields) < 2 {
		return 0.0, fmt.Errorf("cannot parse ovs version: %v", sOut.String())
	}

	log.Debugln(ovsVersionFields)
	ovsVersion, err := strconv.ParseFloat(strings.Join(ovsVersionFields[:2], "."), 64)
	if err != nil {
		return 0.0, fmt.Errorf("cannot parse ovs version: %v %v", sOut.String(), err)
	}

	return ovsVersion, nil
}

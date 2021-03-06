// Copyright (2015) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.

package main

import (
	"encoding/gob"
	"errors"
	"fmt"
	"io/ioutil"
	"minicli"
	log "minilog"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

var vmCLIHandlers = []minicli.Handler{
	{ // vm info
		HelpShort: "print information about VMs",
		HelpLong: `
Print information about VMs in tabular form. The .filter and .columns commands
can be used to subselect a set of rows and/or columns. See the help pages for
.filter and .columns, respectively, for their usage. Columns returned by VM
info include:

- host       : the host that the VM is running on
- id         : the VM ID, as an integer
- name       : the VM name, if it exists
- state      : one of (building, running, paused, quit, error)
- type       : one of (kvm)
- vcpus      : the number of allocated CPUs
- memory     : allocated memory, in megabytes
- vlan       : vlan, as an integer
- bridge     : bridge name
- tap        : tap name
- mac        : mac address
- ip         : IPv4 address
- ip6        : IPv6 address
- bandwidth  : stats regarding bandwidth usage
- tags       : any additional information attached to the VM
- uuid       : QEMU system uuid
- cc_active  : whether cc is active

Additional fields are available for KVM-based VMs:

- append     : kernel command line string
- cdrom      : cdrom image
- disk       : disk image
- kernel     : kernel image
- initrd     : initrd image
- migrate    : qemu migration image

Additional fields are available for container-based VMs:

- init	     : process to invoke as init
- preinit    : process to invoke at container launch before isolation
- filesystem : root filesystem for the container

Examples:

Display a list of all IPs for all VMs:
	.columns ip,ip6 vm info

Display information about all VMs:
	vm info`,
		Patterns: []string{
			"vm info",
		},
		Call: wrapBroadcastCLI(cliVmInfo),
	},
	{ // vm summary
		HelpShort: "print summary information about VMs",
		HelpLong: `
Simpler version of "vm info" -- same meanings but fewer columns. `,
		Patterns: []string{
			"vm summary",
		},
		Call: wrapBroadcastCLI(cliVmSummary),
	},
	{ // vm launch
		HelpShort: "launch virtual machines in a paused state",
		HelpLong: fmt.Sprintf(`
Launch virtual machines in a paused state, using the parameters defined leading
up to the launch command. Any changes to the VM parameters after launching will
have no effect on launched VMs.

When you launch a VM, you supply the type of VM in the launch command.
Currently, the supported VM types are:

- kvm : QEMU-based vms
- container: Linux containers

If you supply a name instead of a number of VMs, one VM with that name will be
launched. You may also supply a range expression to launch VMs with a specific
naming scheme:

	vm launch kvm foo[0-9]

Note: VM names cannot be integers or reserved words (e.g. "%[1]s").

The optional 'noblock' suffix forces minimega to return control of the command
line immediately instead of waiting on potential errors from launching the
VM(s). The user must check logs or error states from vm info.

The launch behavior changes when namespace are active. If a namespace is
active, invocations that include the VM type and the name or number of VMs will
be queue until a subsequent invocation that does not include any arguments.
This allows the scheduler to better allocate resources across the cluster. The
'noblock' suffix is ignored when namespaces are active.`, Wildcard),
		Patterns: []string{
			"vm launch",
			"vm launch <kvm,> <name or count> [noblock,]",
			"vm launch <container,> <name or count> [noblock,]",
		},
		Call: wrapSimpleCLI(cliVmLaunch),
	},
	{ // vm kill
		HelpShort: "kill running virtual machines",
		HelpLong: `
Kill one or more running virtual machines. See "vm start" for a full
description of allowable targets.`,
		Patterns: []string{
			"vm kill <target>",
		},
		Call: wrapVMTargetCLI(cliVmKill),
		Suggest: func(val, prefix string) []string {
			if val == "target" {
				return cliVMSuggest(prefix, VM_ANY_STATE)
			} else {
				return nil
			}
		},
	},
	{ // vm start
		HelpShort: "start paused virtual machines",
		HelpLong: fmt.Sprintf(`
Start one or more paused virtual machines. VMs may be selected by name, ID, range, or
wildcard. For example,

To start vm foo:

		vm start foo

To start vms foo and bar:

		vm start foo,bar

To start vms foo0, foo1, foo2, and foo5:

		vm start foo[0-2,5]

VMs can also be specified by ID, such as:

		vm start 0

Or, a range of IDs:

		vm start [2-4,6]

There is also a wildcard (%[1]s) which allows the user to specify all VMs:

		vm start %[1]s

Note that including the wildcard in a list of VMs results in the wildcard
behavior (although a message will be logged).

Calling "vm start" on a specific list of VMs will cause them to be started if
they are in the building, paused, quit, or error states. When used with the
wildcard, only vms in the building or paused state will be started.`, Wildcard),
		Patterns: []string{
			"vm start <target>",
		},
		Call: wrapVMTargetCLI(cliVmStart),
		Suggest: func(val, prefix string) []string {
			if val == "target" {
				return cliVMSuggest(prefix, ^VM_RUNNING)
			} else {
				return nil
			}
		},
	},
	{ // vm stop
		HelpShort: "stop/pause virtual machines",
		HelpLong: `
Stop one or more running virtual machines. See "vm start" for a full
description of allowable targets.

Calling stop will put VMs in a paused state. Use "vm start" to restart them.`,
		Patterns: []string{
			"vm stop <target>",
		},
		Call: wrapVMTargetCLI(cliVmStop),
		Suggest: func(val, prefix string) []string {
			if val == "target" {
				return cliVMSuggest(prefix, VM_RUNNING)
			} else {
				return nil
			}
		},
	},
	{ // vm flush
		HelpShort: "discard information about quit or failed VMs",
		HelpLong: `
Discard information about VMs that have either quit or encountered an error.
This will remove any VMs with a state of "quit" or "error" from vm info. Names
of VMs that have been flushed may be reused.`,
		Patterns: []string{
			"vm flush",
		},
		Call: wrapBroadcastCLI(cliVmFlush),
	},
	{ // vm hotplug
		HelpShort: "add and remove USB drives",
		HelpLong: `
Add and remove USB drives to a launched VM.

To view currently attached media, call vm hotplug with the 'show' argument and
a VM ID or name. To add a device, use the 'add' argument followed by the VM ID
or name, and the name of the file to add. For example, to add foo.img to VM 5:

	vm hotplug add 5 foo.img

The add command will assign a disk ID, shown in vm hotplug show. To remove
media, use the 'remove' argument with the VM ID and the disk ID. For example,
to remove the drive added above, named 0:

	vm hotplug remove 5 0

To remove all hotplug devices, use ID "all" for the disk ID.`,
		Patterns: []string{
			"vm hotplug <show,> <vm id or name>",
			"vm hotplug <add,> <vm id or name> <filename>",
			"vm hotplug <remove,> <vm id or name> <disk id or all>",
		},
		Call: wrapVMTargetCLI(cliVmHotplug),
		Suggest: func(val, prefix string) []string {
			if val == "vm" {
				return cliVMSuggest(prefix, VM_ANY_STATE)
			} else {
				return nil
			}
		},
	},
	{ // vm net
		HelpShort: "disconnect or move network connections",
		HelpLong: `
Disconnect or move existing network connections on a running VM.

Network connections are indicated by their position in vm net (same order in vm
info) and are zero indexed. For example, to disconnect the first network
connection from a VM named vm-0 with 4 network connections:

	vm net disconnect vm-0 0

To disconnect the second connection:

	vm net disconnect vm-0 1

To move a connection, specify the new VLAN tag and bridge:

	vm net <vm name or id> 0 bridgeX 100`,
		Patterns: []string{
			"vm net <connect,> <vm id or name> <tap position> <bridge> <vlan>",
			"vm net <disconnect,> <vm id or name> <tap position>",
		},
		Call: wrapSimpleCLI(cliVmNetMod),
		Suggest: func(val, prefix string) []string {
			if val == "vm" {
				return cliVMSuggest(prefix, VM_ANY_STATE)
			} else if val == "vlan" {
				return suggestVLAN(prefix)
			} else {
				return nil
			}
		},
	},
	{ // vm qmp
		HelpShort: "issue a JSON-encoded QMP command",
		HelpLong: `
Issue a JSON-encoded QMP command. This is a convenience function for accessing
the QMP socket of a VM via minimega. vm qmp takes two arguments, a VM ID or
name, and a JSON string, and returns the JSON encoded response. For example:

	vm qmp 0 '{ "execute": "query-status" }'
	{"return":{"running":false,"singlestep":false,"status":"prelaunch"}}`,
		Patterns: []string{
			"vm qmp <vm id or name> <qmp command>",
		},
		Call: wrapVMTargetCLI(cliVmQmp),
		Suggest: func(val, prefix string) []string {
			if val == "vm" {
				return cliVMSuggest(prefix, VM_ANY_STATE)
			} else {
				return nil
			}
		},
	},
	{ // vm screenshot
		HelpShort: "take a screenshot of a running vm",
		HelpLong: `
Take a screenshot of the framebuffer of a running VM. The screenshot is saved
in PNG format as "screenshot.png" in the VM's runtime directory (by default
/tmp/minimega/<vm id>/screenshot.png).

An optional argument sets the maximum dimensions in pixels, while keeping the
aspect ratio. For example, to set either maximum dimension of the output image
to 100 pixels:

	vm screenshot foo 100

The screenshot can be saved elsewhere like this:

        vm screenshot foo file /tmp/foo.png

You can also specify the maximum dimension:

        vm screenshot foo file /tmp/foo.png 100`,
		Patterns: []string{
			"vm screenshot <vm id or name> [maximum dimension]",
			"vm screenshot <vm id or name> file <filename> [maximum dimension]",
		},
		Call: wrapVMTargetCLI(cliVmScreenshot),
		Suggest: func(val, prefix string) []string {
			if val == "vm" {
				return cliVMSuggest(prefix, VM_ANY_STATE)
			} else {
				return nil
			}
		},
	},
	{ // vm migrate
		HelpShort: "write VM state to disk",
		HelpLong: `
Migrate runtime state of a VM to disk, which can later be booted with vm config
migrate.

Migration files are written to the files directory as specified with -filepath.
On success, a call to migrate a VM will return immediately. You can check the
status of in-flight migrations by invoking vm migrate with no arguments.`,
		Patterns: []string{
			"vm migrate",
			"vm migrate <vm id or name> <filename>",
		},
		Call: wrapVMTargetCLI(cliVmMigrate),
		Suggest: func(val, prefix string) []string {
			if val == "vm" {
				return cliVMSuggest(prefix, VM_ANY_STATE)
			} else {
				return nil
			}
		},
	},
	{ // vm cdrom
		HelpShort: "eject or change an active VM's cdrom",
		HelpLong: `
Eject or change an active VM's cdrom image.

Eject VM 0's cdrom:

        vm cdrom eject 0

Eject all VM cdroms:

        vm cdrom eject all

Change a VM to use a new ISO:

        vm cdrom change 0 /tmp/debian.iso

"vm cdrom change" implies that the current ISO will be ejected.`,
		Patterns: []string{
			"vm cdrom <eject,> <vm id or name>",
			"vm cdrom <change,> <vm id or name> <path>",
		},
		Call: wrapVMTargetCLI(cliVmCdrom),
		Suggest: func(val, prefix string) []string {
			if val == "vm" {
				return cliVMSuggest(prefix, VM_ANY_STATE)
			} else {
				return nil
			}
		},
	},
	{ // vm tag
		HelpShort: "display or set a tag for the specified VM",
		HelpLong: `
Display or set a tag for one or more virtual machines. See "vm start" for a
full description of allowable targets.

Tags are key-value pairs. A VM can have any number of tags associated with it.
They can be used to attach additional information to a virtual machine, for
example specifying a VM "group", or the correct rendering color for some
external visualization tool.

To set a tag "foo" to "bar" for VM 2:

        vm tag 2 foo bar

To read a tag:

        vm tag <target> <key or all>`,
		Patterns: []string{
			"vm tag <target> [key or all]",  // get
			"vm tag <target> <key> <value>", // set
		},
		Call: wrapVMTargetCLI(cliVmTag),
		Suggest: func(val, prefix string) []string {
			if val == "target" {
				return cliVMSuggest(prefix, VM_ANY_STATE)
			} else {
				return nil
			}
		},
	},
	{ // clear vm tag
		HelpShort: "remove tags from a VM",
		HelpLong: `
Clears one, many, or all tags from a virtual machine.

Clear the tag "foo" from VM 0:

        clear vm tag 0 foo

Clear the tag "foo" from all VMs:

        clear vm tag all foo

Clear all tags from VM 0:

        clear vm tag 0

Clear all tags from all VMs:

        clear vm tag all`,
		Patterns: []string{
			"clear vm tag",
			"clear vm tag <target> [tag]",
		},
		Call: wrapVMTargetCLI(cliClearVmTag),
		Suggest: func(val, prefix string) []string {
			if val == "target" {
				return cliVMSuggest(prefix, VM_ANY_STATE)
			} else {
				return nil
			}
		},
	},
}

func init() {
	// Register these so we can serialize the VMs
	gob.Register(VMs{})
	gob.Register(&KvmVM{})
	gob.Register(&ContainerVM{})
}

func cliVmStart(c *minicli.Command, resp *minicli.Response) error {
	return makeErrSlice(vms.Start(c.StringArgs["target"]))
}

func cliVmStop(c *minicli.Command, resp *minicli.Response) error {
	return makeErrSlice(vms.Stop(c.StringArgs["target"]))
}

func cliVmKill(c *minicli.Command, resp *minicli.Response) error {
	return makeErrSlice(vms.Kill(c.StringArgs["target"]))
}

func cliVmInfo(c *minicli.Command, resp *minicli.Response) error {
	vms.Info(vmInfo, resp)

	return nil
}

func cliVmSummary(c *minicli.Command, resp *minicli.Response) error {
	vms.Info(vmInfoLite, resp)

	return nil
}

func cliVmCdrom(c *minicli.Command, resp *minicli.Response) error {
	arg := c.StringArgs["vm"]

	doVms := make([]*KvmVM, 0)

	if arg == Wildcard {
		doVms = vms.FindKvmVMs()
	} else {
		vm, err := vms.FindKvmVM(arg)
		if err != nil {
			return err
		}

		doVms = append(doVms, vm)
	}

	if c.BoolArgs["eject"] {
		for _, v := range doVms {
			err := v.q.BlockdevEject("ide0-cd1")
			v.CdromPath = ""
			if err != nil {
				return err
			}
		}
	} else if c.BoolArgs["change"] {
		for _, v := range doVms {
			// First eject it, then change it
			err := v.q.BlockdevEject("ide0-cd1")
			v.CdromPath = ""
			if err != nil {
				return err
			}

			err = v.q.BlockdevChange("ide0-cd1", c.StringArgs["path"])
			if err != nil {
				return err
			}
			v.CdromPath = c.StringArgs["path"]
		}
	}

	return nil
}

func cliVmTag(c *minicli.Command, resp *minicli.Response) error {
	target := c.StringArgs["target"]

	key := c.StringArgs["key"]
	if key == "" {
		// If they didn't specify a key then they probably want all the tags
		// for a given VM
		key = Wildcard
	}

	value, write := c.StringArgs["value"]
	if write {
		if key == Wildcard {
			return errors.New("cannot assign to wildcard")
		}

		vms.SetTag(target, key, value)

		return nil
	}

	if key == Wildcard {
		resp.Header = []string{"ID", "Tag", "Value"}
	} else {
		resp.Header = []string{"ID", "Value"}
	}

	for _, tag := range vms.GetTags(target, key) {
		row := []string{strconv.Itoa(tag.ID)}

		if key == Wildcard {
			row = append(row, tag.Key)
		}

		row = append(row, tag.Value)
		resp.Tabular = append(resp.Tabular, row)
	}

	return nil
}

func cliClearVmTag(c *minicli.Command, resp *minicli.Response) error {
	// Get the specified tag name or use Wildcard if not provided
	key, ok := c.StringArgs["key"]
	if !ok {
		key = Wildcard
	}

	// Get the specified VM target or use Wildcard if not provided
	target, ok := c.StringArgs["target"]
	if !ok {
		target = Wildcard
	}

	vms.ClearTags(target, key)

	return nil
}

func cliVmLaunch(c *minicli.Command, resp *minicli.Response) error {
	ns := GetNamespace()

	if ns == nil && len(c.StringArgs) == 0 {
		return errors.New("invalid command when namespace is not active")
	}

	// see note in wrapBroadcastCLI
	if ns != nil && c.Source != ns.Name {
		if len(c.StringArgs) > 0 {
			arg := c.StringArgs["name"]

			vmType, err := findVMType(c.BoolArgs)
			if err != nil {
				return err
			}

			return ns.Queue(arg, vmType)
		}

		return ns.Launch()
	}

	// expand the names to launch, scheduler should have ensured that they were
	// globally unique.
	names, err := ExpandLaunchNames(c.StringArgs["name"], vms)
	if err != nil {
		return err
	}

	if len(names) > 1 && vmConfig.UUID != "" {
		return errors.New("cannot launch multiple VMs with a pre-configured UUID")
	}

	for i, name := range names {
		if isReserved(name) {
			return fmt.Errorf("`%s` is a reserved word -- cannot use for vm name", name)
		}

		if _, err := strconv.Atoi(name); err == nil {
			return fmt.Errorf("`%s` is an integer -- cannot use for vm name", name)
		}

		// Check for conflicts within the provided names. Don't conflict with
		// ourselves or if the name is unspecified.
		for j, name2 := range names {
			if i != j && name == name2 && name != "" {
				return fmt.Errorf("`%s` is specified twice in VMs to launch", name)
			}
		}
	}

	noblock := c.BoolArgs["noblock"]

	vmType, err := findVMType(c.BoolArgs)
	if err != nil {
		return err
	}

	errChan := vms.Launch(names, vmType)

	// Collect all the errors from errChan and turn them into a string
	collectErrs := func() error {
		errs := []error{}
		for err := range errChan {
			errs = append(errs, err)
		}

		return makeErrSlice(errs)
	}

	if noblock {
		go func() {
			if err := collectErrs(); err != nil {
				log.Errorln(err)
			}
		}()

		return nil
	}

	return collectErrs()
}

func cliVmFlush(c *minicli.Command, resp *minicli.Response) error {
	vms.Flush()

	return nil
}

func cliVmQmp(c *minicli.Command, resp *minicli.Response) error {
	vm, err := vms.FindKvmVM(c.StringArgs["vm"])
	if err != nil {
		return err
	}

	out, err := vm.QMPRaw(c.StringArgs["qmp"])
	if err != nil {
		return err
	}

	resp.Response = out
	return nil
}

func cliVmScreenshot(c *minicli.Command, resp *minicli.Response) error {
	file := c.StringArgs["filename"]

	var max int

	if arg := c.StringArgs["maximum"]; arg != "" {
		v, err := strconv.Atoi(arg)
		if err != nil {
			return err
		}
		max = v
	}

	vm := vms.FindVM(c.StringArgs["vm"])
	if vm == nil {
		return vmNotFound(c.StringArgs["vm"])
	}

	data, err := vm.Screenshot(max)
	if err != nil {
		return err
	}

	path := filepath.Join(*f_base, strconv.Itoa(vm.GetID()), "screenshot.png")
	if file != "" {
		path = file
	}

	// add user data in case this is going across meshage
	err = ioutil.WriteFile(path, data, os.FileMode(0644))
	if err != nil {
		return err
	}

	resp.Data = data

	return nil
}

func cliVmMigrate(c *minicli.Command, resp *minicli.Response) error {
	if _, ok := c.StringArgs["vm"]; !ok { // report current migrations
		resp.Header = []string{"id", "name", "status", "%% complete"}

		for _, vm := range vms.FindKvmVMs() {
			status, complete, err := vm.QueryMigrate()
			if err != nil {
				return err
			}
			if status == "" {
				continue
			}

			resp.Tabular = append(resp.Tabular, []string{
				fmt.Sprintf("%v", vm.GetID()),
				vm.GetName(),
				status,
				fmt.Sprintf("%.2f", complete)})
		}

		return nil
	}

	vm, err := vms.FindKvmVM(c.StringArgs["vm"])
	if err != nil {
		return err
	}

	return vm.Migrate(c.StringArgs["filename"])
}

func cliVmHotplug(c *minicli.Command, resp *minicli.Response) error {
	vm, err := vms.FindKvmVM(c.StringArgs["vm"])
	if err != nil {
		return err
	}

	if c.BoolArgs["add"] {
		// generate an id by adding 1 to the highest in the list for the
		// hotplug devices, 0 if it's empty
		id := 0
		for k, _ := range vm.hotplug {
			if k >= id {
				id = k + 1
			}
		}
		hid := fmt.Sprintf("hotplug%v", id)
		log.Debugln("hotplug generated id:", hid)

		r, err := vm.q.DriveAdd(hid, c.StringArgs["filename"])
		if err != nil {
			return err
		}

		log.Debugln("hotplug drive_add response:", r)
		r, err = vm.q.USBDeviceAdd(hid)
		if err != nil {
			return err
		}

		log.Debugln("hotplug usb device add response:", r)
		vm.hotplug[id] = c.StringArgs["filename"]

		return nil
	} else if c.BoolArgs["remove"] {
		if c.StringArgs["disk"] == Wildcard {
			for k := range vm.hotplug {
				if err := vm.hotplugRemove(k); err != nil {
					return err
				}
			}

			return nil
		}

		id, err := strconv.Atoi(c.StringArgs["disk"])
		if err != nil {
			return err
		}

		return vm.hotplugRemove(id)
	}

	// must be "show"
	resp.Header = []string{"hotplug ID", "File"}
	resp.Tabular = [][]string{}

	for k, v := range vm.hotplug {
		resp.Tabular = append(resp.Tabular, []string{strconv.Itoa(k), v})
	}

	return nil
}

func cliVmNetMod(c *minicli.Command, resp *minicli.Response) error {
	vm := vms.FindVM(c.StringArgs["vm"])
	if vm == nil {
		return vmNotFound(c.StringArgs["vm"])
	}

	pos, err := strconv.Atoi(c.StringArgs["tap"])
	if err != nil {
		return err
	}

	if c.BoolArgs["disconnect"] {
		return vm.NetworkDisconnect(pos)
	}

	vlan, err := lookupVLAN(c.StringArgs["vlan"])
	if err != nil {
		return err
	}

	return vm.NetworkConnect(pos, c.StringArgs["bridge"], vlan)
}

// cliVMSuggest takes a prefix that could be the start of a VM name or a VM ID
// and makes suggestions for VM names (or IDs) that have a common prefix. mask
// can be used to only complete for VMs that are in a particular state (e.g.
// running). Returns a list of suggestions.
func cliVMSuggest(prefix string, mask VMState) []string {
	var isID bool
	res := []string{}

	if _, err := strconv.Atoi(prefix); err == nil {
		isID = true
	}

	for _, vm := range GlobalVMs() {
		if vm.GetState()&mask == 0 {
			continue
		}

		if isID {
			id := strconv.Itoa(vm.GetID())

			if strings.HasPrefix(id, prefix) {
				res = append(res, id)
			}
		} else if strings.HasPrefix(vm.GetName(), prefix) {
			res = append(res, vm.GetName())
		}
	}

	return res
}

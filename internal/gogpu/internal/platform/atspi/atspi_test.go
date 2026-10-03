//go:build linux

package atspi

import (
	"bufio"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/godbus/dbus/v5"

	"github.com/minelifes/nectar_ui/internal/gogpu/internal/platform/axtypes"
)

// privateBus starts a dbus-daemon for the test and returns its address.
func privateBus(t *testing.T) string {
	t.Helper()
	if _, err := exec.LookPath("dbus-daemon"); err != nil {
		t.Skip("dbus-daemon not installed")
	}
	cmd := exec.Command("dbus-daemon", "--session", "--nofork", "--print-address=1")
	out, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Skip("dbus-daemon:", err)
	}
	t.Cleanup(func() { cmd.Process.Kill(); cmd.Wait() })
	line, err := bufio.NewReader(out).ReadString('\n')
	if err != nil {
		t.Fatal(err)
	}
	return strings.TrimSpace(line)
}

type registry struct{ embedded []ref }

func (r *registry) Embed(plug ref) (ref, *dbus.Error) {
	r.embedded = append(r.embedded, plug)
	return ref{"org.a11y.atspi.Registry", rootPath}, nil
}

func TestBridge(t *testing.T) {
	addr := privateBus(t)
	t.Setenv("AT_SPI_BUS_ADDRESS", addr)

	// A stand-in for the registry, and an assistive technology client.
	reg, err := dbus.Connect(addr)
	if err != nil {
		t.Fatal(err)
	}
	defer reg.Close()
	fake := &registry{}
	if err := reg.Export(fake, rootPath, "org.a11y.atspi.Socket"); err != nil {
		t.Fatal(err)
	}
	if _, err := reg.RequestName("org.a11y.atspi.Registry", 0); err != nil {
		t.Fatal(err)
	}
	at, err := dbus.Connect(addr)
	if err != nil {
		t.Fatal(err)
	}
	defer at.Close()
	if err := at.AddMatchSignal(dbus.WithMatchInterface(ifaceEventObject)); err != nil {
		t.Fatal(err)
	}
	events := make(chan *dbus.Signal, 32)
	at.Signal(events)

	a, err := BusAddress("")
	if err != nil || a != addr {
		t.Fatalf("bus address %q, %v", a, err)
	}
	b, err := Connect(a, "nectar")
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()
	if len(fake.embedded) != 1 || fake.embedded[0].Path != rootPath || b.parent.Name != "org.a11y.atspi.Registry" {
		t.Fatalf("embed %+v, parent %+v", fake.embedded, b.parent)
	}

	pressed := make(chan uint64, 1)
	tree := &axtypes.Tree{Title: "Demo", Width: 400, Height: 300, Roots: []uint64{1}, Nodes: map[uint64]*axtypes.Node{
		1: {ID: 1, Role: "group", W: 400, H: 300, Children: []uint64{2, 3}},
		2: {ID: 2, Role: "button", Name: "OK", X: 10, Y: 20, W: 80, H: 30, Actionable: true},
		3: {ID: 3, Role: "checkbox", Name: "Wrap", X: 10, Y: 60, W: 80, H: 20, Checkable: true, Actionable: true},
	}}
	b.SetWindow(7, tree, func(id uint64) { pressed <- id })

	app := at.Object(b.UniqueName(), rootPath)
	var kids []ref
	if err := app.Call(ifaceAccessible+".GetChildren", 0).Store(&kids); err != nil || len(kids) != 1 || kids[0].Path != framePath(7) {
		t.Fatalf("app children %+v, %v", kids, err)
	}
	var role uint32
	frame := at.Object(b.UniqueName(), kids[0].Path)
	if err := frame.Call(ifaceAccessible+".GetRole", 0).Store(&role); err != nil || role != roleFrame {
		t.Fatalf("frame role %d, %v", role, err)
	}
	name, err := frame.GetProperty(ifaceAccessible + ".Name")
	if err != nil || name.Value() != "Demo" {
		t.Fatalf("frame name %v, %v", name, err)
	}

	ok := at.Object(b.UniqueName(), nodePath(2))
	if err := ok.Call(ifaceAccessible+".GetRole", 0).Store(&role); err != nil || role != rolePushButton {
		t.Fatalf("button role %d, %v", role, err)
	}
	var idx int32
	if err := ok.Call(ifaceAccessible+".GetIndexInParent", 0).Store(&idx); err != nil || idx != 0 {
		t.Fatalf("index %d, %v", idx, err)
	}
	parent, err := ok.GetProperty(ifaceAccessible + ".Parent")
	if err != nil {
		t.Fatal(err)
	}
	var pr ref
	if err := dbus.Store([]any{parent.Value()}, &pr); err != nil || pr.Path != nodePath(1) {
		t.Fatalf("parent %v, %v", parent, err)
	}
	var ext struct{ X, Y, W, H int32 }
	if err := ok.Call(ifaceComponent+".GetExtents", 0, uint32(1)).Store(&ext); err != nil || ext.X != 10 || ext.W != 80 {
		t.Fatalf("extents %+v, %v", ext, err)
	}
	var hit ref
	if err := frame.Call(ifaceComponent+".GetAccessibleAtPoint", 0, int32(15), int32(65), uint32(1)).Store(&hit); err != nil || hit.Path != nodePath(3) {
		t.Fatalf("hit %+v, %v", hit, err)
	}
	var did bool
	if err := ok.Call(ifaceAction+".DoAction", 0, int32(0)).Store(&did); err != nil || !did {
		t.Fatalf("action %v, %v", did, err)
	}
	select {
	case id := <-pressed:
		if id != 2 {
			t.Fatalf("pressed %d", id)
		}
	case <-time.After(time.Second):
		t.Fatal("action not delivered")
	}

	// Checking the box and moving focus are announced.
	drain(events)
	next := *tree
	next.Nodes = map[uint64]*axtypes.Node{}
	for id, n := range tree.Nodes {
		c := *n
		next.Nodes[id] = &c
	}
	next.Nodes[3].Checked, next.Nodes[3].Focused, next.Focus = true, true, 3
	b.SetWindow(7, &next, func(uint64) {})
	got := map[string]bool{}
	timeout := time.After(time.Second)
	for len(got) < 2 {
		select {
		case s := <-events:
			got[s.Name+":"+s.Body[0].(string)+":"+string(s.Path)] = true
		case <-timeout:
			t.Fatalf("events %v", got)
		}
	}
	if !got[ifaceEventObject+".StateChanged:checked:"+string(nodePath(3))] || !got[ifaceEventObject+".StateChanged:focused:"+string(nodePath(3))] {
		t.Fatalf("events %v", got)
	}
	var st []uint32
	box := at.Object(b.UniqueName(), nodePath(3))
	if err := box.Call(ifaceAccessible+".GetState", 0).Store(&st); err != nil || st[0]&(1<<stateChecked) == 0 || st[0]&(1<<stateFocused) == 0 || st[1]&(1<<(stateCheckable-32)) == 0 {
		t.Fatalf("state %v, %v", st, err)
	}

	b.SetWindow(7, nil, nil)
	if err := app.Call(ifaceAccessible+".GetChildren", 0).Store(&kids); err != nil || len(kids) != 0 {
		t.Fatalf("after removal %+v, %v", kids, err)
	}
}

func drain(c chan *dbus.Signal) {
	for {
		select {
		case <-c:
		default:
			return
		}
	}
}

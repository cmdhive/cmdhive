package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// A device group in a temp folder: AIT's own store plus somewhere to act as
// the user's cloud drive.
func deviceApp(t *testing.T) (*App, string, string) {
	t.Helper()
	root, home := t.TempDir(), t.TempDir()
	mem := filepath.Join(t.TempDir(), "memory")
	os.MkdirAll(mem, 0o755)
	store, err := newStoreAt(root, home)
	if err != nil {
		t.Fatal(err)
	}
	c := store.Config()
	c.MemoryDir = mem
	c.DeviceName = "Desktop"
	store.saveConfig(c)
	app := NewApp(store)
	app.emit = func(string, ...any) {}
	return app, mem, filepath.Join(t.TempDir(), "Cloud Drive")
}

// writePeer is another device already in the shared folder.
func writePeer(t *testing.T, shared, id, name string, files map[string]string) {
	t.Helper()
	dir := filepath.Join(shared, "devices", id)
	os.MkdirAll(filepath.Join(dir, "memory"), 0o755)
	b, _ := json.Marshal(Device{ID: id, Name: name, OS: "Mac", LastSeen: time.Now().Unix()})
	if err := os.WriteFile(filepath.Join(dir, deviceFile), b, 0o644); err != nil {
		t.Fatal(err)
	}
	for n, body := range files {
		if err := os.WriteFile(filepath.Join(dir, "memory", n), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func connect(t *testing.T, app *App, drive string) string {
	t.Helper()
	os.MkdirAll(drive, 0o755)
	if _, err := app.ConnectDevices(drive); err != nil {
		t.Fatal(err)
	}
	return app.store.syncDir()
}

// One device on its own: its memory folder is left exactly as it was, and the
// AIs are told nothing about devices.
func TestOneDeviceChangesNothing(t *testing.T) {
	app, mem, drive := deviceApp(t)
	os.WriteFile(filepath.Join(mem, "MEMORY.md"), []byte("# Memory\n- [A](a.md)\n"), 0o644)
	os.WriteFile(filepath.Join(mem, "a.md"), []byte("fact a"), 0o644)
	shared := connect(t, app, drive)

	names := dirNames(t, mem)
	if len(names) != 2 || !names["MEMORY.md"] || !names["a.md"] {
		t.Fatalf("the memory folder changed: %v", names)
	}
	if _, text := app.store.RulesFor(); strings.Contains(text, mirrorPrefix) {
		t.Error("the AIs were told about devices when only one is connected")
	}
	// The memories are published, ready for a second device.
	if b, err := os.ReadFile(filepath.Join(shared, "devices", app.store.deviceID(), "memory", "a.md")); err != nil || string(b) != "fact a" {
		t.Fatalf("own memories not shared: %q %v", b, err)
	}
	if info := app.Devices(); len(info.Devices) != 1 || !info.Devices[0].This || info.Devices[0].Memories != 2 {
		t.Fatalf("dropdown: %+v", info.Devices)
	}
}

// Two devices: each keeps its own memories, reads the other's from a folder
// named after it, and the AIs are told which is which.
func TestTwoDevicesShareMemory(t *testing.T) {
	app, mem, drive := deviceApp(t)
	os.WriteFile(filepath.Join(mem, "MEMORY.md"), []byte("# Memory\n"), 0o644)
	os.WriteFile(filepath.Join(mem, "desk.md"), []byte("from the desktop"), 0o644)
	shared := connect(t, app, drive)
	writePeer(t, shared, "mac-1", "Kaya's MacBook", map[string]string{
		"MEMORY.md": "# Memory\n- [Trip](trip.md)\n", "trip.md": "wrote this on vacation",
	})
	if err := app.syncDevices(); err != nil {
		t.Fatal(err)
	}

	mirror := filepath.Join(mem, "from-kayas-macbook")
	if b, err := os.ReadFile(filepath.Join(mirror, "trip.md")); err != nil || string(b) != "wrote this on vacation" {
		t.Fatalf("the other device's memory is missing: %q %v", b, err)
	}
	if !fileExists(filepath.Join(mirror, "MEMORY.md")) {
		t.Error("the other device's index is missing")
	}
	// Its memories are not mixed into this device's own.
	if fileExists(filepath.Join(mem, "trip.md")) {
		t.Error("another device's memory was copied in as this device's own")
	}
	idx, _ := os.ReadFile(filepath.Join(mem, devicesIndex))
	for _, want := range []string{"Desktop", "Kaya's MacBook", "from-kayas-macbook/", "active now"} {
		if !strings.Contains(string(idx), want) {
			t.Errorf("%s lacks %q:\n%s", devicesIndex, want, idx)
		}
	}
	if _, text := app.store.RulesFor(); !strings.Contains(text, mirrorPrefix+"<device>/") || !strings.Contains(text, devicesIndex) {
		t.Error("the AIs were not told about the other device")
	}
	info := app.Devices()
	if len(info.Devices) != 2 || info.Devices[1].Name != "Kaya's MacBook" || info.Devices[1].Memories != 2 || !info.Devices[1].Online {
		t.Fatalf("dropdown: %+v", info.Devices)
	}
}

// The mirror never travels on: a memory read from another device is not
// published again as this device's own, however many times AIT syncs.
func TestMirroredMemoriesAreNotRepublished(t *testing.T) {
	app, mem, drive := deviceApp(t)
	os.WriteFile(filepath.Join(mem, "own.md"), []byte("mine"), 0o644)
	shared := connect(t, app, drive)
	writePeer(t, shared, "mac-1", "MacBook", map[string]string{"trip.md": "theirs"})
	for i := 0; i < 3; i++ {
		if err := app.syncDevices(); err != nil {
			t.Fatal(err)
		}
	}
	ours := dirNames(t, filepath.Join(shared, "devices", app.store.deviceID(), "memory"))
	if ours["trip.md"] {
		t.Error("another device's memory was published as this device's own")
	}
	if !ours["own.md"] {
		t.Error("this device's own memory is not published")
	}
}

// AIT only ever writes its own folder in the shared area.
func TestPeerFolderIsNeverWritten(t *testing.T) {
	app, _, drive := deviceApp(t)
	shared := connect(t, app, drive)
	writePeer(t, shared, "mac-1", "MacBook", map[string]string{"trip.md": "theirs"})
	peer := filepath.Join(shared, "devices", "mac-1")
	before := snapshot(t, peer)
	if err := app.syncDevices(); err != nil {
		t.Fatal(err)
	}
	if after := snapshot(t, peer); after != before {
		t.Errorf("the other device's folder was written to:\n%s\n%s", before, after)
	}
}

// What a device deletes disappears everywhere, in both directions.
func TestDeletionsFollow(t *testing.T) {
	app, mem, drive := deviceApp(t)
	os.WriteFile(filepath.Join(mem, "own.md"), []byte("mine"), 0o644)
	shared := connect(t, app, drive)
	writePeer(t, shared, "mac-1", "MacBook", map[string]string{"a.md": "1", "b.md": "2"})
	app.syncDevices()

	os.Remove(filepath.Join(shared, "devices", "mac-1", "memory", "b.md"))
	os.Remove(filepath.Join(mem, "own.md"))
	app.syncDevices()

	if fileExists(filepath.Join(mem, mirrorPrefix+"macbook", "b.md")) {
		t.Error("a memory the other device deleted is still mirrored")
	}
	if !fileExists(filepath.Join(mem, mirrorPrefix+"macbook", "a.md")) {
		t.Error("the other memories went with it")
	}
	if fileExists(filepath.Join(shared, "devices", app.store.deviceID(), "memory", "own.md")) {
		t.Error("a memory this device deleted is still shared")
	}
}

// A device that leaves takes its memories out of this one, and Disconnect
// leaves the memory folder as it started.
func TestForgetAndDisconnect(t *testing.T) {
	app, mem, drive := deviceApp(t)
	os.WriteFile(filepath.Join(mem, "own.md"), []byte("mine"), 0o644)
	shared := connect(t, app, drive)
	writePeer(t, shared, "mac-1", "MacBook", map[string]string{"a.md": "1"})
	writePeer(t, shared, "pc-2", "Old PC", map[string]string{"b.md": "2"})
	app.syncDevices()
	if len(app.Devices().Devices) != 3 {
		t.Fatalf("expected two other devices: %+v", app.Devices().Devices)
	}

	app.ForgetDevice("pc-2")
	if fileExists(filepath.Join(mem, mirrorPrefix+"old-pc")) {
		t.Error("a forgotten device's memories are still here")
	}
	if !fileExists(filepath.Join(mem, mirrorPrefix+"macbook", "a.md")) {
		t.Error("forgetting one device removed another's memories")
	}
	if !fileExists(filepath.Join(shared, "devices", "pc-2", deviceFile)) {
		t.Error("forgetting a device deleted it from the shared folder")
	}

	app.DisconnectDevices()
	names := dirNames(t, mem)
	if !names["own.md"] || names[devicesIndex] {
		t.Fatalf("after disconnecting: own memories stay, the device index goes: %v", names)
	}
	for n := range names {
		if strings.HasPrefix(n, mirrorPrefix) {
			t.Errorf("another device's memories are still here: %s", n)
		}
	}
	if fileExists(filepath.Join(shared, "devices", app.store.deviceID())) {
		t.Error("this device is still in the shared folder")
	}
	if app.Devices().Connected {
		t.Error("still connected")
	}
}

// The drive is not mounted (on a plane, or the stick is out): AIT says so and
// keeps every memory it already has.
func TestOfflineDriveKeepsMemories(t *testing.T) {
	app, mem, drive := deviceApp(t)
	shared := connect(t, app, drive)
	writePeer(t, shared, "mac-1", "MacBook", map[string]string{"a.md": "1"})
	app.syncDevices()

	gone := filepath.Join(t.TempDir(), "unmounted", syncFolder)
	c := app.store.Config()
	c.SyncDir = gone
	app.store.saveConfig(c)

	err := app.syncDevices()
	if err == nil || !strings.Contains(err.Error(), "available") {
		t.Fatalf("an unavailable drive should be reported: %v", err)
	}
	if !fileExists(filepath.Join(mem, mirrorPrefix+"macbook", "a.md")) {
		t.Error("memories were thrown away while the drive was offline")
	}
	if info := app.Devices(); info.Error == "" {
		t.Error("the dropdown does not show the problem")
	}
}

func TestSharedFolderChoice(t *testing.T) {
	picked := t.TempDir()
	mem := t.TempDir()
	got, err := sharedFolderFor(picked, mem)
	if err != nil || got != filepath.Join(picked, syncFolder) {
		t.Fatalf("AIT's folder goes inside the one picked: %q %v", got, err)
	}
	// The second device picks the same folder, which the drive has synced by
	// then, so it is already AIT's own.
	os.MkdirAll(got, 0o755)
	again, err := sharedFolderFor(got, mem)
	if err != nil || again != got {
		t.Fatalf("picking the shared folder itself: %q %v", again, err)
	}
	if _, err := sharedFolderFor(mem, mem); err == nil {
		t.Error("the memory folder itself was accepted, which would copy into itself")
	}
	inside := filepath.Join(mem, "sub")
	os.MkdirAll(inside, 0o755)
	if _, err := sharedFolderFor(inside, mem); err == nil {
		t.Error("a folder inside the memory folder was accepted")
	}
	if _, err := sharedFolderFor(filepath.Join(picked, "nope"), mem); err == nil {
		t.Error("a folder that does not exist was accepted")
	}
}

// Two devices the user called the same thing still get a folder each.
func TestSameNamedDevices(t *testing.T) {
	a := Device{ID: "1111-aaaa", Name: "MacBook"}
	b := Device{ID: "2222-bbbb", Name: "macbook"}
	peers := []Device{a, b}
	if mirrorName(a, peers) == mirrorName(b, peers) {
		t.Fatalf("both devices mirror into %q", mirrorName(a, peers))
	}
	if only := mirrorName(a, []Device{a}); only != "from-macbook" {
		t.Errorf("a single device should keep the plain name: %q", only)
	}
}

// A device joining or leaving while AIT is open replaces the agent waiting in
// the wings, whose instructions were written before it. The first sync of a
// run only takes stock: that agent already knew.
func TestPeerArrivalReplacesStandby(t *testing.T) {
	app, _, drive := deviceApp(t)
	shared := connect(t, app, drive)
	writePeer(t, shared, "mac-1", "MacBook", map[string]string{"a.md": "1"})

	app.sync.known = false // as at start-up: the folder has not been read yet
	app.standby = &Tab{id: 99, agent: registry["claude"], profile: "claude"}
	app.syncDevices() // the first one: the standby was started knowing this
	if app.standby == nil {
		t.Fatal("the first sync threw away a standby that was already up to date")
	}
	writePeer(t, shared, "pc-2", "Old PC", map[string]string{"b.md": "2"})
	app.syncDevices()
	if app.standby != nil {
		t.Error("a device arriving did not replace the standby")
	}
}

func dirNames(t *testing.T, dir string) map[string]bool {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]bool{}
	for _, e := range entries {
		out[e.Name()] = true
	}
	return out
}

// snapshot is every file under dir with its contents, to prove nothing changed.
func snapshot(t *testing.T, dir string) string {
	t.Helper()
	var b strings.Builder
	filepath.Walk(dir, func(p string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		body, _ := os.ReadFile(p)
		rel, _ := filepath.Rel(dir, p)
		b.WriteString(rel + "=" + string(body) + "\n")
		return nil
	})
	return b.String()
}

package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"
)

// Connected devices: every PC and Mac that runs Cmdhive can share one memory.
//
// There is no server and no account. The user points every device at the same
// folder — one their own cloud drive already syncs (iCloud Drive, OneDrive,
// Dropbox, Syncthing), or a NAS share, or a USB stick — and Cmdhive keeps the
// shared memory inside it:
//
//	<picked folder>/Cmdhive Memory/devices/<device id>/device.json   name, OS, last seen
//	                                              /memory/*.md   that device's memories
//
// A device writes only its own folder there, so two devices can never edit the
// same file and the drive can never leave a conflict copy behind. What a device
// reads from the others it mirrors into its own memory folder, one subfolder
// per device, so the AIs see every memory and always know which device it came
// from:
//
//	<memory folder>/MEMORY.md          this device's own memories (unchanged)
//	               /from-macbook/…     another device's, read-only
//	               /DEVICES.md         what is connected (Cmdhive writes this)
//
// With one device connected there is nothing to mirror and nothing changes: the
// memory folder stays exactly as it was.

const (
	syncFolder   = "Cmdhive Memory" // created inside the folder the user picks
	legacyFolder = "AIT Memory"     // its name before Cmdhive was renamed
	mirrorPrefix = "from-"
	mirrorMark   = ".ait-mirror" // marks a folder as Cmdhive's copy, safe to remove
	devicesIndex = "DEVICES.md"
	deviceFile   = "device.json"
	syncEvery    = 30 * time.Second
	deviceOnline = 5 * time.Minute // seen this recently: Cmdhive is running there
)

// Device is one Cmdhive installation, as recorded in the shared folder.
type Device struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	OS       string `json:"os"`
	Version  string `json:"version"`
	LastSeen int64  `json:"lastSeen"`
}

// DeviceView is a device as the dropdown shows it.
type DeviceView struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	OS       string `json:"os"`
	Folder   string `json:"folder"`   // where its memories are mirrored locally ("" for this device)
	Seen     string `json:"seen"`     // "active now", "4 h ago", …
	This     bool   `json:"this"`     // this device
	Online   bool   `json:"online"`   // Cmdhive is running there now
	Memories int    `json:"memories"` // .md files it shares
}

// DevicesInfo is the whole dropdown's state.
type DevicesInfo struct {
	Connected bool         `json:"connected"` // a shared folder is set
	Folder    string       `json:"folder"`    // the shared folder, for display
	Devices   []DeviceView `json:"devices"`   // this device first
	Memory    string       `json:"memory"`    // the local memory folder, for display
	Error     string       `json:"error"`     // the drive is offline, the folder was deleted, …
}

// ---- this device ------------------------------------------------------------

// deviceID is this installation's own id, made once and kept in config.json.
func (s *Store) deviceID() string {
	if c := s.Config(); c.DeviceID != "" {
		return c.DeviceID
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	c := s.Config() // again under the lock: another goroutine may have made one
	if c.DeviceID == "" {
		c.DeviceID = newUUID()
		s.saveConfig(c)
	}
	return c.DeviceID
}

// deviceName is what the user sees in the dropdown: the computer's own name,
// unless they renamed it here.
func (s *Store) deviceName() string {
	if n := strings.TrimSpace(s.Config().DeviceName); n != "" {
		return n
	}
	if h, err := os.Hostname(); err == nil && h != "" {
		return strings.TrimSuffix(h, ".local")
	}
	return deviceOS()
}

func deviceOS() string {
	switch runtime.GOOS {
	case "windows":
		return "Windows"
	case "darwin":
		return "Mac"
	default:
		return runtime.GOOS
	}
}

// ---- the shared folder ------------------------------------------------------

func (s *Store) syncDir() string      { return s.Config().SyncDir }
func (s *Store) devicesRoot() string  { return filepath.Join(s.syncDir(), "devices") }
func (s *Store) ownDeviceDir() string { return filepath.Join(s.devicesRoot(), s.deviceID()) }
func (s *Store) ownShareDir() string  { return filepath.Join(s.ownDeviceDir(), "memory") }

// peers are the other devices in the shared folder, newest sighting first.
// Devices the user forgot on this device are left out.
func (s *Store) peers() []Device {
	own, forgot := s.deviceID(), s.Config().ForgotDevices
	entries, _ := os.ReadDir(s.devicesRoot())
	var out []Device
	for _, e := range entries {
		if !e.IsDir() || e.Name() == own || slicesContains(forgot, e.Name()) {
			continue
		}
		if d, ok := readDevice(filepath.Join(s.devicesRoot(), e.Name())); ok {
			out = append(out, d)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].LastSeen > out[j].LastSeen })
	return out
}

func readDevice(dir string) (Device, bool) {
	b, err := os.ReadFile(filepath.Join(dir, deviceFile))
	if err != nil {
		return Device{}, false
	}
	var d Device
	if json.Unmarshal(b, &d) != nil || d.ID == "" {
		return Device{}, false
	}
	if d.Name == "" {
		d.Name = d.OS
	}
	return d, true
}

// mirrorName is the folder a peer's memories are mirrored into. Two devices
// with the same name are told apart by the start of their id.
func mirrorName(d Device, peers []Device) string {
	slug := slugify(d.Name)
	if slug == "" {
		slug = "device"
	}
	for _, o := range peers {
		if o.ID != d.ID && slugify(o.Name) == slug {
			return mirrorPrefix + slug + "-" + shortID(d.ID)
		}
	}
	return mirrorPrefix + slug
}

func shortID(id string) string {
	flat := strings.ReplaceAll(id, "-", "")
	return flat[:min(4, len(flat))]
}

func slugify(s string) string {
	var b strings.Builder
	dash := false
	for _, r := range strings.ToLower(s) {
		switch {
		case r >= 'a' && r <= 'z' || r >= '0' && r <= '9':
			b.WriteRune(r)
			dash = false
		case r == '\'' || r == '’' || r == '.':
			// "Kaya's MacBook" is one word, not two: from-kayas-macbook.
		case b.Len() > 0 && !dash:
			b.WriteByte('-')
			dash = true
		}
	}
	return strings.Trim(b.String(), "-")
}

func slicesContains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

// ---- syncing ----------------------------------------------------------------

type deviceSync struct {
	run   sync.Mutex // one sync at a time: the clock and "Sync now" can collide
	mu    sync.Mutex
	last  string // the last error, shown in the dropdown
	peers string // the devices last seen, to notice one arriving or leaving
	known bool   // peers has been read at least once
}

// startDeviceSync keeps this device's memories and the other devices' in step,
// from start-up until Cmdhive quits. Doing nothing is the normal case: no shared
// folder means no work.
func (a *App) startDeviceSync() {
	go func() {
		for {
			if a.store.syncDir() != "" {
				a.syncDevices()
			}
			time.Sleep(syncEvery)
		}
	}()
}

// syncDevices publishes this device's memories and mirrors the other devices'
// into the local memory folder.
func (a *App) syncDevices() error {
	root := a.store.syncDir()
	if root == "" {
		return nil
	}
	a.sync.run.Lock()
	err := a.runSync(root)
	a.sync.run.Unlock()
	msg := ""
	if err != nil {
		msg = err.Error()
	}
	a.sync.mu.Lock()
	changed := a.sync.last != msg
	a.sync.last = msg
	a.sync.mu.Unlock()
	if changed {
		a.emit("devices:changed")
	}
	return err
}

func (a *App) runSync(root string) error {
	// A cloud drive that has not mounted yet, or a USB stick that is out: leave
	// every mirror alone, so the AIs keep reading what they already have.
	if !fileExists(filepath.Dir(root)) {
		return fmt.Errorf("%s isn't available right now", maskUser(filepath.Dir(root)))
	}
	if err := os.MkdirAll(a.store.ownShareDir(), 0o755); err != nil {
		return err
	}
	mem := a.store.ensureMemory()

	// 1. Say we are here, and what this device is called.
	own := Device{ID: a.store.deviceID(), Name: a.store.deviceName(), OS: deviceOS(), Version: Version, LastSeen: time.Now().Unix()}
	b, _ := json.MarshalIndent(own, "", "  ")
	if err := writeAtomic(filepath.Join(a.store.ownDeviceDir(), deviceFile), b); err != nil {
		return err
	}

	// 2. Publish this device's own memories: the top level of the memory
	//    folder. The mirrors of other devices are subfolders, so what came
	//    from another device is never sent back out as ours.
	if err := mirrorFlat(mem, a.store.ownShareDir(), devicesIndex); err != nil {
		return err
	}

	// 3. Mirror every other device's memories in, one folder each.
	peers := a.store.peers()
	keep := map[string]bool{}
	for _, p := range peers {
		name := mirrorName(p, peers)
		keep[name] = true
		into := filepath.Join(mem, name)
		if err := mirrorFlat(filepath.Join(a.store.devicesRoot(), p.ID, "memory"), into); err != nil {
			return err
		}
		os.WriteFile(filepath.Join(into, mirrorMark), []byte(p.ID), 0o644)
	}
	// 4. Drop mirrors of devices that are no longer there, and write the index.
	dropMirrors(mem, keep)
	if err := writeDevicesIndex(mem, own, peers); err != nil {
		return err
	}
	a.peersChanged(peers)
	return nil
}

// peersChanged notices another device joining or leaving while Cmdhive is open.
// The instructions AIs are started with name the other devices, so the agent
// waiting in the wings was briefed before this one arrived: it is replaced.
func (a *App) peersChanged(peers []Device) {
	ids := make([]string, 0, len(peers))
	for _, p := range peers {
		ids = append(ids, p.ID)
	}
	sort.Strings(ids)
	now := strings.Join(ids, ",")
	a.sync.mu.Lock()
	// The first sync only takes stock: the agent waiting in the wings was
	// started after these devices were already in the shared folder.
	changed := a.sync.known && a.sync.peers != now
	a.sync.peers, a.sync.known = now, true
	a.sync.mu.Unlock()
	if !changed {
		return
	}
	a.killStandby()
	a.emit("devices:changed")
}

// mirrorFlat makes dst hold exactly the files at the top level of src: changed
// files are copied, files no longer in src are removed. It is only ever
// pointed at a folder Cmdhive owns — this device's folder in the shared area, or a
// mirror folder in the local memory — never another device's.
func mirrorFlat(src, dst string, skip ...string) error {
	entries, err := os.ReadDir(src)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dst, 0o755); err != nil {
		return err
	}
	want := map[string]bool{}
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || strings.HasPrefix(name, ".") || slicesContains(skip, name) {
			continue
		}
		from, err := e.Info()
		if err != nil {
			continue
		}
		want[name] = true
		if to, err := os.Stat(filepath.Join(dst, name)); err == nil &&
			to.Size() == from.Size() && !from.ModTime().After(to.ModTime()) {
			continue // already there, and no newer
		}
		body, err := os.ReadFile(filepath.Join(src, name))
		if err != nil {
			continue
		}
		if err := writeAtomic(filepath.Join(dst, name), body); err != nil {
			return err
		}
		os.Chtimes(filepath.Join(dst, name), from.ModTime(), from.ModTime())
	}
	old, _ := os.ReadDir(dst)
	for _, e := range old {
		if !e.IsDir() && !want[e.Name()] && !strings.HasPrefix(e.Name(), ".") {
			os.Remove(filepath.Join(dst, e.Name()))
		}
	}
	return nil
}

// dropMirrors removes the mirror folders of devices that are no longer
// connected. Nothing else in the memory folder is touched.
func dropMirrors(mem string, keep map[string]bool) {
	entries, _ := os.ReadDir(mem)
	for _, e := range entries {
		if !e.IsDir() || keep[e.Name()] || !strings.HasPrefix(e.Name(), mirrorPrefix) {
			continue
		}
		// Only a folder Cmdhive filled itself; a folder of the user's that happens
		// to be named this way is left alone.
		if fileExists(filepath.Join(mem, e.Name(), mirrorMark)) {
			os.RemoveAll(filepath.Join(mem, e.Name()))
		}
	}
}

// writeDevicesIndex writes DEVICES.md: the one page that tells an AI which
// devices are connected and where each one's memories are. With no other
// device connected there is nothing to explain, so the file goes away.
func writeDevicesIndex(mem string, own Device, peers []Device) error {
	p := filepath.Join(mem, devicesIndex)
	if len(peers) == 0 {
		os.Remove(p)
		return nil
	}
	var b strings.Builder
	b.WriteString("# Connected devices\n\n")
	b.WriteString("Cmdhive keeps one memory across several computers. Cmdhive writes this file — don't edit it.\n\n")
	fmt.Fprintf(&b, "- **%s** (%s) — this device. Its memories are the `.md` files next to this one, indexed by `%s`. Save new memories here.\n", own.Name, own.OS, memoryIndex)
	for _, d := range peers {
		folder := mirrorName(d, peers)
		fmt.Fprintf(&b, "- **%s** (%s, %s) — in `%s/`, indexed by `%s/%s`. Read-only: Cmdhive overwrites that folder.\n",
			d.Name, d.OS, seenText(d.LastSeen), folder, folder, memoryIndex)
	}
	b.WriteString("\nWhich device a memory came from can matter: paths, installed tools and hardware differ per device.\n")
	return writeAtomic(p, []byte(b.String()))
}

func seenText(unix int64) string {
	if unix == 0 {
		return "never seen"
	}
	d := time.Since(time.Unix(unix, 0))
	switch {
	case d < deviceOnline:
		return "active now"
	case d < time.Hour:
		return fmt.Sprintf("%d min ago", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%d h ago", int(d.Hours()))
	case d < 48*time.Hour:
		return "yesterday"
	default:
		return fmt.Sprintf("%d days ago", int(d.Hours()/24))
	}
}

// writeAtomic writes a file in one step, so a cloud drive or another Cmdhive never
// reads half of it.
func writeAtomic(p string, b []byte) error {
	os.MkdirAll(filepath.Dir(p), 0o755)
	tmp := p + ".ait-tmp"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return err
	}
	if err := os.Rename(tmp, p); err != nil {
		os.Remove(tmp)
		return err
	}
	return nil
}

// countMemories counts the memories at the top level of a folder.
func countMemories(dir string) int {
	entries, _ := os.ReadDir(dir)
	n := 0
	for _, e := range entries {
		if !e.IsDir() && strings.EqualFold(filepath.Ext(e.Name()), ".md") && e.Name() != devicesIndex {
			n++
		}
	}
	return n
}

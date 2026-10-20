package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// What the device dropdown calls, and finding the folder to share through.

// CloudFolder is a folder AIT found that is already synced between this
// user's computers, offered as a one-click choice when connecting.
type CloudFolder struct {
	Name string `json:"name"` // "iCloud Drive", "OneDrive", …
	Path string `json:"path"`
	Show string `json:"show"` // the path as the user knows it
}

// cloudCandidates are the usual folders of the drives people already have,
// per OS. Globs, because macOS mounts each service under its own name.
func cloudCandidates() []CloudFolder {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil
	}
	type cand struct{ name, glob string }
	var list []cand
	if runtime.GOOS == "darwin" {
		list = []cand{
			{"iCloud Drive", "Library/Mobile Documents/com~apple~CloudDocs"},
			{"", "Library/CloudStorage/*"}, // OneDrive-…, Dropbox, GoogleDrive-…
			{"Dropbox", "Dropbox"},
			{"Sync", "Sync"},
		}
	} else {
		list = []cand{
			{"", "OneDrive*"},
			{"Dropbox", "Dropbox"},
			{"Google Drive", "Google Drive"},
			{"iCloud Drive", "iCloudDrive"},
			{"Sync", "Sync"},
		}
	}
	var out []CloudFolder
	seen := map[string]bool{}
	for _, c := range list {
		matches, _ := filepath.Glob(filepath.Join(home, filepath.FromSlash(c.glob)))
		for _, m := range matches {
			if st, err := os.Stat(m); err != nil || !st.IsDir() || seen[m] {
				continue
			}
			seen[m] = true
			name := c.name
			if name == "" { // the folder names itself: "OneDrive - Personal", "GoogleDrive-me@…"
				name = cloudServiceName(filepath.Base(m))
			}
			out = append(out, CloudFolder{Name: name, Path: m, Show: maskUser(m)})
		}
	}
	return out
}

// cloudServiceName turns a mount folder's name into the service's name.
func cloudServiceName(base string) string {
	name := base
	for _, cut := range []string{" - ", "-"} {
		if i := strings.Index(name, cut); i > 0 {
			name = name[:i]
			break
		}
	}
	switch strings.ToLower(name) {
	case "googledrive":
		return "Google Drive"
	case "onedrive":
		return "OneDrive"
	}
	return name
}

// CloudFolders offers the synced folders AIT found on this computer.
func (a *App) CloudFolders() []CloudFolder { return cloudCandidates() }

// Devices is the dropdown's state: this device first, then the others.
func (a *App) Devices() DevicesInfo {
	s := a.store
	mem := s.MemoryDir()
	info := DevicesInfo{Connected: s.syncDir() != "", Folder: maskUser(s.syncDir()), Memory: maskUser(mem)}
	a.sync.mu.Lock()
	info.Error = a.sync.last
	a.sync.mu.Unlock()
	info.Devices = []DeviceView{{
		ID: s.deviceID(), Name: s.deviceName(), OS: deviceOS(),
		This: true, Online: true, Seen: "this device", Memories: countMemories(mem),
	}}
	if !info.Connected {
		return info
	}
	peers := s.peers()
	for _, d := range peers {
		folder := mirrorName(d, peers)
		info.Devices = append(info.Devices, DeviceView{
			ID: d.ID, Name: d.Name, OS: d.OS, Folder: folder, Seen: seenText(d.LastSeen),
			Online:   seenText(d.LastSeen) == "active now",
			Memories: countMemories(filepath.Join(mem, folder)),
		})
	}
	return info
}

// ConnectDevices starts sharing memory through a folder every device of the
// user's can reach. An empty path opens the folder picker.
func (a *App) ConnectDevices(path string) (DevicesInfo, error) {
	if path == "" {
		picked, err := pickFolder(a.ctx, a.store.home)
		if err != nil || picked == "" {
			return a.Devices(), err
		}
		path = picked
	}
	dir, err := sharedFolderFor(path, a.store.MemoryDir())
	if err != nil {
		return a.Devices(), err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return a.Devices(), fmt.Errorf("AIT can't write to that folder: %w", err)
	}
	c := a.store.Config()
	c.SyncDir = dir
	c.ForgotDevices = nil
	a.store.saveConfig(c)
	if err := a.syncDevices(); err != nil {
		return a.Devices(), err
	}
	a.killStandby() // its instructions were written before the other devices
	a.emit("devices:changed")
	return a.Devices(), nil
}

// sharedFolderFor is the folder AIT will keep the shared memory in: the one
// the user picked, with AIT's own folder inside it unless they picked that
// folder itself (choosing the same one again on a second device).
func sharedFolderFor(picked, memory string) (string, error) {
	picked = filepath.Clean(picked)
	if st, err := os.Stat(picked); err != nil || !st.IsDir() {
		return "", errors.New("that folder does not exist")
	}
	dir := picked
	if !strings.EqualFold(filepath.Base(picked), syncFolder) {
		dir = filepath.Join(picked, syncFolder)
	}
	// The shared folder holds copies of the memory folder, so one inside the
	// other would copy into itself for ever.
	if insideDir(dir, memory) || insideDir(memory, dir) {
		return "", errors.New("pick a folder outside the shared memory folder itself")
	}
	return dir, nil
}

// insideDir reports whether child is dir or sits under it.
func insideDir(child, dir string) bool {
	rel, err := filepath.Rel(filepath.Clean(dir), filepath.Clean(child))
	if err != nil {
		return false
	}
	return rel == "." || !strings.HasPrefix(rel, "..")
}

// DisconnectDevices stops sharing: this device is removed from the shared
// folder, and the other devices' memories go with it. Nothing of this
// device's own memory is touched.
func (a *App) DisconnectDevices() DevicesInfo {
	s := a.store
	if dir := s.ownDeviceDir(); s.syncDir() != "" {
		os.RemoveAll(dir) // our own folder there, nobody else's
	}
	c := s.Config()
	c.SyncDir = ""
	c.ForgotDevices = nil
	s.saveConfig(c)
	mem := s.MemoryDir()
	dropMirrors(mem, nil)
	os.Remove(filepath.Join(mem, devicesIndex))
	a.sync.mu.Lock()
	a.sync.last = ""
	a.sync.mu.Unlock()
	a.killStandby()
	a.emit("devices:changed")
	return a.Devices()
}

// RenameDevice is what this device is called in the dropdown and in the
// memory folder of every other device.
func (a *App) RenameDevice(name string) DevicesInfo {
	name = strings.TrimSpace(name)
	if len(name) > 40 {
		name = strings.TrimSpace(name[:40])
	}
	c := a.store.Config()
	c.DeviceName = name
	a.store.saveConfig(c)
	a.syncDevices()
	a.emit("devices:changed")
	return a.Devices()
}

// ForgetDevice drops another device's memories from this device. It only
// stops this device from reading them: the device itself and its own
// memories are left alone, and it can be taken back with Connect.
func (a *App) ForgetDevice(id string) DevicesInfo {
	c := a.store.Config()
	if id != "" && id != a.store.deviceID() && !slicesContains(c.ForgotDevices, id) {
		c.ForgotDevices = append(c.ForgotDevices, id)
		a.store.saveConfig(c)
	}
	a.syncDevices()
	a.killStandby()
	a.emit("devices:changed")
	return a.Devices()
}

// SyncDevicesNow syncs at once instead of at the next turn of the clock.
func (a *App) SyncDevicesNow() DevicesInfo {
	a.syncDevices()
	return a.Devices()
}

// OpenSharedFolder shows the shared folder in Explorer or Finder.
func (a *App) OpenSharedFolder() {
	if d := a.store.syncDir(); d != "" {
		openFolder(d)
	}
}

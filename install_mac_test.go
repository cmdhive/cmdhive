package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// Node.js for a Mac comes as a .tar.gz with a top folder; it is unpacked into
// Cmdhive's own folder without that folder, links and run bits intact, and nothing
// may escape the folder.
func TestUntarStripped(t *testing.T) {
	build := func(files map[string]string, link bool) []byte {
		var buf bytes.Buffer
		zw := gzip.NewWriter(&buf)
		tw := tar.NewWriter(zw)
		tw.WriteHeader(&tar.Header{Name: "node-v1/", Typeflag: tar.TypeDir, Mode: 0o755})
		for name, body := range files {
			tw.WriteHeader(&tar.Header{Name: name, Typeflag: tar.TypeReg, Mode: 0o755, Size: int64(len(body))})
			tw.Write([]byte(body))
		}
		if link {
			tw.WriteHeader(&tar.Header{Name: "node-v1/bin/npm", Typeflag: tar.TypeSymlink, Linkname: "../lib/npm.js"})
		}
		tw.Close()
		zw.Close()
		return buf.Bytes()
	}
	dir := filepath.Join(t.TempDir(), "node")
	if err := untarStripped(build(map[string]string{"node-v1/bin/node": "#!x", "node-v1/lib/npm.js": "js"}, runtime.GOOS != "windows"), dir); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(filepath.Join(dir, "bin", "node")); string(b) != "#!x" {
		t.Errorf("node: %q", b)
	}
	if b, err := os.ReadFile(filepath.Join(dir, "lib", "npm.js")); err != nil || string(b) != "js" {
		t.Errorf("npm.js: %q %v", b, err)
	}
	if err := untarStripped(build(map[string]string{"node-v1/../../evil": "x"}, false), dir); err == nil {
		t.Error("a path that leaves the folder was unpacked")
	}
}

package update

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

// fakeGitHub serves releases like GitHub: releases/latest redirects to the tag, assets are files.
type fakeGitHub struct {
	latest string            // tag, empty without releases
	assets map[string][]byte // path below releases/download/
}

func (f *fakeGitHub) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch {
	case r.URL.Path == "/releases/latest" && f.latest == "":
		http.Redirect(w, r, "/releases", http.StatusFound)
	case r.URL.Path == "/releases/latest":
		http.Redirect(w, r, "/releases/tag/"+f.latest, http.StatusFound)
	default:
		data, ok := f.assets[r.URL.Path]
		if !ok {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write(data)
	}
}

func serve(t *testing.T, f *fakeGitHub) *Source {
	t.Helper()
	server := httptest.NewServer(f)
	t.Cleanup(server.Close)
	return &Source{Repo: server.URL, Client: server.Client(), OS: "linux", Arch: "amd64"}
}

func TestLatest(t *testing.T) {
	source := serve(t, &fakeGitHub{latest: "v0.3.0"})

	release, err := source.Latest(context.Background())

	if err != nil || release.Version != "0.3.0" || release.URL != source.Repo+"/releases/tag/v0.3.0" {
		t.Errorf("release = %+v, err = %v", release, err)
	}
}

func TestLatestWithoutRelease(t *testing.T) {
	_, err := serve(t, &fakeGitHub{}).Latest(context.Background())

	if !errors.Is(err, ErrNoRelease) {
		t.Errorf("err = %v", err)
	}
}

func TestLatestWithInvalidTag(t *testing.T) {
	if _, err := serve(t, &fakeGitHub{latest: "nightly"}).Latest(context.Background()); err == nil {
		t.Error("an invalid tag is accepted")
	}
}

// release creates the archive of a release with the binary and its checksums.txt.
func release(t *testing.T, binary string) (archive, sums []byte) {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for name, content := range map[string]string{"README.md": "readme", "pt": binary} {
		if err := tw.WriteHeader(&tar.Header{Name: name, Mode: 0o644, Size: int64(len(content)), Typeflag: tar.TypeReg}); err != nil {
			t.Fatal(err)
		}
		_, _ = tw.Write([]byte(content))
	}
	_ = tw.Close()
	_ = gz.Close()

	sum := sha256.Sum256(buf.Bytes())
	sums = []byte("0000  pt_0.3.0_darwin_arm64.tar.gz\n" + hex.EncodeToString(sum[:]) + "  pt_0.3.0_linux_amd64.tar.gz\n")
	return buf.Bytes(), sums
}

func TestInstall(t *testing.T) {
	archive, sums := release(t, "new binary")
	source := serve(t, &fakeGitHub{assets: map[string][]byte{
		"/releases/download/v0.3.0/pt_0.3.0_linux_amd64.tar.gz": archive,
		"/releases/download/v0.3.0/checksums.txt":               sums,
	}})
	target := filepath.Join(t.TempDir(), "pt")
	if err := os.WriteFile(target, []byte("old binary"), 0o755); err != nil {
		t.Fatal(err)
	}

	var done, total int64
	err := source.Install(context.Background(), Release{Version: "0.3.0"}, target, func(d, t int64) { done, total = d, t })

	if err != nil {
		t.Fatal(err)
	}
	content, _ := os.ReadFile(target)
	info, _ := os.Stat(target)
	if string(content) != "new binary" || info.Mode().Perm() != 0o755 {
		t.Errorf("binary = %q, mode = %v", content, info.Mode())
	}
	if done != int64(len(archive)) || total != int64(len(archive)) {
		t.Errorf("progress = %d / %d, want %d", done, total, len(archive))
	}
	if entries, _ := os.ReadDir(filepath.Dir(target)); len(entries) != 1 {
		t.Errorf("leftovers: %v", entries)
	}
}

func TestInstallRejectsDamagedArchive(t *testing.T) {
	archive, sums := release(t, "new binary")
	archive[len(archive)-1] ^= 0xff
	source := serve(t, &fakeGitHub{assets: map[string][]byte{
		"/releases/download/v0.3.0/pt_0.3.0_linux_amd64.tar.gz": archive,
		"/releases/download/v0.3.0/checksums.txt":               sums,
	}})
	target := filepath.Join(t.TempDir(), "pt")
	_ = os.WriteFile(target, []byte("old binary"), 0o755)

	err := source.Install(context.Background(), Release{Version: "0.3.0"}, target, nil)

	var checksumErr *ChecksumError
	content, _ := os.ReadFile(target)
	if !errors.As(err, &checksumErr) || string(content) != "old binary" {
		t.Errorf("err = %v, binary = %q", err, content)
	}
	if entries, _ := os.ReadDir(filepath.Dir(target)); len(entries) != 1 {
		t.Errorf("leftovers: %v", entries)
	}
}

func TestInstallWithoutBuildForPlatform(t *testing.T) {
	_, sums := release(t, "new binary")
	source := serve(t, &fakeGitHub{assets: map[string][]byte{"/releases/download/v0.3.0/checksums.txt": sums}})
	source.OS = "plan9"

	err := source.Install(context.Background(), Release{Version: "0.3.0"}, filepath.Join(t.TempDir(), "pt"), nil)

	if err == nil {
		t.Error("installed without an archive")
	}
}

func TestInstallIntoReadOnlyDirectory(t *testing.T) {
	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		t.Skip("permissions don't apply")
	}
	dir := t.TempDir()
	target := filepath.Join(dir, "pt")
	_ = os.WriteFile(target, []byte("old binary"), 0o755)
	_ = os.Chmod(dir, 0o555)
	t.Cleanup(func() { _ = os.Chmod(dir, 0o755) })

	err := serve(t, &fakeGitHub{}).Install(context.Background(), Release{Version: "0.3.0"}, target, nil)

	var notWritable *NotWritableError
	if !errors.As(err, &notWritable) || notWritable.Dir != dir {
		t.Errorf("err = %v", err)
	}
}

func TestSchedule(t *testing.T) {
	now := time.Date(2026, 11, 2, 12, 0, 0, 0, time.UTC)
	s := Schedule{Path: filepath.Join(t.TempDir(), "pipetower", "update-check")}

	if !s.Due(now) {
		t.Error("not due without a check")
	}
	s.Checked(now)
	if s.Due(now.Add(23 * time.Hour)) {
		t.Error("due within a day")
	}
	if !s.Due(now.Add(24 * time.Hour)) {
		t.Error("not due after a day")
	}
	if !s.Due(now.Add(-time.Hour)) {
		t.Error("not due with the clock set back")
	}
	if (Schedule{}).Due(now) {
		t.Error("due without a path")
	}
}

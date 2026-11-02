// Package update finds new releases of pt on GitHub and replaces the installed binary with them,
// the same way the install script does: the archive of the platform, verified by its checksum.
package update

import (
	"archive/tar"
	"bufio"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/pipetower/pipetower-cli/internal/version"
)

// InstallCommand installs or updates pt, it is the only way to install it.
const InstallCommand = "curl -fsSL https://pipetower.com/install-cli | bash"

// maxArchiveSize limits the download, a release archive has a few megabytes.
const maxArchiveSize = 256 << 20

// Release is a published version of pt.
type Release struct {
	// Version without the v of the tag: 1.4.0.
	Version string
	// URL is the page of the release with its notes.
	URL string
}

// Source finds and downloads the releases of a GitHub repository.
type Source struct {
	// Repo is the URL of the repository: https://github.com/pipetower/pipetower-cli.
	Repo   string
	Client *http.Client
	// OS and Arch choose the archive, they default to the running platform.
	OS, Arch string
}

// RepoURL is the repository whose releases are installed, set at link time to test against another server.
var RepoURL = "https://github.com/pipetower/pipetower-cli"

// GitHub returns the source of the published releases of pt.
func GitHub() *Source {
	return &Source{Repo: RepoURL, Client: &http.Client{}}
}

// Latest returns the latest stable release. It follows the redirect of releases/latest to the tag
// instead of asking the API, whose rate limit is easily exhausted behind a shared address.
func (s *Source) Latest(ctx context.Context) (Release, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodHead, s.Repo+"/releases/latest", nil)
	if err != nil {
		return Release{}, err
	}
	req.Header.Set("User-Agent", version.UserAgent())

	client := *s.Client
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	resp, err := client.Do(req)
	if err != nil {
		return Release{}, err
	}
	_ = resp.Body.Close()

	location, err := resp.Location()
	if err != nil {
		return Release{}, fmt.Errorf("finding the latest release: %s", resp.Status)
	}
	// without any release, GitHub redirects to the list of releases instead of a tag
	_, tag, found := strings.Cut(location.Path, "/releases/tag/")
	if !found {
		return Release{}, ErrNoRelease
	}
	if tag, err = url.PathUnescape(tag); err != nil {
		return Release{}, err
	}
	v := strings.TrimPrefix(tag, "v")
	if !version.Valid(v) {
		return Release{}, fmt.Errorf("the latest release has no valid version: %s", tag)
	}

	return Release{Version: v, URL: s.Repo + "/releases/tag/" + url.PathEscape(tag)}, nil
}

// ErrNoRelease means that nothing has been released yet.
var ErrNoRelease = errors.New("no release of pt has been published yet")

// Executable returns the path of the running binary with symbolic links resolved, the file to replace.
func Executable() (string, error) {
	path, err := os.Executable()
	if err != nil {
		return "", err
	}
	return filepath.EvalSymlinks(path)
}

// Install downloads the release, verifies its checksum and replaces the binary at target with it.
// Progress is called while downloading with the bytes so far and the total (0 when unknown). The
// running process is not affected, the new version takes effect on the next start.
func (s *Source) Install(ctx context.Context, release Release, target string, progress func(done, total int64)) error {
	// fail before downloading when the binary can't be replaced
	staged, err := os.CreateTemp(filepath.Dir(target), ".pt-update-*")
	if err != nil {
		if errors.Is(err, os.ErrPermission) {
			return &NotWritableError{Dir: filepath.Dir(target)}
		}
		return err
	}
	defer func() {
		_ = staged.Close()
		_ = os.Remove(staged.Name()) // gone after a successful rename
	}()

	archive := s.archiveName(release)
	sums, err := s.download(ctx, release, "checksums.txt", nil)
	if err != nil {
		return err
	}
	want, err := checksumOf(sums, archive)
	if err != nil {
		return err
	}
	data, err := s.download(ctx, release, archive, progress)
	if err != nil {
		return err
	}
	if got := sha256.Sum256(data); hex.EncodeToString(got[:]) != want {
		return &ChecksumError{Archive: archive}
	}

	if err := extractBinary(data, staged); err != nil {
		return fmt.Errorf("unpacking %s: %w", archive, err)
	}
	if err := staged.Chmod(0o755); err != nil {
		return err
	}
	if err := staged.Close(); err != nil {
		return err
	}
	// renaming within the directory replaces the binary atomically, even while it runs
	return os.Rename(staged.Name(), target)
}

// archiveName is the archive of the platform as goreleaser names it: pt_1.4.0_linux_amd64.tar.gz.
func (s *Source) archiveName(release Release) string {
	goos, arch := s.OS, s.Arch
	if goos == "" {
		goos = runtime.GOOS
	}
	if arch == "" {
		arch = runtime.GOARCH
	}
	return fmt.Sprintf("pt_%s_%s_%s.tar.gz", release.Version, goos, arch)
}

// download fetches a file of the release.
func (s *Source) download(ctx context.Context, release Release, name string, progress func(done, total int64)) ([]byte, error) {
	link := s.Repo + "/releases/download/v" + release.Version + "/" + name
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, link, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", version.UserAgent())

	resp, err := s.Client.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode == http.StatusNotFound {
		return nil, &MissingError{Release: release, Name: name}
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("downloading %s: %s", name, resp.Status)
	}

	total := max(resp.ContentLength, 0)
	body := io.LimitReader(resp.Body, maxArchiveSize)
	if progress != nil {
		progress(0, total)
		body = &progressReader{r: body, total: total, report: progress}
	}
	return io.ReadAll(body)
}

// checksumOf finds the SHA-256 checksum of a file in checksums.txt ("<hex>  <name>" per line).
func checksumOf(sums []byte, name string) (string, error) {
	scanner := bufio.NewScanner(bytes.NewReader(sums))
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) == 2 && fields[1] == name {
			return strings.ToLower(fields[0]), nil
		}
	}
	return "", fmt.Errorf("checksums.txt has no checksum of %s", name)
}

// extractBinary writes the pt binary of a tar.gz archive.
func extractBinary(archive []byte, w io.Writer) error {
	gz, err := gzip.NewReader(bytes.NewReader(archive))
	if err != nil {
		return err
	}
	tr := tar.NewReader(gz)
	for {
		header, err := tr.Next()
		if errors.Is(err, io.EOF) {
			return errors.New("the archive contains no pt binary")
		}
		if err != nil {
			return err
		}
		if header.Typeflag == tar.TypeReg && header.Name == "pt" {
			_, err := io.Copy(w, io.LimitReader(tr, maxArchiveSize))
			return err
		}
	}
}

// progressReader reports the bytes read.
type progressReader struct {
	r      io.Reader
	done   int64
	total  int64
	report func(done, total int64)
}

func (p *progressReader) Read(b []byte) (int, error) {
	n, err := p.r.Read(b)
	p.done += int64(n)
	p.report(p.done, p.total)
	return n, err
}

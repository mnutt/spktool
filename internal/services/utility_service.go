package services

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/mnutt/spktool/internal/config"
	"github.com/mnutt/spktool/internal/domain"
)

const (
	sandstormUtilsRepo            = "mnutt/sandstorm-utils"
	sandstormUtilsReleaseAPIURL   = "https://api.github.com/repos/" + sandstormUtilsRepo + "/releases/latest"
	sandstormUtilsManifestAsset   = "utils.json"
	sandstormUtilsManifestVersion = 1
)

type UtilityCatalog struct {
	ReleaseTag string        `json:"release_tag"`
	Version    int           `json:"version"`
	Utilities  []UtilitySpec `json:"utilities"`
}

type UtilitySpec struct {
	Name        string   `json:"name"`
	Summary     string   `json:"summary"`
	Description string   `json:"description"`
	Examples    []string `json:"examples"`
}

type UtilityDetails struct {
	ReleaseTag string      `json:"release_tag"`
	Version    int         `json:"version"`
	Utility    UtilitySpec `json:"utility"`
}

type githubRelease struct {
	TagName string               `json:"tag_name"`
	Assets  []githubReleaseAsset `json:"assets"`
}

type githubReleaseAsset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
}

type utilityManifest struct {
	Version   int           `json:"version"`
	Utilities []UtilitySpec `json:"utilities"`
}

func (s *UtilityService) ListUtilities(ctx context.Context) (*UtilityCatalog, error) {
	release, manifest, err := s.latestUtilityCatalog(ctx)
	if err != nil {
		return nil, err
	}
	return &UtilityCatalog{
		ReleaseTag: release.TagName,
		Version:    manifest.Version,
		Utilities:  manifest.Utilities,
	}, nil
}

func (s *UtilityService) DescribeUtility(ctx context.Context, name string) (*UtilityDetails, error) {
	if strings.TrimSpace(name) == "" {
		return nil, &domain.Error{Code: domain.ErrInvalidArgument, Op: "services.DescribeUtility", Message: "utility name is required"}
	}

	release, manifest, err := s.latestUtilityCatalog(ctx)
	if err != nil {
		return nil, err
	}
	utility, err := manifest.findUtility(name)
	if err != nil {
		return nil, err
	}
	return &UtilityDetails{
		ReleaseTag: release.TagName,
		Version:    manifest.Version,
		Utility:    *utility,
	}, nil
}

func (s *UtilityService) Add(ctx context.Context, workDir, util string) (*domain.ProjectState, error) {
	if strings.TrimSpace(util) == "" {
		return nil, &domain.Error{Code: domain.ErrInvalidArgument, Op: "services.Add", Message: "utility name is required"}
	}

	projectState, _, err := s.deps.loadResolvedProject(ctx, workDir, "")
	if err != nil {
		return nil, err
	}

	release, manifest, err := s.latestUtilityCatalog(ctx)
	if err != nil {
		return nil, err
	}
	entry, err := manifest.findUtility(util)
	if err != nil {
		return nil, err
	}

	assetURL, err := release.assetURL(sandstormUtilsAssetName(release.TagName))
	if err != nil {
		return nil, err
	}

	target := filepath.Join(workDir, ".sandstorm", "utils", util)
	if err := s.installUtilityFromRelease(ctx, assetURL, entry.Name, target); err != nil {
		return nil, err
	}

	utilsCfg, err := config.LoadUtils(workDir)
	if err != nil {
		return nil, err
	}
	utilsCfg.Installed[util] = release.TagName
	if err := config.WriteUtils(workDir, utilsCfg); err != nil {
		return nil, err
	}

	return projectState, nil
}

func sandstormUtilsAssetName(tag string) string {
	return fmt.Sprintf("sandstorm-utils_%s_linux_amd64.tar.gz", tag)
}

func (s *UtilityService) latestUtilityCatalog(ctx context.Context) (*githubRelease, *utilityManifest, error) {
	release, err := s.latestSandstormUtilsRelease(ctx)
	if err != nil {
		return nil, nil, err
	}
	manifestURL, err := release.assetURL(sandstormUtilsManifestAsset)
	if err != nil {
		return nil, nil, err
	}
	manifest, err := s.fetchUtilityManifest(ctx, manifestURL)
	if err != nil {
		return nil, nil, err
	}
	return release, manifest, nil
}

func (s *UtilityService) latestSandstormUtilsRelease(ctx context.Context) (*githubRelease, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, sandstormUtilsReleaseAPIURL, nil)
	if err != nil {
		return nil, domain.Wrap(domain.ErrExternal, "services.ListUtilities", "create release metadata request", err)
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "spktool/"+ToolVersion)

	resp, err := s.deps.http.Do(req)
	if err != nil {
		return nil, domain.Wrap(domain.ErrExternal, "services.ListUtilities", "fetch latest sandstorm-utils release", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, &domain.Error{
			Code:    domain.ErrExternal,
			Op:      "services.ListUtilities",
			Message: fmt.Sprintf("fetch latest sandstorm-utils release: unexpected status %s", resp.Status),
		}
	}

	var release githubRelease
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return nil, domain.Wrap(domain.ErrExternal, "services.ListUtilities", "decode latest sandstorm-utils release", err)
	}
	if strings.TrimSpace(release.TagName) == "" {
		return nil, &domain.Error{Code: domain.ErrExternal, Op: "services.ListUtilities", Message: "latest sandstorm-utils release is missing tag_name"}
	}
	return &release, nil
}

func (s *UtilityService) fetchUtilityManifest(ctx context.Context, manifestURL string) (*utilityManifest, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, manifestURL, nil)
	if err != nil {
		return nil, domain.Wrap(domain.ErrExternal, "services.ListUtilities", "create utility manifest request", err)
	}
	req.Header.Set("User-Agent", "spktool/"+ToolVersion)

	resp, err := s.deps.http.Do(req)
	if err != nil {
		return nil, domain.Wrap(domain.ErrExternal, "services.ListUtilities", "download utility manifest", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, &domain.Error{
			Code:    domain.ErrExternal,
			Op:      "services.ListUtilities",
			Message: fmt.Sprintf("download utility manifest: unexpected status %s", resp.Status),
		}
	}

	var manifest utilityManifest
	if err := json.NewDecoder(resp.Body).Decode(&manifest); err != nil {
		return nil, domain.Wrap(domain.ErrExternal, "services.ListUtilities", "decode utility manifest", err)
	}
	if manifest.Version != sandstormUtilsManifestVersion {
		return nil, &domain.Error{
			Code:    domain.ErrUnsupported,
			Op:      "services.ListUtilities",
			Message: fmt.Sprintf("unsupported utils.json schema version %d", manifest.Version),
		}
	}
	return &manifest, nil
}

func (m *utilityManifest) findUtility(name string) (*UtilitySpec, error) {
	for i := range m.Utilities {
		if m.Utilities[i].Name == name {
			return &m.Utilities[i], nil
		}
	}
	return nil, &domain.Error{
		Code:    domain.ErrNotFound,
		Op:      "services.Add",
		Message: fmt.Sprintf("utility %q was not found in utils.json", name),
	}
}

func (r *githubRelease) assetURL(name string) (string, error) {
	for _, asset := range r.Assets {
		if asset.Name == name && strings.TrimSpace(asset.BrowserDownloadURL) != "" {
			return asset.BrowserDownloadURL, nil
		}
	}
	return "", &domain.Error{
		Code:    domain.ErrNotFound,
		Op:      "services.ListUtilities",
		Message: fmt.Sprintf("latest sandstorm-utils release %q does not contain asset %q", r.TagName, name),
	}
}

func (s *UtilityService) installUtilityFromRelease(ctx context.Context, assetURL, binary, target string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, assetURL, nil)
	if err != nil {
		return domain.Wrap(domain.ErrExternal, "services.Add", "create utility download request", err)
	}
	req.Header.Set("User-Agent", "spktool/"+ToolVersion)

	resp, err := s.deps.http.Do(req)
	if err != nil {
		return domain.Wrap(domain.ErrExternal, "services.Add", "download sandstorm-utils archive", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return &domain.Error{
			Code:    domain.ErrExternal,
			Op:      "services.Add",
			Message: fmt.Sprintf("download sandstorm-utils archive: unexpected status %s", resp.Status),
		}
	}

	gzipReader, err := gzip.NewReader(resp.Body)
	if err != nil {
		return domain.Wrap(domain.ErrExternal, "services.Add", "open sandstorm-utils archive", err)
	}
	defer gzipReader.Close()

	tarReader := tar.NewReader(gzipReader)
	found := false
	for {
		header, err := tarReader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return domain.Wrap(domain.ErrExternal, "services.Add", "read sandstorm-utils archive", err)
		}
		if !archivePathMatchesBinary(header.Name, binary) {
			continue
		}
		if header.Typeflag != tar.TypeReg && header.Typeflag != tar.TypeRegA {
			return &domain.Error{Code: domain.ErrExternal, Op: "services.Add", Message: fmt.Sprintf("binary %q is not a regular file in archive", binary)}
		}
		if err := installUtilityFile(tarReader, target); err != nil {
			return err
		}
		found = true
		break
	}
	if !found {
		return &domain.Error{
			Code:    domain.ErrNotFound,
			Op:      "services.Add",
			Message: fmt.Sprintf("binary %q was not found in sandstorm-utils archive", binary),
		}
	}
	return nil
}

func archivePathMatchesBinary(name, binary string) bool {
	clean := path.Clean(strings.TrimSpace(name))
	if clean == "." || strings.HasPrefix(clean, "../") {
		return false
	}
	return clean == path.Join("bin", binary) || strings.HasSuffix(clean, "/"+path.Join("bin", binary))
}

func installUtilityFile(src io.Reader, target string) error {
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return domain.Wrap(domain.ErrExternal, "services.Add", "create utility directory", err)
	}

	tmp, err := os.CreateTemp(filepath.Dir(target), filepath.Base(target)+".tmp-*")
	if err != nil {
		return domain.Wrap(domain.ErrExternal, "services.Add", "create temporary utility file", err)
	}
	tmpName := tmp.Name()
	defer func() {
		_ = os.Remove(tmpName)
	}()

	if _, err := io.Copy(tmp, src); err != nil {
		_ = tmp.Close()
		return domain.Wrap(domain.ErrExternal, "services.Add", "write utility file", err)
	}
	if err := tmp.Chmod(0o755); err != nil {
		_ = tmp.Close()
		return domain.Wrap(domain.ErrExternal, "services.Add", "chmod utility file", err)
	}
	if err := tmp.Close(); err != nil {
		return domain.Wrap(domain.ErrExternal, "services.Add", "close utility file", err)
	}
	if err := os.Rename(tmpName, target); err != nil {
		return domain.Wrap(domain.ErrExternal, "services.Add", "install utility file", err)
	}
	return nil
}

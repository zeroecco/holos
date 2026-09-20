package images

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"regexp"
	"strings"
	"time"
)

const (
	ubuntuStreamPath      = "releases/streams/v1/com.ubuntu.cloud:released:download.json"
	ubuntuStreamContentID = "com.ubuntu.cloud:released:download"
	ubuntuStreamMaxBytes  = 32 << 20
	ubuntuMetadataTimeout = 2 * time.Minute
)

var ubuntuBuildSerial = regexp.MustCompile(`^[0-9]{8}(\.[0-9]+)?$`)

type ubuntuStream struct {
	ContentID string                   `json:"content_id"`
	Format    string                   `json:"format"`
	Products  map[string]ubuntuProduct `json:"products"`
}

type ubuntuProduct struct {
	OS       string                   `json:"os"`
	Release  string                   `json:"release"`
	Version  string                   `json:"version"`
	Arch     string                   `json:"arch"`
	Versions map[string]ubuntuVersion `json:"versions"`
}

type ubuntuVersion struct {
	Label string                `json:"label"`
	Items map[string]ubuntuItem `json:"items"`
}

type ubuntuItem struct {
	FileType string `json:"ftype"`
	Path     string `json:"path"`
	SHA256   string `json:"sha256"`
}

// resolveUbuntuImage binds a release alias to an immutable artifact and its
// checksum from the same Simple Streams snapshot. Resolve itself remains an
// offline catalog lookup; network resolution happens only when pulling.
func resolveUbuntuImage(img *Image) (*Image, error) {
	base, err := ubuntuBaseURL(img.URL)
	if err != nil {
		return nil, err
	}
	streamURL := base.ResolveReference(&url.URL{Path: ubuntuStreamPath}).String()
	stream, err := fetchUbuntuStream(streamURL)
	if err != nil {
		return nil, err
	}
	item, err := selectUbuntuArtifact(stream, img.UbuntuRelease)
	if err != nil {
		return nil, err
	}
	artifact, err := url.Parse(item.Path)
	if err != nil || artifact.IsAbs() || artifact.Host != "" || strings.HasPrefix(item.Path, "/") {
		return nil, fmt.Errorf("Ubuntu %s image path must be relative: %q", img.UbuntuRelease, item.Path)
	}
	// Validate before resolving: ResolveReference would normalize away '..'.
	if artifact.Path != path.Clean(artifact.Path) || strings.HasPrefix(artifact.Path, "../") || artifact.Path == ".." {
		return nil, fmt.Errorf("invalid Ubuntu %s image path %q", img.UbuntuRelease, item.Path)
	}
	artifactURL := base.ResolveReference(artifact).String()
	if err := validateUbuntuArtifactURL(img.URL, img.UbuntuRelease, artifactURL); err != nil {
		return nil, err
	}
	resolved := *img
	resolved.URL = artifactURL
	resolved.SHA256 = strings.ToLower(item.SHA256)
	resolved.SHA512 = ""
	resolved.SHA256URL = ""
	resolved.SHA512URL = ""
	return &resolved, nil
}

func fetchUbuntuStream(streamURL string) (ubuntuStream, error) {
	ctx, cancel := context.WithTimeout(context.Background(), ubuntuMetadataTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, streamURL, nil)
	if err != nil {
		return ubuntuStream{}, err
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return ubuntuStream{}, fmt.Errorf("fetch Ubuntu release metadata: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return ubuntuStream{}, fmt.Errorf("HTTP %d from %s", resp.StatusCode, streamURL)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, ubuntuStreamMaxBytes+1))
	if err != nil {
		return ubuntuStream{}, fmt.Errorf("read Ubuntu release metadata: %w", err)
	}
	if len(data) > ubuntuStreamMaxBytes {
		return ubuntuStream{}, fmt.Errorf("Ubuntu release metadata exceeds %d bytes", ubuntuStreamMaxBytes)
	}
	var stream ubuntuStream
	if err := json.Unmarshal(data, &stream); err != nil {
		return ubuntuStream{}, fmt.Errorf("decode Ubuntu release metadata: %w", err)
	}
	if stream.ContentID != ubuntuStreamContentID || stream.Format != "products:1.0" {
		return ubuntuStream{}, fmt.Errorf("unexpected Ubuntu release metadata format or content ID")
	}
	return stream, nil
}

func selectUbuntuArtifact(stream ubuntuStream, release string) (ubuntuItem, error) {
	var product *ubuntuProduct
	for key, candidate := range stream.Products {
		if candidate.OS != "ubuntu" || candidate.Release != release || candidate.Arch != "amd64" || candidate.Version == "" {
			continue
		}
		if key != "com.ubuntu.cloud:server:"+candidate.Version+":amd64" {
			continue
		}
		if product != nil {
			return ubuntuItem{}, fmt.Errorf("multiple Ubuntu %s amd64 server products in release metadata", release)
		}
		product = &candidate
	}
	if product == nil {
		return ubuntuItem{}, fmt.Errorf("Ubuntu %s amd64 server image not found in release metadata", release)
	}
	// Simple Streams version keys are explicitly ordered by LANG=C sort.
	// Select the newest release first, then fail closed on incomplete metadata
	// rather than silently choosing an older build with a usable checksum.
	latest := ""
	for serial, version := range product.Versions {
		if version.Label == "release" && ubuntuBuildSerial.MatchString(serial) && serial > latest {
			latest = serial
		}
	}
	if latest == "" {
		return ubuntuItem{}, fmt.Errorf("Ubuntu %s has no released image versions", release)
	}
	item, ok := product.Versions[latest].Items["disk1.img"]
	if !ok || item.FileType != "disk1.img" {
		return ubuntuItem{}, fmt.Errorf("Ubuntu %s release %s has no disk1.img artifact", release, latest)
	}
	if len(item.SHA256) != sha256HexLength || !isHex(item.SHA256) {
		return ubuntuItem{}, fmt.Errorf("Ubuntu %s release %s disk1.img has a missing or invalid SHA256", release, latest)
	}
	if !strings.Contains(item.Path, "/release-"+latest+"/") {
		return ubuntuItem{}, fmt.Errorf("Ubuntu %s release %s has a mismatched image path %q", release, latest, item.Path)
	}
	return item, nil
}

func ubuntuBaseURL(baseURL string) (*url.URL, error) {
	base, err := url.Parse(baseURL)
	if err != nil {
		return nil, fmt.Errorf("invalid Ubuntu image base URL: %w", err)
	}
	if (base.Scheme != "https" && base.Scheme != "http") || base.Host == "" || base.User != nil || base.RawQuery != "" || base.ForceQuery || base.Fragment != "" || base.RawPath != "" {
		return nil, fmt.Errorf("invalid Ubuntu image base URL %q", baseURL)
	}
	base.Path = strings.TrimSuffix(base.Path, "/") + "/"
	if path.Clean(base.Path)+"/" != base.Path && base.Path != "/" {
		return nil, fmt.Errorf("invalid Ubuntu image base path %q", base.Path)
	}
	return base, nil
}

// validateUbuntuArtifactURL also validates persisted resolution records. Only
// dated server release images under this mirror and release can be reused.
func validateUbuntuArtifactURL(baseURL, release, artifactURL string) error {
	base, err := ubuntuBaseURL(baseURL)
	if err != nil {
		return err
	}
	artifact, err := url.Parse(artifactURL)
	if err != nil {
		return fmt.Errorf("invalid Ubuntu artifact URL: %w", err)
	}
	invalid := func() error {
		return fmt.Errorf("invalid Ubuntu %s released artifact URL %q", release, artifactURL)
	}
	if artifact.Scheme != base.Scheme || artifact.Host != base.Host || artifact.User != nil || artifact.RawQuery != "" || artifact.ForceQuery || artifact.Fragment != "" || artifact.RawPath != "" || artifact.Path != path.Clean(artifact.Path) {
		return invalid()
	}
	relative, ok := strings.CutPrefix(artifact.Path, base.Path)
	if !ok {
		return invalid()
	}
	// New streams use server/releases; older streams used releases directly.
	relative = strings.TrimPrefix(relative, "server/")
	parts := strings.Split(relative, "/")
	if len(parts) != 4 || parts[0] != "releases" || parts[1] != release || release == "" {
		return invalid()
	}
	serial, ok := strings.CutPrefix(parts[2], "release-")
	if !ok || !ubuntuBuildSerial.MatchString(serial) || !strings.HasPrefix(parts[3], "ubuntu-") || !strings.HasSuffix(parts[3], "-server-cloudimg-amd64.img") {
		return invalid()
	}
	return nil
}

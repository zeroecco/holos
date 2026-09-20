package images

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/zeroecco/holos/internal/config"
)

const (
	testUbuntuProductID = "com.ubuntu.cloud:server:24.04:amd64"
	testUbuntuSerial    = "20260911"
	testUbuntuPath      = "server/releases/noble/release-20260911/ubuntu-24.04-server-cloudimg-amd64.img"
	testUbuntuSHA256    = "612b2c0cc1bc413a6cb8c38fd611794caf0f2b436c50013d8b3794db12ad7354"
)

func ubuntuTestStream() ubuntuStream {
	return ubuntuStream{
		ContentID: ubuntuStreamContentID,
		Format:    "products:1.0",
		Products: map[string]ubuntuProduct{
			testUbuntuProductID: {
				OS: "ubuntu", Release: "noble", Version: "24.04", Arch: "amd64",
				Versions: map[string]ubuntuVersion{
					testUbuntuSerial: {
						Label: "release",
						Items: map[string]ubuntuItem{
							"disk1.img": {FileType: "disk1.img", Path: testUbuntuPath, SHA256: testUbuntuSHA256},
						},
					},
				},
			},
		},
	}
}

func TestResolveUbuntuImage(t *testing.T) {
	stream := ubuntuTestStream()
	item := stream.Products[testUbuntuProductID].Versions[testUbuntuSerial].Items["disk1.img"]
	item.SHA256 = strings.ToUpper(item.SHA256)
	stream.Products[testUbuntuProductID].Versions[testUbuntuSerial].Items["disk1.img"] = item
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if r.URL.Path != "/"+ubuntuStreamPath {
			t.Errorf("requested %q, want released metadata", r.URL.Path)
			http.NotFound(w, r)
			return
		}
		if err := json.NewEncoder(w).Encode(stream); err != nil {
			t.Errorf("encode metadata: %v", err)
		}
	}))
	t.Cleanup(server.Close)
	img := &Image{
		Name: "ubuntu", Tag: "24.04", UbuntuRelease: "noble", URL: server.URL + "/",
		Format: config.ImageFormatQCOW2, User: "ubuntu", OSFamily: config.ImageOSSystemd,
		SHA256URL: "old-checksum", SHA512: "old-hash", SHA512URL: "old-checksum",
	}
	resolved, err := resolveUbuntuImage(img)
	if err != nil {
		t.Fatal(err)
	}
	if resolved.URL != server.URL+"/"+testUbuntuPath || resolved.SHA256 != testUbuntuSHA256 {
		t.Fatalf("resolved artifact = %+v", resolved)
	}
	if resolved.Tag != img.Tag || resolved.UbuntuRelease != "noble" || resolved.User != "ubuntu" || resolved.Format != img.Format || resolved.OSFamily != img.OSFamily {
		t.Fatalf("resolver lost catalog metadata: %+v", resolved)
	}
	if resolved.SHA256URL != "" || resolved.SHA512URL != "" || resolved.SHA512 != "" {
		t.Fatalf("resolver retained stale checksum metadata: %+v", resolved)
	}
	if img.URL != server.URL+"/" || img.SHA256 != "" || img.SHA256URL != "old-checksum" {
		t.Fatalf("resolver mutated catalog entry: %+v", img)
	}
	if requests != 1 {
		t.Fatalf("requests = %d, want one metadata snapshot", requests)
	}
}

func TestSelectUbuntuArtifactNewestRelease(t *testing.T) {
	stream := ubuntuTestStream()
	product := stream.Products[testUbuntuProductID]
	for _, serial := range []string{"20260826", "20260911.1", "20260911.2"} {
		product.Versions[serial] = ubuntuVersion{
			Label: "release",
			Items: map[string]ubuntuItem{
				"disk1.img": {
					FileType: "disk1.img", SHA256: testUbuntuSHA256,
					Path: strings.Replace(testUbuntuPath, testUbuntuSerial, serial, 1),
				},
			},
		}
	}
	product.Versions["20260920"] = ubuntuVersion{Label: "daily"}
	product.Versions["not-a-build"] = ubuntuVersion{Label: "release"}
	stream.Products[testUbuntuProductID] = product
	item, err := selectUbuntuArtifact(stream, "noble")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(item.Path, "/release-20260911.2/") {
		t.Fatalf("selected %q, want newest released serial including suffix", item.Path)
	}
}

func TestSelectUbuntuArtifactIgnoresOtherProducts(t *testing.T) {
	cases := []struct {
		name   string
		key    string
		change func(*ubuntuProduct)
	}{
		{name: "architecture", key: "com.ubuntu.cloud:server:24.04:arm64", change: func(p *ubuntuProduct) { p.Arch = "arm64" }},
		{name: "amd64v3", key: "com.ubuntu.cloud:server:24.04:amd64v3", change: func(p *ubuntuProduct) { p.Arch = "amd64v3" }},
		{name: "minimal flavor", key: "com.ubuntu.cloud:server:24.04:amd64:minimal"},
		{name: "daily product", key: "com.ubuntu.cloud.daily:server:24.04:amd64"},
		{name: "different release", key: testUbuntuProductID, change: func(p *ubuntuProduct) { p.Release = "jammy" }},
		{name: "different os", key: testUbuntuProductID, change: func(p *ubuntuProduct) { p.OS = "other" }},
		{name: "missing version", key: testUbuntuProductID, change: func(p *ubuntuProduct) { p.Version = "" }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			stream := ubuntuTestStream()
			product := stream.Products[testUbuntuProductID]
			if tc.change != nil {
				tc.change(&product)
			}
			stream.Products = map[string]ubuntuProduct{tc.key: product}
			if _, err := selectUbuntuArtifact(stream, "noble"); err == nil || !strings.Contains(err.Error(), "not found") {
				t.Fatalf("error = %v, want no matching server image", err)
			}
		})
	}
}

func TestSelectUbuntuArtifactFailsOnIncompleteNewestBuild(t *testing.T) {
	cases := []struct {
		name string
		item ubuntuItem
		omit bool
		want string
	}{
		{name: "missing artifact", omit: true, want: "no disk1.img"},
		{name: "wrong artifact type", item: ubuntuItem{FileType: "uefi1.img"}, want: "no disk1.img"},
		{name: "missing hash", item: ubuntuItem{FileType: "disk1.img"}, want: "missing or invalid SHA256"},
		{name: "short hash", item: ubuntuItem{FileType: "disk1.img", SHA256: "abc"}, want: "missing or invalid SHA256"},
		{name: "nonhex hash", item: ubuntuItem{FileType: "disk1.img", SHA256: strings.Repeat("z", 64)}, want: "missing or invalid SHA256"},
		{name: "wrong build path", item: ubuntuItem{FileType: "disk1.img", SHA256: testUbuntuSHA256, Path: testUbuntuPath}, want: "mismatched image path"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			stream := ubuntuTestStream()
			version := ubuntuVersion{Label: "release", Items: map[string]ubuntuItem{}}
			if !tc.omit {
				version.Items["disk1.img"] = tc.item
			}
			stream.Products[testUbuntuProductID].Versions["20260920"] = version
			if _, err := selectUbuntuArtifact(stream, "noble"); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %v, want %q without falling back to older build", err, tc.want)
			}
		})
	}
}

func TestFetchUbuntuStreamRejectsBadResponses(t *testing.T) {
	cases := []struct {
		name   string
		status int
		body   string
		want   string
	}{
		{name: "HTTP failure", status: http.StatusNotFound, body: "not found", want: "HTTP 404"},
		{name: "malformed JSON", status: http.StatusOK, body: "{", want: "decode Ubuntu release metadata"},
		{name: "trailing JSON", status: http.StatusOK, body: `{}` + `{}`, want: "decode Ubuntu release metadata"},
		{name: "wrong feed", status: http.StatusOK, body: `{"content_id":"com.ubuntu.cloud:daily:download","format":"products:1.0"}`, want: "unexpected Ubuntu release metadata"},
		{name: "wrong format", status: http.StatusOK, body: `{"content_id":"com.ubuntu.cloud:released:download","format":"unknown"}`, want: "unexpected Ubuntu release metadata"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tc.status)
				_, _ = fmt.Fprint(w, tc.body)
			}))
			t.Cleanup(server.Close)
			if _, err := fetchUbuntuStream(server.URL); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %v, want %q", err, tc.want)
			}
		})
	}
}

func TestFetchUbuntuStreamBoundsResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		block := strings.Repeat(" ", 1<<20)
		for i := 0; i <= ubuntuStreamMaxBytes/len(block); i++ {
			if _, err := fmt.Fprint(w, block); err != nil {
				return
			}
		}
	}))
	t.Cleanup(server.Close)
	if _, err := fetchUbuntuStream(server.URL); err == nil || !strings.Contains(err.Error(), "exceeds") {
		t.Fatalf("error = %v, want response size limit", err)
	}
}

func TestValidateUbuntuArtifactURL(t *testing.T) {
	base := "https://cloud-images.ubuntu.com/"
	valid := base + testUbuntuPath
	for _, artifact := range []string{valid, strings.Replace(valid, "/server/releases/", "/releases/", 1)} {
		if err := validateUbuntuArtifactURL(base, "noble", artifact); err != nil {
			t.Errorf("valid URL %q: %v", artifact, err)
		}
	}
	cases := []string{
		strings.Replace(valid, "https://", "http://", 1),
		strings.Replace(valid, "cloud-images.ubuntu.com", "example.com", 1),
		strings.Replace(valid, "cloud-images.ubuntu.com", "user@cloud-images.ubuntu.com", 1),
		strings.Replace(valid, "/noble/", "/jammy/", 1),
		strings.Replace(valid, "/release-20260911/", "/release/", 1),
		strings.Replace(valid, "/release-20260911/", "/current/", 1),
		strings.Replace(valid, "/release-20260911/", "/release-bad/", 1),
		strings.Replace(valid, "/server/releases/", "/server/../server/releases/", 1),
		strings.Replace(valid, "/server/releases/", "/server/%2e%2e/server/releases/", 1),
		strings.Replace(valid, "/server/releases/", "//server/releases/", 1),
		strings.Replace(valid, "/server/releases/", "/server%2freleases/", 1),
		strings.Replace(valid, "-amd64.img", "-arm64.img", 1),
		strings.Replace(valid, "-amd64.img", "-amd64-uefi1.img", 1),
		valid + "?redirect=elsewhere", valid + "?", valid + "#fragment", "/" + testUbuntuPath,
	}
	for _, artifact := range cases {
		if err := validateUbuntuArtifactURL(base, "noble", artifact); err == nil {
			t.Errorf("accepted invalid artifact URL %q", artifact)
		}
	}
}

func TestResolveUbuntuImageRejectsUnsafePaths(t *testing.T) {
	for _, itemPath := range []string{
		"https://example.com/" + testUbuntuPath,
		"//example.com/" + testUbuntuPath,
		"/" + testUbuntuPath,
		"../" + testUbuntuPath,
		"discard/../" + testUbuntuPath,
		testUbuntuPath + "?download=1",
		testUbuntuPath + "#fragment",
		strings.Replace(testUbuntuPath, "noble", "jammy", 1),
	} {
		t.Run(itemPath, func(t *testing.T) {
			stream := ubuntuTestStream()
			item := stream.Products[testUbuntuProductID].Versions[testUbuntuSerial].Items["disk1.img"]
			item.Path = itemPath
			stream.Products[testUbuntuProductID].Versions[testUbuntuSerial].Items["disk1.img"] = item
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				_ = json.NewEncoder(w).Encode(stream)
			}))
			t.Cleanup(server.Close)
			if _, err := resolveUbuntuImage(&Image{URL: server.URL + "/", UbuntuRelease: "noble"}); err == nil {
				t.Fatalf("accepted unsafe path %q", itemPath)
			}
		})
	}
}

package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"

	"github.com/zeroecco/holos/internal/compose"
	"github.com/zeroecco/holos/internal/images"
)

const (
	pullMissingImageErrorMsg    = "pull requires an image name (e.g. alpine, ubuntu:noble)"
	verifyAllWithArgsErrorMsg   = "verify --all does not accept image arguments"
	verifyMissingTargetErrorMsg = "verify requires an image name, local path, or --all"
	defaultImageLockName        = "holos.images.lock"
)

func runPull(args []string) error {
	flags := newFlagSet("pull")
	stateDir := addStateDirFlag(flags)
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 1 {
		return errors.New(pullMissingImageErrorMsg)
	}

	cacheDir := images.DefaultCacheDir(*stateDir)
	path, format, err := images.Pull(flags.Arg(0), cacheDir)
	if err != nil {
		return err
	}

	fmt.Print(formatPullResult(path, format))
	return nil
}

func runVerify(args []string) error {
	flags := newFlagSet("verify")
	stateDir := addStateDirFlag(flags)
	all := flags.Bool("all", false, "verify every cached registry image with checksum metadata")
	if err := flags.Parse(args); err != nil {
		return err
	}
	refs := flags.Args()
	if *all {
		if len(refs) != 0 {
			return errors.New(verifyAllWithArgsErrorMsg)
		}
		for _, img := range images.ListAvailable() {
			refs = append(refs, imageRef(img))
		}
	} else if len(refs) == 0 {
		return errors.New(verifyMissingTargetErrorMsg)
	}

	cacheDir := images.DefaultCacheDir(*stateDir)
	for _, ref := range refs {
		res, err := images.Verify(ref, cacheDir)
		if err != nil {
			if verifyAllSkipsMissingCache(*all, err) {
				fmt.Println(formatVerifyMissingCache(ref))
				continue
			}
			return fmt.Errorf("verify %s: %w", ref, err)
		}
		if res.Skipped {
			fmt.Println(formatVerifySkipped(ref, res.Reason))
			continue
		}
		fmt.Println(formatVerifySuccess(ref, res))
	}
	return nil
}

func formatPullResult(path, format string) string {
	return fmt.Sprintf("image: %s\nformat: %s\n", path, format)
}

func formatVerifyMissingCache(ref string) string {
	return fmt.Sprintf("%s: skipped (not cached)", ref)
}

func formatVerifySkipped(ref, reason string) string {
	return fmt.Sprintf("%s: skipped (%s)", ref, reason)
}

func formatVerifySuccess(ref string, res images.Verification) string {
	return fmt.Sprintf("%s: verified %s:%s %s", ref, res.Algorithm, res.HashDisplay(), res.Path)
}

func runImages(args []string) error {
	if len(args) > 0 && args[0] == "lock" {
		return runImagesLock(args[1:])
	}
	if len(args) > 0 {
		return fmt.Errorf("usage: holos images | holos images lock -f holos.yaml [-o %s]", defaultImageLockName)
	}
	return writeImagesTable(os.Stdout, images.ListAvailable())
}

type imageLockfile struct {
	Version int              `json:"version"`
	Project string           `json:"project"`
	Images  []imageLockEntry `json:"images"`
}

type imageLockEntry struct {
	Service   string `json:"service"`
	Path      string `json:"path"`
	Format    string `json:"format"`
	SizeBytes int64  `json:"size_bytes"`
	SHA256    string `json:"sha256"`
}

func runImagesLock(args []string) error {
	flags := newFlagSet("images lock")
	projectFlags := addProjectFlags(flags, "path to holos.yaml")
	output := flags.String("o", "", "output lockfile path")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 || *projectFlags.filePath == "" {
		return fmt.Errorf("usage: holos images lock -f holos.yaml [-o %s]", defaultImageLockName)
	}

	project, composePath, err := loadProjectWithPath(*projectFlags.filePath, *projectFlags.stateDir)
	if err != nil {
		return err
	}
	lockfile, err := imageLockfileForProject(project)
	if err != nil {
		return err
	}
	outputPath := imageLockOutputPath(*output, composePath)
	if err := writeImageLockfile(outputPath, lockfile); err != nil {
		return err
	}
	fmt.Printf("wrote %s\n", outputPath)
	return nil
}

func imageLockOutputPath(outputPath, composePath string) string {
	if outputPath != "" {
		return outputPath
	}
	return filepath.Join(filepath.Dir(composePath), defaultImageLockName)
}

func imageLockfileForProject(project *compose.Project) (imageLockfile, error) {
	services := make([]string, 0, len(project.Services))
	for name := range project.Services {
		services = append(services, name)
	}
	sort.Strings(services)

	entries := make([]imageLockEntry, 0, len(services))
	for _, service := range services {
		manifest := project.Services[service]
		entry, err := imageLockEntryForService(service, manifest.Image, manifest.ImageFormat)
		if err != nil {
			return imageLockfile{}, err
		}
		entries = append(entries, entry)
	}
	return imageLockfile{
		Version: 1,
		Project: project.Name,
		Images:  entries,
	}, nil
}

func imageLockEntryForService(service, path, format string) (imageLockEntry, error) {
	info, err := os.Stat(path)
	if err != nil {
		return imageLockEntry{}, fmt.Errorf("stat image for service %q: %w", service, err)
	}
	if info.IsDir() {
		return imageLockEntry{}, fmt.Errorf("image for service %q is a directory: %s", service, path)
	}
	sum, err := sha256File(path)
	if err != nil {
		return imageLockEntry{}, fmt.Errorf("hash image for service %q: %w", service, err)
	}
	return imageLockEntry{
		Service:   service,
		Path:      path,
		Format:    format,
		SizeBytes: info.Size(),
		SHA256:    sum,
	}, nil
}

func sha256File(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()

	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", err
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

func writeImageLockfile(path string, lockfile imageLockfile) error {
	payload, err := json.MarshalIndent(lockfile, "", "  ")
	if err != nil {
		return fmt.Errorf("encode image lockfile: %w", err)
	}
	payload = append(payload, '\n')
	if err := os.WriteFile(path, payload, 0o644); err != nil {
		return fmt.Errorf("write image lockfile %s: %w", path, err)
	}
	return nil
}

func writeImagesTable(output io.Writer, available []images.Image) error {
	writer := newTableWriter(output)
	fmt.Fprintln(writer, "NAME\tTAG\tFORMAT\tOS\tVERIFY")
	for _, img := range available {
		name := img.Name
		if img.Default {
			name += " *"
		}
		verify := tablePlaceholder
		if algorithm := img.ChecksumAlgorithm(); algorithm != "" {
			verify = algorithm
		}
		fmt.Fprintf(writer, "%s\t%s\t%s\t%s\t%s\n", name, img.Tag, img.Format, img.OSFamily, verify)
	}
	return writer.Flush()
}

func verifyAllSkipsMissingCache(all bool, err error) bool {
	return all && os.IsNotExist(err)
}

func imageRef(img images.Image) string {
	return img.Name + ":" + img.Tag
}

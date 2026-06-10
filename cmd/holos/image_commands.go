package main

import (
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/zeroecco/holos/internal/images"
)

const (
	pullMissingImageErrorMsg    = "pull requires an image name (e.g. alpine, ubuntu:noble)"
	verifyAllWithArgsErrorMsg   = "verify --all does not accept image arguments"
	verifyMissingTargetErrorMsg = "verify requires an image name, local path, or --all"
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

func runImages(_ []string) error {
	return writeImagesTable(os.Stdout, images.ListAvailable())
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

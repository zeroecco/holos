package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	goruntime "runtime"
	"runtime/debug"
	"sort"
	"strings"

	"github.com/zeroecco/holos/internal/compose"
)

const (
	buildVCSRevisionKey    = "vcs.revision"
	buildVCSTimeKey        = "vcs.time"
	buildVCSModifiedKey    = "vcs.modified"
	buildCommitPlaceholder = "none"
	buildDatePlaceholder   = "unknown"
	buildModifiedTrue      = "true"
	buildDirtySuffix       = "-dirty"
)

func runValidate(args []string) error {
	flags := newFlagSet("validate")
	projectFlags := addProjectFlags(flags, "")
	if err := flags.Parse(args); err != nil {
		return err
	}

	project, err := loadProject(*projectFlags.filePath, *projectFlags.stateDir)
	if err != nil {
		return err
	}

	return writeValidateReport(os.Stdout, project)
}

func writeValidateReport(output io.Writer, project *compose.Project) error {
	fmt.Fprintf(output, "project: %s\n", project.Name)
	fmt.Fprintf(output, "spec_hash: %s\n", project.SpecHash)
	fmt.Fprintf(output, "services: %d\n", len(project.Services))
	fmt.Fprintf(output, "order: %v\n\n", project.ServiceOrder)

	writer := newTableWriter(output)
	fmt.Fprintln(writer, "SERVICE\tIMAGE\tREPLICAS\tVCPU\tMEMORY")
	for _, name := range project.ServiceOrder {
		m := project.Services[name]
		fmt.Fprintf(writer, "%s\t%s\t%d\t%d\t%dMB\n",
			name,
			filepath.Base(m.Image),
			m.Replicas,
			m.VM.VCPU,
			m.VM.MemoryMB,
		)
	}
	if err := writer.Flush(); err != nil {
		return err
	}

	fmt.Fprintf(output, "\nnetwork: %s (mcast %s:%d)\n",
		project.Network.Subnet,
		project.Network.MulticastGroup,
		project.Network.MulticastPort,
	)
	if len(project.Network.Segments) > 1 {
		fmt.Fprintln(output, "segments:")
		for _, name := range sortedNetworkSegmentNames(project.Network.Segments) {
			segment := project.Network.Segments[name]
			fmt.Fprintf(output, "  %s: %s (mcast %s:%d%s)\n",
				name,
				segment.Subnet,
				segment.MulticastGroup,
				segment.MulticastPort,
				networkSegmentBackendSuffix(segment),
			)
		}
	}
	fmt.Fprintln(output, "hosts:")
	for host, ip := range project.Network.Hosts {
		fmt.Fprintf(output, "  %s -> %s\n", host, ip)
	}
	return nil
}

func networkSegmentBackendSuffix(segment compose.NetworkSegmentPlan) string {
	if segment.Backend == "bridge" && segment.BridgeName != "" {
		return ", bridge " + segment.BridgeName
	}
	if segment.Backend == "tap" && segment.BridgeName != "" {
		return ", managed tap -> " + segment.BridgeName
	}
	return ""
}

func sortedNetworkSegmentNames(segments map[string]compose.NetworkSegmentPlan) []string {
	names := make([]string, 0, len(segments))
	for name := range segments {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func runVersion(args []string) error {
	flags := newFlagSet("version")
	short := flags.Bool("short", false, "print only the version string")
	if err := flags.Parse(args); err != nil {
		return err
	}

	build := newBuildVersion(version, commit, date)
	if info, ok := debug.ReadBuildInfo(); ok {
		for _, s := range info.Settings {
			build.applySetting(s.Key, s.Value)
		}
	}

	if *short {
		writeVersion(os.Stdout, build, true, goruntime.Version(), goruntime.GOOS, goruntime.GOARCH)
		return nil
	}
	writeVersion(os.Stdout, build, false, goruntime.Version(), goruntime.GOOS, goruntime.GOARCH)
	return nil
}

func writeVersion(output io.Writer, build buildVersion, short bool, goVersion, goos, goarch string) {
	if short {
		fmt.Fprintln(output, build.version)
		return
	}
	fmt.Fprintf(output, "holos %s\n", build.version)
	fmt.Fprintf(output, "  commit: %s\n", build.commit)
	fmt.Fprintf(output, "  built:  %s\n", build.date)
	fmt.Fprintf(output, "  go:     %s\n", goVersion)
	fmt.Fprintf(output, "  os/arch: %s/%s\n", goos, goarch)
}

type buildVersion struct {
	version string
	commit  string
	date    string
}

func newBuildVersion(version, commit, date string) buildVersion {
	return buildVersion{
		version: version,
		commit:  commit,
		date:    date,
	}
}

func (v *buildVersion) applySetting(key, value string) {
	switch key {
	case buildVCSRevisionKey:
		if buildMetadataCanAdopt(v.commit, buildCommitPlaceholder, value) {
			v.commit = value
		}
	case buildVCSTimeKey:
		if buildMetadataCanAdopt(v.date, buildDatePlaceholder, value) {
			v.date = value
		}
	case buildVCSModifiedKey:
		if buildVersionShouldAppendDirty(v.commit, value) {
			v.commit += buildDirtySuffix
		}
	}
}

func buildMetadataCanAdopt(current, placeholder, candidate string) bool {
	return current == placeholder && candidate != ""
}

func buildVersionShouldAppendDirty(commit, modified string) bool {
	return modified == buildModifiedTrue && commit != buildCommitPlaceholder && !strings.HasSuffix(commit, buildDirtySuffix)
}

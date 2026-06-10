package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/zeroecco/holos/internal/virtimport"
)

func runImport(args []string) error {
	flags := newFlagSet("import")
	output := flags.String("o", "", fmt.Sprintf("output file (default stdout; %q is stdout)", importStdoutOutput))
	projectName := flags.String("project", "", "project name (defaults to first imported domain)")
	fromXML := flags.String("xml", "", "read libvirt XML from a file instead of invoking virsh")
	connectURI := flags.String("connect", "", "libvirt connection URI passed as `virsh -c <uri>`")
	all := flags.Bool("all", false, "import every domain returned by `virsh list --all`")
	if err := flags.Parse(args); err != nil {
		return err
	}

	imported := newImportAccumulator()

	switch {
	case *fromXML != "":
		if flags.NArg() > 0 || *all {
			return errors.New("--xml cannot be combined with domain names or --all")
		}
		data, err := os.ReadFile(*fromXML)
		if err != nil {
			return fmt.Errorf("read xml: %w", err)
		}
		if err := imported.addDomain(filepath.Base(*fromXML), data); err != nil {
			return err
		}
	default:
		v := virtimport.Virsh{URI: *connectURI}
		var domains []string
		if *all {
			list, err := v.ListDomains()
			if err != nil {
				return err
			}
			domains, err = importDomainNames(true, flags.Args(), list)
			if err != nil {
				return err
			}
		} else {
			var err error
			domains, err = importDomainNames(false, flags.Args(), nil)
			if err != nil {
				return err
			}
		}
		for _, dom := range domains {
			data, err := v.DumpXML(dom)
			if err != nil {
				return err
			}
			if err := imported.addDomain(dom, data); err != nil {
				return err
			}
		}
	}

	return writeImportOutput(*output, imported.composeFile(*projectName), imported.warnings)
}

func importDomainNames(all bool, args []string, listed []string) ([]string, error) {
	switch {
	case all && len(args) > 0:
		return nil, errors.New("--all cannot be combined with explicit domain names")
	case all && len(listed) == 0:
		return nil, errors.New("virsh list --all returned no domains")
	case all:
		return listed, nil
	case len(args) > 0:
		return args, nil
	default:
		return nil, errors.New("import requires a domain name, --all, or --xml <file>")
	}
}

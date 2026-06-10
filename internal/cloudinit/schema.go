package cloudinit

// Cloud-init user-data schema.

type cloudConfig struct {
	Hostname       string   `yaml:"hostname"`
	ManageEtcHosts bool     `yaml:"manage_etc_hosts"`
	SSHPwAuth      bool     `yaml:"ssh_pwauth"`
	PackageUpdate  bool     `yaml:"package_update,omitempty"`
	Packages       []string `yaml:"packages,omitempty"`
	Users          []ccUser `yaml:"users"`
	WriteFiles     []ccFile `yaml:"write_files,omitempty"`
	BootCmd        []string `yaml:"bootcmd,omitempty"`
	RunCmd         []string `yaml:"runcmd,omitempty"`
}

type ccUser struct {
	Name              string   `yaml:"name"`
	Groups            []string `yaml:"groups,omitempty"`
	Shell             string   `yaml:"shell,omitempty"`
	Sudo              string   `yaml:"sudo,omitempty"`
	SSHAuthorizedKeys []string `yaml:"ssh_authorized_keys,omitempty"`
}

type ccFile struct {
	Path        string `yaml:"path"`
	Content     string `yaml:"content"`
	Permissions string `yaml:"permissions"`
	Owner       string `yaml:"owner"`
}

// Cloud-init network-config schema (netplan v2).

type netConfig struct {
	Network netConfigBody `yaml:"network"`
}

type netConfigBody struct {
	Version   int                    `yaml:"version"`
	Ethernets map[string]ethernetDef `yaml:"ethernets"`
}

type ethernetDef struct {
	Match       matchDef       `yaml:"match"`
	DHCP4       bool           `yaml:"dhcp4"`
	Addresses   []string       `yaml:"addresses,omitempty"`
	Nameservers *nameserverDef `yaml:"nameservers,omitempty"`
}

type matchDef struct {
	MACAddress string `yaml:"macaddress"`
}

type nameserverDef struct {
	Search []string `yaml:"search,omitempty"`
}

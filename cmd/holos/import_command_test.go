package main

import "testing"

func TestImportDomainNames(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		all     bool
		args    []string
		listed  []string
		want    []string
		wantErr string
	}{
		{name: "explicit domains", args: []string{"web", "db"}, want: []string{"web", "db"}},
		{name: "all domains", all: true, listed: []string{"web", "db"}, want: []string{"web", "db"}},
		{name: "all with explicit domains", all: true, args: []string{"web"}, wantErr: "--all cannot be combined"},
		{name: "all empty", all: true, wantErr: "returned no domains"},
		{name: "missing input", wantErr: "requires a domain name"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := importDomainNames(tt.all, tt.args, tt.listed)
			if tt.wantErr != "" {
				assertErrorContains(t, err, tt.wantErr)
				return
			}
			if err != nil {
				t.Fatalf("importDomainNames() error: %v", err)
			}
			assertStringSliceEqual(t, "importDomainNames()", got, tt.want)
		})
	}
}

func TestRunImportRejectsXMLWithOtherSources(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		args []string
	}{
		{name: "domain names", args: []string{"--xml", "domain.xml", "web"}},
		{name: "all domains", args: []string{"--xml", "domain.xml", "--all"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			assertErrorContains(t, runImport(tt.args), "--xml cannot be combined")
		})
	}
}

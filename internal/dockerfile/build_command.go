package dockerfile

// BuildCommand returns the runcmd entry that executes the generated build script.
func BuildCommand() string {
	return "bash " + buildScriptPath
}

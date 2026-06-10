package runtime

const (
	testPathRoot    = "state/holos"
	testPathProject = "demo"
	testPathService = "web"
	testPathIndex   = 2
)

func testPathWorkDir(index int) string {
	return projectInstanceDir(testPathRoot, testPathProject, testPathService, index)
}

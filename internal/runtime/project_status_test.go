package runtime

import "testing"

const (
	testStatusDBService      = "db"
	testStatusAPIService     = "api"
	testStatusMissingService = "missing"
)

func TestFindInstanceRecord(t *testing.T) {
	t.Parallel()

	const (
		webInstanceIndex = 0
		dbInstanceIndex  = 1
	)

	webServiceName := testPathService
	webInstanceName := instanceDirName(webServiceName, webInstanceIndex)
	dbServiceName := testStatusDBService
	dbInstanceName := instanceDirName(dbServiceName, dbInstanceIndex)
	record := &ProjectRecord{
		Services: []ServiceRecord{
			{
				Name: webServiceName,
				Instances: []InstanceRecord{
					{Name: webInstanceName, Index: webInstanceIndex},
				},
			},
			{
				Name: dbServiceName,
				Instances: []InstanceRecord{
					{Name: dbInstanceName, Index: dbInstanceIndex},
				},
			},
		},
	}

	inst, serviceName, ok := findInstanceRecord(record, dbInstanceName)
	if !ok {
		t.Fatal("findInstanceRecord ok = false, want true")
	}
	if serviceName != dbServiceName || inst.Name != dbInstanceName || inst.Index != dbInstanceIndex {
		t.Fatalf("findInstanceRecord = (%+v, %q), want %s in %s", inst, serviceName, dbInstanceName, dbServiceName)
	}

	missingInstanceName := instanceDirName(testStatusMissingService, webInstanceIndex)
	inst, serviceName, ok = findInstanceRecord(record, missingInstanceName)
	if ok || serviceName != "" || inst.Name != "" || inst.Index != 0 {
		t.Fatalf("findInstanceRecord miss = (%+v, %q, %v), want zero", inst, serviceName, ok)
	}
}

func TestFindServiceRecord(t *testing.T) {
	t.Parallel()

	record := &ProjectRecord{
		Services: []ServiceRecord{
			{Name: testPathService},
			{Name: testStatusDBService},
		},
	}

	service, ok := findServiceRecord(record, testStatusDBService)
	if !ok || service == nil || service.Name != testStatusDBService {
		t.Fatalf("findServiceRecord = (%+v, %v), want %s", service, ok, testStatusDBService)
	}

	service, ok = findServiceRecord(record, testStatusMissingService)
	if ok || service != nil {
		t.Fatalf("findServiceRecord miss = (%+v, %v), want nil false", service, ok)
	}
}

func TestInstanceNotFoundError(t *testing.T) {
	t.Parallel()

	err := instanceNotFoundError("demo", "web-2")
	want := `instance "web-2" not found in project "demo"`
	if got := err.Error(); got != want {
		t.Fatalf("instanceNotFoundError = %q, want %q", got, want)
	}
}

func TestServiceNotFoundError(t *testing.T) {
	t.Parallel()

	err := serviceNotFoundError("demo", "api")
	want := `service "api" not found in project "demo"`
	if got := err.Error(); got != want {
		t.Fatalf("serviceNotFoundError = %q, want %q", got, want)
	}
}

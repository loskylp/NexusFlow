// Package system — TASK-030 system tests: MinIO startup and worker registration log.
//
// Requirement: DEMO-001, TASK-030
//
// These tests exercise AC-1 at the system level: they verify that:
//   - MinIO starts and becomes healthy via `docker compose --profile demo up`
//   - The worker logs the expected registration message when MINIO_ENDPOINT is set
//   - The worker logs the expected skip message when MINIO_ENDPOINT is unset
//
// Tests run against the live Docker Compose environment. Skipped unless
// SYSTEM_TEST_DOCKER_COMPOSE is set to "true" (these tests mutate running containers
// and are not safe to run in parallel with other compose stacks on the same host).
//
// Run:
//
//	SYSTEM_TEST_DOCKER_COMPOSE=true go test ./tests/system/... -v -run TASK030 -timeout 120s
package system

import (
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// AC-1: MinIO starts via `docker compose --profile demo up`
// ---------------------------------------------------------------------------

// TestTASK030_AC1_MinIOHealthCheckPassesAfterDockerComposeUp verifies that the MinIO
// container starts and its /minio/health/live endpoint returns HTTP 200 when the
// demo profile is active.
//
// DEMO-001 / TASK-030 AC-1: MinIO starts via `docker compose --profile demo up`.
//
// Given: docker compose is available and the demo profile is used
// When:  the minio service has started and the healthcheck interval has elapsed
// Then:  GET http://localhost:9000/minio/health/live returns HTTP 200
func TestTASK030_AC1_MinIOHealthCheckPassesAfterDockerComposeUp(t *testing.T) {
	if os.Getenv("SYSTEM_TEST_DOCKER_COMPOSE") != "true" {
		t.Skip("SYSTEM_TEST_DOCKER_COMPOSE not set to 'true' — skipping compose system tests")
	}

	// Poll the healthcheck endpoint; MinIO may still be starting.
	deadline := time.Now().Add(60 * time.Second)
	var lastCode string
	for time.Now().Before(deadline) {
		out, err := exec.Command("curl", "-s", "-o", "/dev/null", "-w", "%{http_code}",
			"http://localhost:9000/minio/health/live").Output()
		if err == nil {
			lastCode = strings.TrimSpace(string(out))
			if lastCode == "200" {
				return // PASS
			}
		}
		time.Sleep(2 * time.Second)
	}
	t.Errorf("AC-1 FAIL: MinIO health endpoint did not return 200 within 60s (last: %s)", lastCode)
}

// TestTASK030_AC1_MinIOSeedBucketsExist verifies that the minio-init service creates
// demo-input and demo-output buckets and seeds demo-input with 3 records.
//
// DEMO-001 / TASK-030 AC-1: demo-input seeded with sample data; demo-output ready.
//
// Given: minio-init has completed (condition: service_healthy on minio)
// When:  mc ls is used to list demo-input/data/ objects
// Then:  3 objects are present (record-001.json, record-002.json, record-003.json)
func TestTASK030_AC1_MinIOSeedBucketsExist(t *testing.T) {
	if os.Getenv("SYSTEM_TEST_DOCKER_COMPOSE") != "true" {
		t.Skip("SYSTEM_TEST_DOCKER_COMPOSE not set to 'true' — skipping compose system tests")
	}

	// Use mc via docker run to list objects in demo-input/data/
	out, err := exec.Command(
		"docker", "run", "--rm", "--network", "nexusflow_internal",
		"--entrypoint", "/bin/sh",
		"minio/mc:latest",
		"-c",
		"mc alias set local http://minio:9000 minioadmin minioadmin && mc ls local/demo-input/data/",
	).CombinedOutput()
	if err != nil {
		t.Fatalf("AC-1 FAIL: mc ls command failed: %v\nOutput: %s", err, out)
	}

	output := string(out)
	for _, name := range []string{"record-001.json", "record-002.json", "record-003.json"} {
		if !strings.Contains(output, name) {
			t.Errorf("AC-1 FAIL: expected seed file %q not found in mc ls output:\n%s", name, output)
		}
	}
}

// TestTASK030_AC1_WorkerLogsMinIORegistrationOnStartup verifies that when
// MINIO_ENDPOINT is set, the worker logs the expected registration confirmation.
//
// DEMO-001 / TASK-030 AC-1 / Builder integration check #3.
//
// Given: docker compose --profile demo is running with MINIO_ENDPOINT=http://minio:9000
// When:  the worker container starts
// Then:  worker logs contain "worker: MinIO connectors registered (endpoint=minio:9000 ssl=false)"
func TestTASK030_AC1_WorkerLogsMinIORegistrationOnStartup(t *testing.T) {
	if os.Getenv("SYSTEM_TEST_DOCKER_COMPOSE") != "true" {
		t.Skip("SYSTEM_TEST_DOCKER_COMPOSE not set to 'true' — skipping compose system tests")
	}

	out, err := exec.Command(
		"docker", "compose",
		"--profile", "demo",
		"-f", "/Users/pablo/projects/Nexus/NexusTests/NexusFlow/docker-compose.yml",
		"logs", "worker", "--no-log-prefix",
	).CombinedOutput()
	if err != nil {
		t.Fatalf("AC-1 FAIL: docker compose logs worker: %v\nOutput: %s", err, out)
	}

	const expectedLog = "worker: MinIO connectors registered (endpoint=minio:9000 ssl=false)"
	if !strings.Contains(string(out), expectedLog) {
		t.Errorf("AC-1 FAIL: worker log does not contain expected line %q\nFull log:\n%s",
			expectedLog, string(out))
	}
}

// TestTASK030_AC1_WorkerLogsSkipWhenMINIOEndpointUnset verifies the negative case:
// when MINIO_ENDPOINT is absent, the worker logs the expected skip warning and
// continues to start normally (non-demo deployments are unaffected).
//
// DEMO-001 / TASK-030 AC-1 / Builder integration check #4 (negative path).
//
// Given: docker compose is running WITHOUT --profile demo (MINIO_ENDPOINT not set)
// When:  the worker starts
// Then:  worker logs contain "worker: MINIO_ENDPOINT not set — MinIO connectors not registered"
//
// [VERIFIER-ADDED]: the skip-when-unset path is critical for non-demo deployments;
// a regression here would break production worker startup.
func TestTASK030_AC1_WorkerLogsSkipWhenMINIOEndpointUnset(t *testing.T) {
	if os.Getenv("SYSTEM_TEST_DOCKER_COMPOSE") != "true" {
		t.Skip("SYSTEM_TEST_DOCKER_COMPOSE not set to 'true' — skipping compose system tests")
	}

	out, err := exec.Command(
		"docker", "compose",
		"-f", "/Users/pablo/projects/Nexus/NexusTests/NexusFlow/docker-compose.yml",
		"logs", "worker", "--no-log-prefix",
	).CombinedOutput()
	if err != nil {
		t.Fatalf("AC-1 FAIL: docker compose logs worker (no demo): %v\nOutput: %s", err, out)
	}

	const expectedLog = "worker: MINIO_ENDPOINT not set — MinIO connectors not registered"
	if !strings.Contains(string(out), expectedLog) {
		t.Errorf("AC-1 FAIL (negative): worker log should contain %q when MINIO_ENDPOINT is unset\nFull log:\n%s",
			expectedLog, string(out))
	}
}

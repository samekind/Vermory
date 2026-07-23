package operationsprofile

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"
)

var errProfileDisabled = errors.New("HA/PITR profile is disabled")

var postgresVersionPattern = regexp.MustCompile(`PostgreSQL\) ([0-9]+\.[0-9]+)`) // PostgreSQL tool output is stable across supported commands.

type profileConfig struct {
	BinDir            string
	Root              string
	RunID             string
	PostgreSQLVersion string
}

type postgresCluster struct {
	Name      string
	DataDir   string
	LogPath   string
	Port      int
	ProcessUp bool
}

type clusterHarness struct {
	Config     profileConfig
	Primary    postgresCluster
	Standby    postgresCluster
	PITRBase   string
	PITR       postgresCluster
	WALArchive string
}

func loadProfileConfigFromEnv(getenv func(string) string) (profileConfig, error) {
	if strings.TrimSpace(getenv("VERMORY_HA_PITR_PROFILE")) != "1" {
		return profileConfig{}, errProfileDisabled
	}
	binDir := strings.TrimSpace(getenv("VERMORY_POSTGRES_BIN_DIR"))
	root := strings.TrimSpace(getenv("VERMORY_HA_PITR_ROOT"))
	runID := strings.TrimSpace(getenv("VERMORY_HA_PITR_RUN_ID"))
	if binDir == "" || root == "" {
		return profileConfig{}, errors.New("VERMORY_POSTGRES_BIN_DIR and VERMORY_HA_PITR_ROOT are required")
	}
	if runID == "" {
		runID = filepath.Base(filepath.Clean(root))
	}
	if !safeNamePattern.MatchString(runID) {
		return profileConfig{}, errors.New("VERMORY_HA_PITR_RUN_ID is invalid")
	}
	absBin, err := filepath.Abs(binDir)
	if err != nil {
		return profileConfig{}, fmt.Errorf("resolve PostgreSQL bin directory: %w", err)
	}
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return profileConfig{}, fmt.Errorf("resolve profile root: %w", err)
	}
	if err := validateDedicatedRoot(absRoot); err != nil {
		return profileConfig{}, err
	}
	postgresVersion, err := readPostgresToolVersion(filepath.Join(absBin, "postgres"))
	if err != nil {
		return profileConfig{}, err
	}
	basebackupVersion, err := readPostgresToolVersion(filepath.Join(absBin, "pg_basebackup"))
	if err != nil {
		return profileConfig{}, err
	}
	if !strings.HasPrefix(postgresVersion, "18.") || !strings.HasPrefix(basebackupVersion, "18.") {
		return profileConfig{}, fmt.Errorf("PostgreSQL 18 tools are required: postgres=%s pg_basebackup=%s", postgresVersion, basebackupVersion)
	}
	if postgresVersion != basebackupVersion {
		return profileConfig{}, fmt.Errorf("PostgreSQL tool versions differ: postgres=%s pg_basebackup=%s", postgresVersion, basebackupVersion)
	}
	if err := os.MkdirAll(absRoot, 0o700); err != nil {
		return profileConfig{}, fmt.Errorf("create dedicated profile root: %w", err)
	}
	return profileConfig{BinDir: absBin, Root: absRoot, RunID: runID, PostgreSQLVersion: postgresVersion}, nil
}

func validateDedicatedRoot(root string) error {
	if !filepath.IsAbs(root) || filepath.Clean(root) == string(filepath.Separator) {
		return errors.New("VERMORY_HA_PITR_ROOT must be a dedicated absolute directory")
	}
	if strings.ContainsAny(root, "'\n\r\x00") {
		return errors.New("VERMORY_HA_PITR_ROOT contains unsupported characters")
	}
	if info, err := os.Lstat(root); err == nil {
		if info.Mode()&os.ModeSymlink != 0 {
			return errors.New("VERMORY_HA_PITR_ROOT must not be a symlink")
		}
		if _, err := os.Stat(filepath.Join(root, "PG_VERSION")); err == nil {
			return errors.New("VERMORY_HA_PITR_ROOT points at a PostgreSQL data directory")
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("inspect profile root: %w", err)
	}
	return nil
}

func readPostgresToolVersion(path string) (string, error) {
	command := exec.Command(path, "--version")
	output, err := command.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("execute %s --version: %w", filepath.Base(path), err)
	}
	match := postgresVersionPattern.FindStringSubmatch(string(output))
	if len(match) != 2 {
		return "", fmt.Errorf("parse %s version", filepath.Base(path))
	}
	return match[1], nil
}

func ensureContainedPath(root, candidate string) error {
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return fmt.Errorf("resolve root: %w", err)
	}
	absCandidate, err := filepath.Abs(candidate)
	if err != nil {
		return fmt.Errorf("resolve candidate: %w", err)
	}
	relative, err := filepath.Rel(absRoot, absCandidate)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return fmt.Errorf("path %q is outside dedicated root", candidate)
	}
	current := absRoot
	if relative == "." {
		return nil
	}
	for _, component := range strings.Split(relative, string(filepath.Separator)) {
		current = filepath.Join(current, component)
		info, err := os.Lstat(current)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return fmt.Errorf("inspect contained path: %w", err)
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("path %q contains a symlink below the dedicated root", candidate)
		}
	}
	return nil
}

func renderPrimaryConfig(port int, archiveDir string) (string, error) {
	if err := validateConfigInputs(port, archiveDir); err != nil {
		return "", err
	}
	archiveCommand := fmt.Sprintf(`test -f "%s/%%f" || cp "%%p" "%s/%%f"`, archiveDir, archiveDir)
	return strings.Join([]string{
		"listen_addresses = '127.0.0.1'",
		"port = " + strconv.Itoa(port),
		"unix_socket_directories = ''",
		"wal_level = replica",
		"max_wal_senders = 4",
		"max_replication_slots = 4",
		"hot_standby = on",
		"archive_mode = on",
		"archive_command = '" + archiveCommand + "'",
		"fsync = on",
		"full_page_writes = on",
		"synchronous_commit = on",
		"logging_collector = off",
		"log_min_messages = warning",
		"",
	}, "\n"), nil
}

func renderStandbyConfig(port int) (string, error) {
	if err := validateConfigInputs(port); err != nil {
		return "", err
	}
	return strings.Join([]string{
		"listen_addresses = '127.0.0.1'",
		"port = " + strconv.Itoa(port),
		"unix_socket_directories = ''",
		"hot_standby = on",
		"logging_collector = off",
		"log_min_messages = warning",
		"",
	}, "\n"), nil
}

func renderPITRConfig(port int, archiveDir, targetLSN string) (string, error) {
	if err := validateConfigInputs(port, archiveDir); err != nil {
		return "", err
	}
	if !regexp.MustCompile(`^[0-9A-F]+/[0-9A-F]+$`).MatchString(targetLSN) {
		return "", errors.New("target LSN is invalid")
	}
	restoreCommand := fmt.Sprintf(`cp "%s/%%f" "%%p"`, archiveDir)
	return strings.Join([]string{
		"listen_addresses = '127.0.0.1'",
		"port = " + strconv.Itoa(port),
		"unix_socket_directories = ''",
		"restore_command = '" + restoreCommand + "'",
		"recovery_target_lsn = '" + targetLSN + "'",
		"recovery_target_timeline = 'current'",
		"recovery_target_inclusive = on",
		"recovery_target_action = promote",
		"logging_collector = off",
		"log_min_messages = warning",
		"",
	}, "\n"), nil
}

func validateConfigInputs(port int, paths ...string) error {
	if port <= 0 || port > 65535 {
		return errors.New("PostgreSQL port is invalid")
	}
	for _, path := range paths {
		if !filepath.IsAbs(path) || strings.ContainsAny(path, "'\n\r\x00") {
			return fmt.Errorf("PostgreSQL config path %q is invalid", path)
		}
	}
	return nil
}

func newClusterHarness(t *testing.T, config profileConfig) *clusterHarness {
	t.Helper()
	primaryPort := allocateLoopbackPort(t)
	standbyPort := allocateLoopbackPort(t)
	pitrPort := allocateLoopbackPort(t)
	logRoot := filepath.Join(config.Root, "logs")
	harness := &clusterHarness{
		Config: config,
		Primary: postgresCluster{
			Name: "primary", DataDir: filepath.Join(config.Root, "primary"), LogPath: filepath.Join(logRoot, "primary.log"), Port: primaryPort,
		},
		Standby: postgresCluster{
			Name: "standby", DataDir: filepath.Join(config.Root, "standby"), LogPath: filepath.Join(logRoot, "standby.log"), Port: standbyPort,
		},
		PITRBase: filepath.Join(config.Root, "pitr-base"),
		PITR: postgresCluster{
			Name: "pitr-restored", DataDir: filepath.Join(config.Root, "pitr-restored"), LogPath: filepath.Join(logRoot, "pitr.log"), Port: pitrPort,
		},
		WALArchive: filepath.Join(config.Root, "wal-archive"),
	}
	for _, path := range []string{harness.Primary.DataDir, harness.Standby.DataDir, harness.PITRBase, harness.PITR.DataDir, harness.WALArchive, logRoot, filepath.Join(config.Root, "artifacts")} {
		if err := ensureContainedPath(config.Root, path); err != nil {
			t.Fatal(err)
		}
	}
	for _, dataDir := range []string{harness.Primary.DataDir, harness.Standby.DataDir, harness.PITRBase, harness.PITR.DataDir} {
		if _, err := os.Stat(filepath.Join(dataDir, "PG_VERSION")); err == nil {
			t.Fatalf("dedicated cluster directory already contains PostgreSQL data: %s", dataDir)
		}
	}
	if err := os.MkdirAll(logRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(harness.WALArchive, 0o700); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		harness.stopCluster(t, &harness.PITR, "fast")
		harness.stopCluster(t, &harness.Standby, "fast")
		harness.stopCluster(t, &harness.Primary, "fast")
	})
	return harness
}

func (h *clusterHarness) runReplicationSmoke(t *testing.T) {
	t.Helper()
	h.initializePrimary(t)
	h.startCluster(t, &h.Primary)
	h.execSQL(t, h.Primary, "CREATE ROLE vermory_i03_repl WITH REPLICATION LOGIN")
	h.execSQL(t, h.Primary, "CREATE TABLE i03_replication_smoke (id integer PRIMARY KEY, value text NOT NULL)")
	h.execSQL(t, h.Primary, "INSERT INTO i03_replication_smoke VALUES (1, 'base-backup-row')")
	h.createStreamingStandby(t, "vermory_i03_repl")
	h.startCluster(t, &h.Standby)
	h.execSQL(t, h.Primary, "INSERT INTO i03_replication_smoke VALUES (2, 'streamed-row')")
	h.waitForQuery(t, h.Standby, "SELECT count(*) FROM i03_replication_smoke", "2", 20*time.Second)
	primaryID := h.systemIdentifier(t, h.Primary)
	standbyID := h.systemIdentifier(t, h.Standby)
	if primaryID == "" || primaryID != standbyID {
		t.Fatalf("system identifier mismatch: primary=%q standby=%q", primaryID, standbyID)
	}
}

func (h *clusterHarness) initializePrimary(t *testing.T) {
	t.Helper()
	h.runTool(t, 60*time.Second, "initdb", "-D", h.Primary.DataDir, "-U", "postgres", "--no-locale", "--encoding=UTF8", "--auth-local=trust", "--auth-host=trust")
	config, err := renderPrimaryConfig(h.Primary.Port, h.WALArchive)
	if err != nil {
		t.Fatal(err)
	}
	appendFile(t, filepath.Join(h.Primary.DataDir, "postgresql.conf"), config)
	appendFile(t, filepath.Join(h.Primary.DataDir, "pg_hba.conf"), "host replication all 127.0.0.1/32 trust\nhost all all 127.0.0.1/32 trust\n")
}

func (h *clusterHarness) createStreamingStandby(t *testing.T, replicationUser string) {
	t.Helper()
	h.runTool(t, 120*time.Second, "pg_basebackup", "-D", h.Standby.DataDir, "-h", "127.0.0.1", "-p", strconv.Itoa(h.Primary.Port), "-U", replicationUser, "-X", "stream", "-R", "--checkpoint=fast")
	config, err := renderStandbyConfig(h.Standby.Port)
	if err != nil {
		t.Fatal(err)
	}
	appendFile(t, filepath.Join(h.Standby.DataDir, "postgresql.conf"), config)
}

func (h *clusterHarness) createPITRBase(t *testing.T, replicationUser string) {
	t.Helper()
	h.runTool(t, 120*time.Second, "pg_basebackup", "-D", h.PITRBase, "-h", "127.0.0.1", "-p", strconv.Itoa(h.Primary.Port), "-U", replicationUser, "-X", "stream", "--checkpoint=fast")
}

func (h *clusterHarness) startCluster(t *testing.T, cluster *postgresCluster) {
	t.Helper()
	h.runTool(t, 30*time.Second, "pg_ctl", "-D", cluster.DataDir, "-l", cluster.LogPath, "-w", "-t", "20", "start")
	cluster.ProcessUp = true
}

func (h *clusterHarness) stopCluster(t *testing.T, cluster *postgresCluster, mode string) {
	t.Helper()
	if !cluster.ProcessUp {
		return
	}
	command := exec.Command(h.tool("pg_ctl"), "-D", cluster.DataDir, "-w", "-t", "20", "stop", "-m", mode)
	if output, err := command.CombinedOutput(); err != nil && !strings.Contains(string(output), "no server running") {
		t.Errorf("stop dedicated %s cluster: %v: %s", cluster.Name, err, redactedTail(string(output), 2000))
	}
	cluster.ProcessUp = false
}

func (h *clusterHarness) promoteCluster(t *testing.T, cluster *postgresCluster) {
	t.Helper()
	if !cluster.ProcessUp {
		t.Fatalf("cannot promote stopped %s cluster", cluster.Name)
	}
	h.runTool(t, 30*time.Second, "pg_ctl", "-D", cluster.DataDir, "-w", "-t", "20", "promote")
	h.waitForQuery(t, *cluster, "SELECT pg_is_in_recovery()", "f", 20*time.Second)
	h.waitForQuery(t, *cluster, "SELECT current_setting('transaction_read_only')", "off", 20*time.Second)
}

func (h *clusterHarness) currentFlushLSN(t *testing.T, cluster postgresCluster) string {
	t.Helper()
	return h.execSQL(t, cluster, "SELECT pg_current_wal_flush_lsn()")
}

func (h *clusterHarness) currentReplayLSN(t *testing.T, cluster postgresCluster) string {
	t.Helper()
	return h.execSQL(t, cluster, "SELECT pg_last_wal_replay_lsn()")
}

func (h *clusterHarness) waitForReplayLSN(t *testing.T, cluster postgresCluster, targetLSN string, timeout time.Duration) {
	t.Helper()
	if !regexp.MustCompile(`^[0-9A-F]+/[0-9A-F]+$`).MatchString(targetLSN) {
		t.Fatalf("invalid replay target LSN %q", targetLSN)
	}
	h.waitForQuery(t, cluster, "SELECT COALESCE(pg_last_wal_replay_lsn() >= '"+targetLSN+"'::pg_lsn, false)", "t", timeout)
}

func (h *clusterHarness) forceArchiveCurrentSegment(t *testing.T, cluster postgresCluster) string {
	t.Helper()
	segment := h.execSQL(t, cluster, "SELECT pg_walfile_name(pg_current_wal_lsn())")
	if !regexp.MustCompile(`^[0-9A-F]{24}$`).MatchString(segment) {
		t.Fatalf("unexpected WAL segment name %q", segment)
	}
	h.execSQL(t, cluster, "SELECT pg_switch_wal()")
	path := filepath.Join(h.WALArchive, segment)
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		if info, err := os.Stat(path); err == nil && info.Mode().IsRegular() && info.Size() > 0 {
			return segment
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("WAL segment %s was not archived", segment)
	return ""
}

func (h *clusterHarness) execSQL(t *testing.T, cluster postgresCluster, sql string) string {
	t.Helper()
	return strings.TrimSpace(h.runTool(t, 30*time.Second, "psql", "-h", "127.0.0.1", "-p", strconv.Itoa(cluster.Port), "-U", "postgres", "-d", "postgres", "-v", "ON_ERROR_STOP=1", "-Atc", sql))
}

func (h *clusterHarness) waitForQuery(t *testing.T, cluster postgresCluster, sql, expected string, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	var last string
	for time.Now().Before(deadline) {
		command := exec.Command(h.tool("psql"), "-h", "127.0.0.1", "-p", strconv.Itoa(cluster.Port), "-U", "postgres", "-d", "postgres", "-Atc", sql)
		output, err := command.CombinedOutput()
		last = strings.TrimSpace(string(output))
		if err == nil && last == expected {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("query did not reach %q on %s, last=%q", expected, cluster.Name, redactedTail(last, 1000))
}

func (h *clusterHarness) systemIdentifier(t *testing.T, cluster postgresCluster) string {
	t.Helper()
	output := h.runTool(t, 30*time.Second, "pg_controldata", cluster.DataDir)
	for _, line := range strings.Split(output, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "Database system identifier:") {
			return strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(line), "Database system identifier:"))
		}
	}
	t.Fatalf("pg_controldata omitted system identifier for %s", cluster.Name)
	return ""
}

func (h *clusterHarness) runTool(t *testing.T, timeout time.Duration, name string, args ...string) string {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	command := exec.CommandContext(ctx, h.tool(name), args...)
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("%s failed: %v: %s", name, err, redactedTail(string(output), 4000))
	}
	return string(output)
}

func (h *clusterHarness) tool(name string) string {
	return filepath.Join(h.Config.BinDir, name)
}

func allocateLoopbackPort(t *testing.T) int {
	t.Helper()
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	if err := listener.Close(); err != nil {
		t.Fatal(err)
	}
	return port
}

func appendFile(t *testing.T, path, content string) {
	t.Helper()
	file, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := file.WriteString(content); err != nil {
		file.Close()
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
}

func redactedTail(value string, limit int) string {
	value = strings.ReplaceAll(value, "\r", "")
	for _, marker := range []string{"postgresql://", "password=", "vmt_", "sk-", "authorization:"} {
		if index := strings.Index(strings.ToLower(value), marker); index >= 0 {
			value = value[:index] + "[redacted]"
		}
	}
	if len(value) > limit {
		value = value[len(value)-limit:]
	}
	return strings.TrimSpace(value)
}

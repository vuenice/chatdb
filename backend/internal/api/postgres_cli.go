package api

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"
	"unicode"

	"chatdb/internal/store"
)

const (
	exportFormatPlain   = "plain"
	exportFormatArchive = "archive"
	importFormatPsql    = "psql"
	importFormatPgdump  = "pgdump"
	exportScopeBoth     = "both"
	exportScopeSchema   = "schema"
	exportScopeData     = "data"
)

func postgresSSLMode(sslmode string) string {
	if strings.TrimSpace(sslmode) == "" {
		return "disable"
	}
	return strings.TrimSpace(sslmode)
}

func sanitizedExportBase(name string) string {
	var b strings.Builder
	for _, r := range name {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' || r == '-' {
			b.WriteRune(r)
		}
	}
	out := strings.Trim(b.String(), "-_")
	if out == "" {
		return "database"
	}
	return out
}

func (s *Server) decryptedConnAuth(c *store.DbConnection, write bool) (username, password string, err error) {
	username = c.ReadUsername
	encPass := c.ReadPassword
	if write && c.WriteUsername != "" {
		username = c.WriteUsername
		encPass = c.WritePassword
	}
	password, err = s.Crypter.Decrypt(encPass)
	if err != nil {
		return "", "", err
	}
	return username, password, nil
}

func postgresCmdEnv(password, sslmode string) []string {
	return append(os.Environ(),
		"PGPASSWORD="+password,
		"PGSSLMODE="+postgresSSLMode(sslmode),
	)
}

func parseExportScope(raw string) (string, error) {
	v := strings.ToLower(strings.TrimSpace(raw))
	if v == "" {
		return exportScopeBoth, nil
	}
	switch v {
	case exportScopeBoth, exportScopeSchema, exportScopeData:
		return v, nil
	default:
		return "", fmt.Errorf("export scope must be both, schema, or data")
	}
}

// runPgDump writes a dump to outPath. If customFormat, uses -Fc (pg_restore-compatible).
// scope must be exportScopeBoth, exportScopeSchema, or exportScopeData.
func runPgDump(ctx context.Context, pgDumpPath string, conn *store.DbConnection, dbName, user, password string, customFormat bool, scope string, outPath string) error {
	args := []string{
		"-h", conn.Host,
		"-p", strconv.Itoa(conn.Port),
		"-U", user,
		"--no-owner",
	}
	switch scope {
	case exportScopeSchema:
		args = append(args, "--schema-only")
	case exportScopeData:
		args = append(args, "--data-only")
	case exportScopeBoth:
	default:
		return fmt.Errorf("invalid export scope: %s", scope)
	}
	args = append(args, "-d", dbName, "-f", outPath)
	if customFormat {
		args = append([]string{"-Fc"}, args...)
	}

	cmd := exec.CommandContext(ctx, pgDumpPath, args...)
	cmd.Env = postgresCmdEnv(password, conn.SslMode)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%w: %s", err, strings.TrimSpace(stderr.String()))
	}
	return nil
}

func runPsqlFile(ctx context.Context, psqlPath string, conn *store.DbConnection, dbName, user, password, sqlPath string) error {
	args := []string{
		"-h", conn.Host,
		"-p", strconv.Itoa(conn.Port),
		"-U", user,
		"-d", dbName,
		"-v", "ON_ERROR_STOP=1",
		"-f", sqlPath,
		"-q",
	}
	cmd := exec.CommandContext(ctx, psqlPath, args...)
	cmd.Env = postgresCmdEnv(password, conn.SslMode)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%w: %s", err, strings.TrimSpace(stderr.String()))
	}
	return nil
}

func runPgRestoreFile(ctx context.Context, restorePath string, conn *store.DbConnection, dbName, user, password, dumpPath string) error {
	args := []string{
		"-h", conn.Host,
		"-p", strconv.Itoa(conn.Port),
		"-U", user,
		"-d", dbName,
		"--no-owner",
		"-v",
		dumpPath,
	}
	cmd := exec.CommandContext(ctx, restorePath, args...)
	cmd.Env = postgresCmdEnv(password, conn.SslMode)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%w: %s", err, strings.TrimSpace(stderr.String()))
	}
	return nil
}

// saveUploadToTemp writes src to a closed temp file path (for psql -f / pg_restore).
func saveUploadToTemp(src io.Reader, pattern string) (path string, err error) {
	f, err := os.CreateTemp("", pattern)
	if err != nil {
		return "", err
	}
	tmpPath := f.Name()
	_, err = io.Copy(f, src)
	closeErr := f.Close()
	if err == nil {
		err = closeErr
	}
	if err != nil {
		_ = os.Remove(tmpPath)
		return "", err
	}
	return tmpPath, nil
}

func exportContext(parent context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(parent, 60*time.Minute)
}

func lookPostgresTool(name string) (string, error) {
	p, err := exec.LookPath(name)
	if err != nil {
		return "", fmt.Errorf("%s not found on PATH (install postgresql-client)", name)
	}
	return p, nil
}

var errWrongImportFormat = errors.New("import format must be psql (plain .sql) or pgdump (custom archive) for PostgreSQL")

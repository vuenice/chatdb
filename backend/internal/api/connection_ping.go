package api

import (
	"context"

	"chatdb/internal/config"
	"chatdb/internal/engine"
)

// pingTargetDB verifies read credentials against the given database (physical DB name).
func pingTargetDB(ctx context.Context, driver, host string, port int, database, sslMode, readUser, readPass string) error {
	switch config.Driver(driver) {
	case config.DriverMySQL:
		e, err := engine.OpenMySQL(host, port, readUser, readPass, database)
		if err != nil {
			return err
		}
		defer e.Close()
		return e.Ping(ctx)
	default:
		e, err := engine.OpenPostgres(host, port, readUser, readPass, database, sslMode)
		if err != nil {
			return err
		}
		defer e.Close()
		return e.Ping(ctx)
	}
}

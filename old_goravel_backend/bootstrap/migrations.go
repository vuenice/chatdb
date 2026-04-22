package bootstrap

import (
	"github.com/goravel/framework/contracts/database/schema"

	"goravel/database/migrations"
)

func Migrations() []schema.Migration {
	return []schema.Migration{
		&migrations.M20210101000001CreateJobsTable{},
		&migrations.M20260219100001CreateUsersTable{},
		&migrations.M20260219100002CreateDbConnectionsTable{},
		&migrations.M20260219100003CreateSavedQueriesTable{},
		&migrations.M20260219100004CreateAuditLogsTable{},
	}
}

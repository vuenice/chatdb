package migrations

import (
	"github.com/goravel/framework/contracts/database/schema"

	"goravel/app/facades"
)

type M20260219100004CreateAuditLogsTable struct{}

func (r *M20260219100004CreateAuditLogsTable) Signature() string {
	return "20260219100004_create_audit_logs_table"
}

func (r *M20260219100004CreateAuditLogsTable) Up() error {
	if facades.Schema().HasTable("audit_logs") {
		return nil
	}
	return facades.Schema().Create("audit_logs", func(table schema.Blueprint) {
		table.ID()
		table.UnsignedBigInteger("user_id").Nullable()
		table.UnsignedBigInteger("connection_id").Nullable()
		table.String("action")
		table.String("pool").Default("")
		table.Text("sql_snippet").Default("")
		table.Text("meta").Nullable()
		table.TimestampsTz()
		table.Index("user_id")
	})
}

func (r *M20260219100004CreateAuditLogsTable) Down() error {
	return facades.Schema().DropIfExists("audit_logs")
}

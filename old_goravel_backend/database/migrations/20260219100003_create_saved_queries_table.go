package migrations

import (
	"github.com/goravel/framework/contracts/database/schema"

	"goravel/app/facades"
)

type M20260219100003CreateSavedQueriesTable struct{}

func (r *M20260219100003CreateSavedQueriesTable) Signature() string {
	return "20260219100003_create_saved_queries_table"
}

func (r *M20260219100003CreateSavedQueriesTable) Up() error {
	if facades.Schema().HasTable("saved_queries") {
		return nil
	}
	return facades.Schema().Create("saved_queries", func(table schema.Blueprint) {
		table.ID()
		table.UnsignedBigInteger("user_id")
		table.UnsignedBigInteger("connection_id")
		table.String("title").Default("")
		table.LongText("sql")
		table.Boolean("is_saved").Default(false)
		table.TimestampTz("last_run_at").Nullable()
		table.TimestampsTz()
		table.Index("user_id")
		table.Index("connection_id")
	})
}

func (r *M20260219100003CreateSavedQueriesTable) Down() error {
	return facades.Schema().DropIfExists("saved_queries")
}

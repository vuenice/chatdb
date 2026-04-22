package migrations

import (
	"github.com/goravel/framework/contracts/database/schema"

	"goravel/app/facades"
)

type M20260219100002CreateDbConnectionsTable struct{}

func (r *M20260219100002CreateDbConnectionsTable) Signature() string {
	return "20260219100002_create_db_connections_table"
}

func (r *M20260219100002CreateDbConnectionsTable) Up() error {
	if facades.Schema().HasTable("db_connections") {
		return nil
	}
	return facades.Schema().Create("db_connections", func(table schema.Blueprint) {
		table.ID()
		table.UnsignedBigInteger("user_id")
		table.String("name")
		table.String("host")
		table.Integer("port").Default(5432)
		table.String("database")
		table.String("ssl_mode").Default("disable")
		table.String("read_username")
		table.Text("read_password")
		table.String("write_username")
		table.Text("write_password")
		table.Text("allowed_schemas").Nullable()
		table.TimestampsTz()
		table.Index("user_id")
	})
}

func (r *M20260219100002CreateDbConnectionsTable) Down() error {
	return facades.Schema().DropIfExists("db_connections")
}

package migrations

import (
	"github.com/goravel/framework/contracts/database/schema"

	"goravel/app/facades"
)

type M20260219100001CreateUsersTable struct{}

func (r *M20260219100001CreateUsersTable) Signature() string {
	return "20260219100001_create_users_table"
}

func (r *M20260219100001CreateUsersTable) Up() error {
	if facades.Schema().HasTable("users") {
		return nil
	}
	return facades.Schema().Create("users", func(table schema.Blueprint) {
		table.ID()
		table.String("name")
		table.String("email").Unique()
		table.String("password")
		table.String("role").Default("viewer")
		table.TimestampsTz()
	})
}

func (r *M20260219100001CreateUsersTable) Down() error {
	return facades.Schema().DropIfExists("users")
}

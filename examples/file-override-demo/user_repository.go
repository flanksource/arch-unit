//go:build examples
// +build examples

package main

import "database/sql"

func GetUser(id int) error {
	// This should be allowed because of [*_repository.go] override
	var db *sql.DB
	_ = db
	return nil
}

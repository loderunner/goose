package main

import (
	"github.com/loderunner/goose"
)

func init() {
	goose.AddMigration(Up00002, Down00002)
}

func Up00002(qe goose.QueryExecer) error {
	_, err := qe.Exec("UPDATE users SET username='admin' WHERE username='root';")
	if err != nil {
		return err
	}
	return nil
}

func Down00002(qe goose.QueryExecer) error {
	_, err := qe.Exec("UPDATE users SET username='root' WHERE username='admin';")
	if err != nil {
		return err
	}
	return nil
}

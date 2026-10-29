package db

import (
	"os"
	"testing"
)

func localeTestDB(t *testing.T) *DB {
	t.Helper()
	dsn := os.Getenv("ARTEX_PG_DSN")
	if dsn == "" {
		t.Fatal("ARTEX_PG_DSN must point to an isolated test database")
	}
	d, err := Open(dsn)
	if err != nil {
		t.Fatal("isolated database unavailable")
	}
	t.Cleanup(func() { d.Close() })
	return d
}

func TestTaskLocalePersistence(t *testing.T) {
	d := localeTestDB(t)
	task, err := d.CreateTask("locale regression", "locale regression", nil, 0, 0)
	if err != nil { t.Fatal(err) }
	t.Cleanup(func() { d.DeleteTask(task.ID) })
	var locale string
	if err := d.QueryRow(`SELECT locale FROM tasks WHERE id=$1`, task.ID).Scan(&locale); err != nil { t.Fatal(err) }
	if locale != "" { t.Fatalf("legacy locale = %q", locale) }
	if _, err := d.Exec(`UPDATE tasks SET locale='invalid' WHERE id=$1`, task.ID); err == nil { t.Fatal("invalid locale accepted") }
	for _, want := range []string{"ko", "en", "zh"} {
		created, err := d.CreateTaskWithOptions("locale", "locale", TaskCreateOptions{Locale: want})
		if err != nil { t.Fatal(err) }
		t.Cleanup(func() { d.DeleteTask(created.ID) })
		if created.Locale != want { t.Fatalf("created locale = %q", created.Locale) }
		reloaded := localeTestDB(t)
		got, err := reloaded.GetTask(created.ID)
		if err != nil || got == nil || got.Locale != want { t.Fatalf("reloaded task = %+v, err = %v", got, err) }
		list, err := reloaded.ListTasks()
		if err != nil { t.Fatal(err) }
		found := false
		for _, item := range list {
			if item.ID == created.ID { found = item.Locale == want }
		}
		if !found { t.Fatal("task list lost locale") }
	}
	if _, err := d.CreateTaskWithOptions("invalid locale", "locale", TaskCreateOptions{Locale: "ZH"}); err == nil { t.Fatal("invalid create locale accepted") }
}

package main

import (
	"database/sql"
	_ "embed"

	_ "modernc.org/sqlite"
)

//go:embed schema.sql
var schema string

func openDB(path string) (*sql.DB, error) {
	db, err := sql.Open("sqlite", path+"?_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)")
	if err != nil {
		return nil, err
	}
	if _, err := db.Exec(schema); err != nil {
		db.Close()
		return nil, err
	}
	if err := seed(db); err != nil {
		db.Close()
		return nil, err
	}
	return db, nil
}

// seed adds starter tasks on first run only
func seed(db *sql.DB) error {
	var n int
	if err := db.QueryRow("SELECT COUNT(*) FROM tasks").Scan(&n); err != nil {
		return err
	}
	if n > 0 {
		return nil
	}
	_, err := db.Exec(`INSERT INTO tasks (title, icon, stars) VALUES
		('Brush teeth', '🪥', 1),
		('Put shoes away', '👟', 1),
		('Tidy toys', '🧸', 3),
		('Help set up table', '🍽️', 3)`)
	return err
}

func todayTasks(db *sql.DB, day string) ([]Task, error) {
	rows, err := db.Query(`
		SELECT t.id, t.title, t.icon, t.stars, l.id IS NOT NULL
		FROM tasks t
		LEFT JOIN task_log l ON l.task_id = t.id AND l.day = ?
		ORDER BY t.id`, day)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tasks []Task
	for rows.Next() {
		var t Task
		if err := rows.Scan(&t.ID, &t.Title, &t.Icon, &t.Stars, &t.Done); err != nil {
			return nil, err
		}
		tasks = append(tasks, t)
	}
	return tasks, rows.Err()
}

func starBalance(db *sql.DB) (int, error) {
	var total int
	err := db.QueryRow("SELECT COALESCE(SUM(delta), 0) FROM ledger").Scan(&total)
	return total, err
}

// completeTask logs the task for the day and adds its stars, all or nothing
func completeTask(db *sql.DB, taskID int, day string) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var title string
	var stars int
	err = tx.QueryRow("SELECT title, stars FROM tasks WHERE id = ?", taskID).Scan(&title, &stars)
	if err != nil {
		return err
	}

	res, err := tx.Exec(`INSERT INTO task_log (task_id, day, status) VALUES (?, ?, 'approved')
		ON CONFLICT (task_id, day) DO NOTHING`, taskID, day)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return nil // already done today, no double stars
	}

	if _, err := tx.Exec("INSERT INTO ledger (delta, reason) VALUES (?, ?)", stars, "task: "+title); err != nil {
		return err
	}
	return tx.Commit()
}

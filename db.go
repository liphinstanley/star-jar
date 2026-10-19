package main

import (
	"database/sql"
	"embed"
	"errors"
	"fmt"
	"io/fs"

	_ "modernc.org/sqlite"
)

//go:embed migrations/*.sql
var migrationFS embed.FS

func openDB(path string) (*sql.DB, error) {
	db, err := sql.Open("sqlite", path+"?_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)&_txlock=immediate")
	if err != nil {
		return nil, err
	}
	if err := migrate(db); err != nil {
		db.Close()
		return nil, err
	}

	if err := seed(db); err != nil {
		db.Close()
		return nil, err
	}
	return db, nil
}

// migrate applies every migrations/NNN_*.sql file the database hasn't seen yet.
// SQLite's user_version remembers how many have been applied.
func migrate(db *sql.DB) error {
	files, err := fs.Glob(migrationFS, "migrations/*.sql")
	if err != nil {
		return err
	}
	var version int
	if err := db.QueryRow("PRAGMA user_version").Scan(&version); err != nil {
		return err
	}
	for i := version; i < len(files); i++ {
		body, err := migrationFS.ReadFile(files[i])
		if err != nil {
			return err
		}
		if err := applyMigration(db, i+1, string(body)); err != nil {
			return fmt.Errorf("%s: %w", files[i], err)
		}
	}
	return nil
}

// applyMigration runs one migration and bumps user_version, all or nothing
func applyMigration(db *sql.DB, version int, body string) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.Exec(body); err != nil {
		return err
	}
	// PRAGMA doesn't accept ? placeholders; version is our own int, not user input
	if _, err := tx.Exec(fmt.Sprintf("PRAGMA user_version = %d", version)); err != nil {
		return err
	}
	return tx.Commit()
}

var errNotEnoughStars = errors.New("not enough stars yet")

// seed adds starter tasks and rewards on first run only
func seed(db *sql.DB) error {
	if err := seedTable(db, "tasks", `INSERT INTO tasks (title, icon, stars) VALUES
		('Brush teeth', '🪥', 1),
		('Put shoes away', '👟', 1),
		('Tidy toys', '🧸', 3),
		('Help set up table', '🍽️', 3)`); err != nil {
		return err
	}
	return seedTable(db, "rewards", `INSERT INTO rewards (title, icon, cost, size) VALUES
		('Walk with papa', '🚶', 5, 'small'),
		('Extra bedtime story', '📖', 5, 'small'),
		('Play area', '🛝', 10, 'small'),
		('New bike', '🚲', 50, 'big')`)
}

// seedTable runs insert only if table is empty
func seedTable(db *sql.DB, table, insert string) error {
	var n int
	if err := db.QueryRow("SELECT COUNT(*) FROM " + table).Scan(&n); err != nil {
		return err
	}
	if n > 0 {
		return nil
	}
	_, err := db.Exec(insert)
	return err
}

func todayTasks(db *sql.DB, day string) ([]Task, error) {
	rows, err := db.Query(`
		SELECT t.id, t.title, t.icon, t.stars, COALESCE(l.status, '')
		FROM tasks t
		LEFT JOIN task_log l ON l.task_id = t.id AND l.day = ?
		WHERE t.active = 1
		ORDER BY t.id`, day)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tasks []Task
	for rows.Next() {
		var t Task
		if err := rows.Scan(&t.ID, &t.Title, &t.Icon, &t.Stars, &t.Status); err != nil {
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

// requestTask marks a task as done for the day, waiting for a parent to approve.
// A rejected task can be asked for again; a pending or approved one stays as is.
func requestTask(db *sql.DB, taskID int, day string) error {
	var exists bool
	err := db.QueryRow("SELECT EXISTS (SELECT 1 FROM tasks WHERE id = ?)", taskID).Scan(&exists)
	if err != nil {
		return err
	}
	if !exists {
		return sql.ErrNoRows
	}

	_, err = db.Exec(`INSERT INTO task_log (task_id, day, status) VALUES (?, ?, 'pending')
		ON CONFLICT (task_id, day) DO UPDATE SET status = 'pending'
		WHERE status = 'rejected'`, taskID, day)
	return err
}

func pendingRequests(db *sql.DB) ([]Request, error) {
	rows, err := db.Query(`
		SELECT l.id, l.day, t.title, t.icon, t.stars
		FROM task_log l
		JOIN tasks t ON t.id = l.task_id
		WHERE l.status = 'pending'
		ORDER BY l.day, l.id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var reqs []Request
	for rows.Next() {
		var q Request
		if err := rows.Scan(&q.ID, &q.Day, &q.Title, &q.Icon, &q.Stars); err != nil {
			return nil, err
		}
		reqs = append(reqs, q)
	}
	return reqs, rows.Err()
}

// approveRequest moves a pending request to approved and adds its stars, all or nothing
func approveRequest(db *sql.DB, logID int) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var title string
	var stars int
	err = tx.QueryRow(`
		SELECT t.title, t.stars
		FROM task_log l
		JOIN tasks t ON t.id = l.task_id
		WHERE l.id = ? AND l.status = 'pending'`, logID).Scan(&title, &stars)
	if err != nil {
		return err
	}

	res, err := tx.Exec("UPDATE task_log SET status = 'approved' WHERE id = ? AND status = 'pending'", logID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return nil // someone else approved it first, no double stars
	}

	if _, err := tx.Exec("INSERT INTO ledger (delta, reason) VALUES (?, ?)", stars, "task: "+title); err != nil {
		return err
	}
	return tx.Commit()
}

func rejectRequest(db *sql.DB, logID int) error {
	res, err := db.Exec("UPDATE task_log SET status = 'rejected' WHERE id = ? AND status = 'pending'", logID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// listRewards is what the kid sees: visible rewards, cheapest first
func listRewards(db *sql.DB) ([]Reward, error) {
	return queryRewards(db, "SELECT id, title, icon, cost, size, active FROM rewards WHERE active = 1 ORDER BY cost, id")
}

// allRewards is what the parent manages, hidden ones last
func allRewards(db *sql.DB) ([]Reward, error) {
	return queryRewards(db, "SELECT id, title, icon, cost, size, active FROM rewards ORDER BY active DESC, cost, id")
}

func queryRewards(db *sql.DB, query string) ([]Reward, error) {
	rows, err := db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var rewards []Reward
	for rows.Next() {
		var rw Reward
		if err := rows.Scan(&rw.ID, &rw.Title, &rw.Icon, &rw.Cost, &rw.Size, &rw.Active); err != nil {
			return nil, err
		}
		rewards = append(rewards, rw)
	}
	return rewards, rows.Err()
}

// openRedemptions lists rewards bought but not given yet
func openRedemptions(db *sql.DB) ([]Redemption, error) {
	rows, err := db.Query(`
		SELECT d.id, r.title, r.icon, d.cost, d.status
		FROM redemptions d
		JOIN rewards r ON r.id = d.reward_id
		WHERE d.status IN ('pending', 'approved')
		ORDER BY d.id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var reds []Redemption
	for rows.Next() {
		var d Redemption
		if err := rows.Scan(&d.ID, &d.Title, &d.Icon, &d.Cost, &d.Status); err != nil {
			return nil, err
		}
		reds = append(reds, d)
	}
	return reds, rows.Err()
}

// spendReward takes the stars out of the jar and records the purchase.
// Small rewards are approved straight away, big ones wait for a parent.
func spendReward(db *sql.DB, rewardID int) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var title, size string
	var cost int
	err = tx.QueryRow("SELECT title, cost, size FROM rewards WHERE id = ? AND active = 1", rewardID).Scan(&title, &cost, &size)
	if err != nil {
		return err
	}

	var balance int
	if err := tx.QueryRow("SELECT COALESCE(SUM(delta), 0) FROM ledger").Scan(&balance); err != nil {
		return err
	}
	if balance < cost {
		return errNotEnoughStars
	}

	status := "approved"
	if size == "big" {
		status = "pending"
	}
	if _, err := tx.Exec("INSERT INTO ledger (delta, reason) VALUES (?, ?)", -cost, "reward: "+title); err != nil {
		return err
	}
	if _, err := tx.Exec("INSERT INTO redemptions (reward_id, status, cost) VALUES (?, ?, ?)", rewardID, status, cost); err != nil {
		return err
	}
	return tx.Commit()
}

// approveRedemption says yes to a big reward; the stars are already spent
func approveRedemption(db *sql.DB, id int) error {
	return setRedemptionStatus(db, id, "pending", "approved")
}

// givenRedemption marks a reward as actually given
func givenRedemption(db *sql.DB, id int) error {
	return setRedemptionStatus(db, id, "approved", "given")
}

func setRedemptionStatus(db *sql.DB, id int, from, to string) error {
	res, err := db.Exec("UPDATE redemptions SET status = ? WHERE id = ? AND status = ?", to, id, from)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// rejectRedemption says no to a big reward and puts the stars back in the jar
func rejectRedemption(db *sql.DB, id int) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var title string
	var cost int
	err = tx.QueryRow(`
		SELECT r.title, d.cost
		FROM redemptions d
		JOIN rewards r ON r.id = d.reward_id
		WHERE d.id = ? AND d.status = 'pending'`, id).Scan(&title, &cost)
	if err != nil {
		return err
	}

	if _, err := tx.Exec("UPDATE redemptions SET status = 'rejected' WHERE id = ?", id); err != nil {
		return err
	}
	if _, err := tx.Exec("INSERT INTO ledger (delta, reason) VALUES (?, ?)", cost, "refund: "+title); err != nil {
		return err
	}
	return tx.Commit()
}
func allTasks(db *sql.DB) ([]Task, error) {
	rows, err := db.Query("SELECT id, title, icon, stars, active FROM tasks ORDER BY active DESC, id")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tasks []Task
	for rows.Next() {
		var t Task
		if err := rows.Scan(&t.ID, &t.Title, &t.Icon, &t.Stars, &t.Active); err != nil {
			return nil, err
		}
		tasks = append(tasks, t)
	}
	return tasks, rows.Err()
}

func createTask(db *sql.DB, in taskInput) error {
	_, err := db.Exec("INSERT INTO tasks (title, icon, stars) VALUES (?, ?, ?)", in.Title, in.Icon, in.Stars)
	return err
}

func updateTask(db *sql.DB, id int, in taskInput) error {
	res, err := db.Exec("UPDATE tasks SET title = ?, icon = ?, stars = ? WHERE id = ?", in.Title, in.Icon, in.Stars, id)
	return mustAffect(res, err)
}

func createReward(db *sql.DB, in rewardInput) error {
	_, err := db.Exec("INSERT INTO rewards (title, icon, cost, size) VALUES (?, ?, ?, ?)", in.Title, in.Icon, in.Cost, in.Size)
	return err
}

func updateReward(db *sql.DB, id int, in rewardInput) error {
	res, err := db.Exec("UPDATE rewards SET title = ?, icon = ?, cost = ?, size = ? WHERE id = ?", in.Title, in.Icon, in.Cost, in.Size, id)
	return mustAffect(res, err)
}

// setActive hides or shows a task or reward. table is always a constant from our own code.
func setActive(db *sql.DB, table string, id int, active bool) error {
	if table != "tasks" && table != "rewards" {
		return fmt.Errorf("setActive: unknown table %q", table)
	}
	res, err := db.Exec("UPDATE "+table+" SET active = ? WHERE id = ?", active, id)
	return mustAffect(res, err)
}

// mustAffect turns "no row matched" into sql.ErrNoRows, so handlers answer 404
func mustAffect(res sql.Result, err error) error {
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

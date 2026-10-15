CREATE TABLE IF NOT EXISTS tasks (
    id    INTEGER PRIMARY KEY,
    title TEXT    NOT NULL,
    icon  TEXT    NOT NULL,
    stars INTEGER NOT NULL CHECK (stars > 0)
);

-- one row per task per day it was done
CREATE TABLE IF NOT EXISTS task_log (
    id      INTEGER PRIMARY KEY,
    task_id INTEGER NOT NULL REFERENCES tasks(id),
    day     TEXT    NOT NULL,
    status  TEXT    NOT NULL CHECK (status IN ('pending', 'approved', 'rejected')),
    UNIQUE (task_id, day)
);

-- every star in or out; balance = SUM(delta)
CREATE TABLE IF NOT EXISTS ledger (
    id     INTEGER PRIMARY KEY,
    delta  INTEGER NOT NULL,
    reason TEXT    NOT NULL,
    at     TEXT    NOT NULL DEFAULT (datetime('now'))
);

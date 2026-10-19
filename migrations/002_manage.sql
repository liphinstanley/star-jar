-- parents can hide tasks and rewards instead of deleting them (history still points at them)
ALTER TABLE tasks ADD COLUMN active INTEGER NOT NULL DEFAULT 1;
ALTER TABLE rewards ADD COLUMN active INTEGER NOT NULL DEFAULT 1;

-- remember the price paid, so changing a reward's price later doesn't change refunds
ALTER TABLE redemptions ADD COLUMN cost INTEGER NOT NULL DEFAULT 0;
UPDATE redemptions SET cost = (SELECT cost FROM rewards WHERE rewards.id = redemptions.reward_id);

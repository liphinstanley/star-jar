# star-jar

A family web app that teaches a 4-year-old discipline, effort → reward, and the
value of saving. Kid does tasks, earns stars, stars fill a jar, the jar buys
rewards (a walk with papa now, or save up for a bike).

Side project: learn Go + web development, end up with a portfolio piece.

## Core idea

- **Tasks**: small (brush teeth, 1 star) and big (tidy whole room, 5 stars),
  each with a "when" window (morning / before dinner / bedtime), not clock times.
- **Kid taps "done" → parent approves.** Stars only land after a parent confirms.
- **Jar**: stars fill a visible jar. Tap a reward to spend, or keep saving.
- **Rewards**: small (walk, play area, story) vs big (new bike). Big ones show
  progress: "12 / 50 stars".
- **Streaks, gently**: celebrate 3 days in a row, never punish a missed day.

## Two views

| Kid view (4 y/o)                      | Parent view                         |
|---------------------------------------|-------------------------------------|
| Pictures/emoji, no reading needed     | Create/edit tasks and rewards       |
| Huge buttons, one tap per action      | Approve "done" requests             |
| Animated jar filling, sound on earn   | Adjust stars (bonus, corrections)   |
| Only today's tasks                    | History, weekly summary             |
| No settings, no way to cheat          | PIN-protected                       |

## Stack (proposed)

- **Go** backend, `net/http` + `html/template` (stdlib, ties into go-learning)
- **HTMX** for interactivity without a JS framework
- **SQLite** for storage, **Litestream** to back it up to GCS
- **PWA** manifest so it installs on a tablet home screen
- **Cloud Run** (max-instances=1), Docker, GitHub Actions CI
- Auth: parent login (email magic link or Google), kid view via device link + PIN

## Milestones

1. **v0 – local, one kid**: tasks list, mark done, star total. In-memory.
2. **v1 – persistence**: SQLite, parent approves, rewards + redeem.
3. **v2 – kid UI**: pictures, jar animation, sounds, PWA install.
4. **v3 – online**: Docker, Cloud Run, Litestream, custom domain, auth.
5. **v4 – nice-to-haves**: multiple kids, recurring schedules, weekly report,
   "save vs spend" choices, savings goal for big rewards.

## Data model (first cut)

```
family(id, name)
parent(id, family_id, email)
child(id, family_id, name, avatar)
task(id, family_id, title, icon, stars, size[small|big], window[morning|day|evening], recurring)
task_log(id, task_id, child_id, date, status[pending|approved|rejected])
reward(id, family_id, title, icon, cost, size[small|big])
redemption(id, reward_id, child_id, at, fulfilled)
ledger(id, child_id, delta, reason, at)   -- every star in/out, balance = SUM(delta)
```

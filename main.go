package main

import (
	"context"
	"database/sql"
	"embed"
	"errors"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"
	_ "time/tzdata" // time zones built in, the container image has none
)

type Task struct {
	ID     int
	Title  string
	Icon   string
	Stars  int
	Status string // "", "pending", "approved" or "rejected"
	Active bool
}

type Reward struct {
	ID     int
	Title  string
	Icon   string
	Cost   int
	Size   string // "small" or "big"
	Active bool
}

type Redemption struct {
	ID     int
	Title  string
	Icon   string
	Cost   int
	Status string // "pending", "approved", "rejected" or "given"
}

type Request struct {
	ID    int
	Day   string
	Title string
	Icon  string
	Stars int
}
type managePage struct {
	Tasks   []Task
	Rewards []Reward
	Error   string
}

type confirmPage struct {
	Reward Reward
	Total  int // stars now
	After  int // stars left after buying
}

//go:embed templates
var templateFS embed.FS

//go:embed static
var staticFS embed.FS

var funcs = template.FuncMap{
	"stars": func(n int) string { return strings.Repeat("⭐", n) },
}

var tmpl = template.Must(template.New("").Funcs(funcs).ParseFS(templateFS, "templates/*.html"))

type kidPage struct {
	Tasks       []Task
	Rewards     []Reward
	Redemptions []Redemption
	Total       int
}

// JarPercent is how full the jar looks: stars saved toward the biggest reward
func (p kidPage) JarPercent() int {
	goal := 0
	for _, rw := range p.Rewards {
		goal = max(goal, rw.Cost)
	}
	if goal == 0 {
		return 0
	}
	return min(p.Total*100/goal, 100)
}

type parentPage struct {
	Requests    []Request
	Redemptions []Redemption
	Total       int
}

func today() string {
	return time.Now().Format("2006-01-02")
}

// getenv returns the environment variable, or def when it isn't set
func getenv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func main() {
	port := getenv("PORT", "8080")
	dbPath := getenv("DB_PATH", "star-jar.db")
	a, err := newAuth(os.Getenv("FAMILY_PASSWORD"), os.Getenv("PARENT_PIN"), os.Getenv("SESSION_KEY"))
	if err != nil {
		log.Fatal(err)
	}

	db, err := openDB(dbPath)
	if err != nil {
		log.Fatal("open db: ", err)
	}
	defer db.Close()

	// three routers: mux is open to everyone, app needs the family login,
	// parent needs the PIN as well
	mux := http.NewServeMux()
	app := http.NewServeMux()
	parent := http.NewServeMux()
	mux.Handle("/", a.requireFamily(app))
	app.Handle("/parent", a.requireParent(parent))
	app.Handle("/parent/", a.requireParent(parent))

	mux.HandleFunc("GET /login", a.loginPage)
	mux.HandleFunc("POST /login", a.login)
	mux.HandleFunc("POST /logout", a.logout)
	app.HandleFunc("GET /parent/pin", a.pinPage)
	app.HandleFunc("POST /parent/pin", a.unlock)
	parent.HandleFunc("POST /parent/lock", a.lock)

	mux.Handle("GET /static/", http.FileServerFS(staticFS))

	// the service worker must be served from / to control the whole site
	mux.HandleFunc("GET /sw.js", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-cache")
		http.ServeFileFS(w, r, staticFS, "static/sw.js")
	})

	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		if err := db.PingContext(r.Context()); err != nil {
			http.Error(w, "database unavailable", http.StatusServiceUnavailable)
			return
		}
		fmt.Fprintln(w, "ok")
	})

	app.HandleFunc("GET /{$}", func(w http.ResponseWriter, r *http.Request) {
		var page kidPage
		var err error
		if page.Tasks, err = todayTasks(db, today()); err != nil {
			serverError(w, err)
			return
		}
		if page.Rewards, err = listRewards(db); err != nil {
			serverError(w, err)
			return
		}
		if page.Redemptions, err = openRedemptions(db); err != nil {
			serverError(w, err)
			return
		}
		if page.Total, err = starBalance(db); err != nil {
			serverError(w, err)
			return
		}
		render(w, "index.html", page)

	})

	app.HandleFunc("POST /tasks/{id}/done", func(w http.ResponseWriter, r *http.Request) {
		id, err := pathID(r)
		if err != nil {
			http.Error(w, "bad id", http.StatusBadRequest)
			return
		}
		if !handled(w, r, requestTask(db, id, today())) {
			return
		}
		http.Redirect(w, r, "/", http.StatusSeeOther)
	})

	parent.HandleFunc("GET /parent", func(w http.ResponseWriter, r *http.Request) {
		var page parentPage
		var err error
		if page.Requests, err = pendingRequests(db); err != nil {
			serverError(w, err)
			return
		}
		if page.Redemptions, err = openRedemptions(db); err != nil {
			serverError(w, err)
			return
		}
		if page.Total, err = starBalance(db); err != nil {
			serverError(w, err)
			return
		}
		render(w, "parent.html", page)

	})

	parent.HandleFunc("POST /parent/requests/{id}/approve", func(w http.ResponseWriter, r *http.Request) {
		id, err := pathID(r)
		if err != nil {
			http.Error(w, "bad id", http.StatusBadRequest)
			return
		}
		if !handled(w, r, approveRequest(db, id)) {
			return
		}
		http.Redirect(w, r, "/parent", http.StatusSeeOther)
	})

	parent.HandleFunc("POST /parent/requests/{id}/reject", func(w http.ResponseWriter, r *http.Request) {
		id, err := pathID(r)
		if err != nil {
			http.Error(w, "bad id", http.StatusBadRequest)
			return
		}
		if !handled(w, r, rejectRequest(db, id)) {
			return
		}
		http.Redirect(w, r, "/parent", http.StatusSeeOther)
	})

	// "are you sure?" before stars leave the jar
	app.HandleFunc("GET /rewards/{id}", func(w http.ResponseWriter, r *http.Request) {
		id, err := pathID(r)
		if err != nil {
			http.Error(w, "bad id", http.StatusBadRequest)
			return
		}
		rw, err := getReward(db, id)
		if !handled(w, r, err) {
			return
		}
		total, err := starBalance(db)
		if err != nil {
			serverError(w, err)
			return
		}
		if total < rw.Cost {
			http.Redirect(w, r, "/", http.StatusSeeOther)
			return
		}
		render(w, "confirm.html", confirmPage{Reward: rw, Total: total, After: total - rw.Cost})
	})

	app.HandleFunc("POST /rewards/{id}/spend", func(w http.ResponseWriter, r *http.Request) {
		id, err := pathID(r)
		if err != nil {
			http.Error(w, "bad id", http.StatusBadRequest)
			return
		}
		if !handled(w, r, spendReward(db, id)) {
			return
		}
		http.Redirect(w, r, "/", http.StatusSeeOther)
	})

	redemptionActions := map[string]func(*sql.DB, int) error{
		"approve": approveRedemption,
		"reject":  rejectRedemption,
		"given":   givenRedemption,
	}
	parent.HandleFunc("POST /parent/redemptions/{id}/{action}", func(w http.ResponseWriter, r *http.Request) {
		action, ok := redemptionActions[r.PathValue("action")]
		if !ok {
			http.NotFound(w, r)
			return
		}
		id, err := pathID(r)
		if err != nil {
			http.Error(w, "bad id", http.StatusBadRequest)
			return
		}
		if !handled(w, r, action(db, id)) {
			return
		}
		http.Redirect(w, r, "/parent", http.StatusSeeOther)
	})

	parent.HandleFunc("GET /parent/manage", func(w http.ResponseWriter, r *http.Request) {
		page := managePage{Error: r.URL.Query().Get("error")}
		var err error
		if page.Tasks, err = allTasks(db); err != nil {
			serverError(w, err)
			return
		}
		if page.Rewards, err = allRewards(db); err != nil {
			serverError(w, err)
			return
		}
		render(w, "manage.html", page)
	})

	parent.HandleFunc("POST /parent/tasks", func(w http.ResponseWriter, r *http.Request) {
		in, err := readTask(r)
		if err == nil {
			err = createTask(db, in)
		}
		backToManage(w, r, err)
	})

	parent.HandleFunc("POST /parent/tasks/{id}", func(w http.ResponseWriter, r *http.Request) {
		id, err := pathID(r)
		if err != nil {
			http.Error(w, "bad id", http.StatusBadRequest)
			return
		}
		in, err := readTask(r)
		if err == nil {
			err = updateTask(db, id, in)
		}
		backToManage(w, r, err)
	})

	parent.HandleFunc("POST /parent/rewards", func(w http.ResponseWriter, r *http.Request) {
		in, err := readReward(r)
		if err == nil {
			err = createReward(db, in)
		}
		backToManage(w, r, err)
	})

	parent.HandleFunc("POST /parent/rewards/{id}", func(w http.ResponseWriter, r *http.Request) {
		id, err := pathID(r)
		if err != nil {
			http.Error(w, "bad id", http.StatusBadRequest)
			return
		}
		in, err := readReward(r)
		if err == nil {
			err = updateReward(db, id, in)
		}
		backToManage(w, r, err)
	})

	parent.HandleFunc("POST /parent/tasks/{id}/active", setActiveHandler(db, "tasks"))
	parent.HandleFunc("POST /parent/rewards/{id}/active", setActiveHandler(db, "rewards"))

	srv := &http.Server{
		Addr:              ":" + port,
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
	}

	// stop on Ctrl+C locally, or SIGTERM from Docker / Cloud Run
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	go func() {
		log.Printf("listening on :%s, db %s, today is %s (%s)", port, dbPath, today(), time.Now().Format("MST"))
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatal(err)
		}
	}()

	<-ctx.Done()
	log.Println("shutting down")

	// let requests in flight finish, but don't wait forever
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Println("shutdown:", err)
	}
	// deferred db.Close() runs now, after the last request
}

func pathID(r *http.Request) (int, error) {
	return strconv.Atoi(r.PathValue("id"))
}

// setActiveHandler builds the hide/show handler for tasks or rewards
func setActiveHandler(db *sql.DB, table string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := pathID(r)
		if err != nil {
			http.Error(w, "bad id", http.StatusBadRequest)
			return
		}
		active := r.FormValue("active") == "1"
		backToManage(w, r, setActive(db, table, id, active))
	}
}

// backToManage redirects to the manage page, carrying a typing mistake along as ?error=
func backToManage(w http.ResponseWriter, r *http.Request, err error) {
	var bad inputError
	if errors.As(err, &bad) {
		http.Redirect(w, r, "/parent/manage?error="+url.QueryEscape(bad.Error()), http.StatusSeeOther)
		return
	}
	if !handled(w, r, err) {
		return
	}
	http.Redirect(w, r, "/parent/manage", http.StatusSeeOther)
}

// handled writes an error response if err is not nil and reports whether it was nil
func handled(w http.ResponseWriter, r *http.Request, err error) bool {
	switch {
	case err == nil:
		return true
	case errors.Is(err, sql.ErrNoRows):
		http.NotFound(w, r)
	case errors.Is(err, errNotEnoughStars):
		http.Error(w, err.Error(), http.StatusConflict)
	default:
		serverError(w, err)
	}
	return false
}

func render(w http.ResponseWriter, name string, data any) {
	if err := tmpl.ExecuteTemplate(w, name, data); err != nil {
		log.Println("render:", err)
	}
}

func serverError(w http.ResponseWriter, err error) {
	log.Println("error:", err)
	http.Error(w, "something went wrong", http.StatusInternalServerError)
}

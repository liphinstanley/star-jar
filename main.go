package main

import (
	"database/sql"
	"embed"
	"errors"
	"html/template"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
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

func main() {
	db, err := openDB("star-jar.db")
	if err != nil {
		log.Fatal("open db: ", err)
	}
	defer db.Close()

	mux := http.NewServeMux()
	mux.Handle("GET /static/", http.FileServerFS(staticFS))

	// the service worker must be served from / to control the whole site
	mux.HandleFunc("GET /sw.js", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-cache")
		http.ServeFileFS(w, r, staticFS, "static/sw.js")
	})

	mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, r *http.Request) {
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

	mux.HandleFunc("POST /tasks/{id}/done", func(w http.ResponseWriter, r *http.Request) {
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

	mux.HandleFunc("GET /parent", func(w http.ResponseWriter, r *http.Request) {
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

	mux.HandleFunc("POST /parent/requests/{id}/approve", func(w http.ResponseWriter, r *http.Request) {
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

	mux.HandleFunc("POST /parent/requests/{id}/reject", func(w http.ResponseWriter, r *http.Request) {
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

	mux.HandleFunc("POST /rewards/{id}/spend", func(w http.ResponseWriter, r *http.Request) {
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
	mux.HandleFunc("POST /parent/redemptions/{id}/{action}", func(w http.ResponseWriter, r *http.Request) {
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

	mux.HandleFunc("GET /parent/manage", func(w http.ResponseWriter, r *http.Request) {
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

	mux.HandleFunc("POST /parent/tasks", func(w http.ResponseWriter, r *http.Request) {
		in, err := readTask(r)
		if err == nil {
			err = createTask(db, in)
		}
		backToManage(w, r, err)
	})

	mux.HandleFunc("POST /parent/tasks/{id}", func(w http.ResponseWriter, r *http.Request) {
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

	mux.HandleFunc("POST /parent/rewards", func(w http.ResponseWriter, r *http.Request) {
		in, err := readReward(r)
		if err == nil {
			err = createReward(db, in)
		}
		backToManage(w, r, err)
	})

	mux.HandleFunc("POST /parent/rewards/{id}", func(w http.ResponseWriter, r *http.Request) {
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

	mux.HandleFunc("POST /parent/tasks/{id}/active", setActiveHandler(db, "tasks"))
	mux.HandleFunc("POST /parent/rewards/{id}/active", setActiveHandler(db, "rewards"))

	log.Println("listening on http://localhost:8080")
	log.Fatal(http.ListenAndServe(":8080", mux))
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

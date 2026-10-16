package main

import (
	"database/sql"
	"embed"
	"errors"
	"html/template"
	"log"
	"net/http"
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
}

type Reward struct {
	ID    int
	Title string
	Icon  string
	Cost  int
	Size  string // "small" or "big"
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

	log.Println("listening on http://localhost:8080")
	log.Fatal(http.ListenAndServe(":8080", mux))
}

func pathID(r *http.Request) (int, error) {
	return strconv.Atoi(r.PathValue("id"))
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

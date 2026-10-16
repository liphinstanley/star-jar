package main

import (
	"database/sql"
	"embed"
	"errors"
	"html/template"
	"log"
	"net/http"
	"strconv"
	"time"
)

type Task struct {
	ID     int
	Title  string
	Icon   string
	Stars  int
	Status string // "", "pending", "approved" or "rejected"
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

var tmpl = template.Must(template.ParseFS(templateFS, "templates/*.html"))

type kidPage struct {
	Tasks []Task
	Total int
}

type parentPage struct {
	Requests []Request
	Total    int
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

	mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, r *http.Request) {
		tasks, err := todayTasks(db, today())
		if err != nil {
			serverError(w, err)
			return
		}
		total, err := starBalance(db)
		if err != nil {
			serverError(w, err)
			return
		}
		render(w, "index.html", kidPage{Tasks: tasks, Total: total})
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
		reqs, err := pendingRequests(db)
		if err != nil {
			serverError(w, err)
			return
		}
		total, err := starBalance(db)
		if err != nil {
			serverError(w, err)
			return
		}
		render(w, "parent.html", parentPage{Requests: reqs, Total: total})
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

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
	ID    int
	Title string
	Icon  string
	Stars int
	Done  bool
}

//go:embed templates
var templateFS embed.FS

var tmpl = template.Must(template.ParseFS(templateFS, "templates/index.html"))

type pageData struct {
	Tasks []Task
	Total int
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
		if err := tmpl.Execute(w, pageData{Tasks: tasks, Total: total}); err != nil {
			log.Println("render:", err)
		}
	})

	mux.HandleFunc("POST /tasks/{id}/done", func(w http.ResponseWriter, r *http.Request) {
		id, err := strconv.Atoi(r.PathValue("id"))
		if err != nil {
			http.Error(w, "bad task id", http.StatusBadRequest)
			return
		}

		err = completeTask(db, id, today())
		if errors.Is(err, sql.ErrNoRows) {
			http.NotFound(w, r)
			return
		}
		if err != nil {
			serverError(w, err)
			return
		}
		http.Redirect(w, r, "/", http.StatusSeeOther)
	})

	log.Println("listening on http://localhost:8080")
	log.Fatal(http.ListenAndServe(":8080", mux))
}

func serverError(w http.ResponseWriter, err error) {
	log.Println("error:", err)
	http.Error(w, "something went wrong", http.StatusInternalServerError)
}

package main

import (
	"embed"
	"html/template"
	"log"
	"net/http"
	"strconv"
	"sync"
)

type Task struct {
	ID    int
	Title string
	Icon  string
	Stars int
	Done  bool
}

// in-memory for now; SQLite comes in v1
var (
	mu    sync.Mutex
	tasks = []Task{
		{ID: 1, Title: "Brush teeth", Icon: "🪥", Stars: 1},
		{ID: 2, Title: "Put shoes away", Icon: "👟", Stars: 1},
		{ID: 3, Title: "Tidy toys", Icon: "🧸", Stars: 3},
		{ID: 4, Title: "Help set up table", Icon: "🍽️", Stars: 3},
	}
)

//go:embed templates
var templateFS embed.FS

var tmpl = template.Must(template.ParseFS(templateFS, "templates/index.html"))

type pageData struct {
	Tasks []Task
	Total int
}

func totalStars() int {
	total := 0
	for _, t := range tasks {
		if t.Done {
			total += t.Stars
		}
	}
	return total
}

func main() {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		data := pageData{Tasks: tasks, Total: totalStars()}
		err := tmpl.Execute(w, data)
		mu.Unlock()
		if err != nil {
			log.Println("render:", err)
		}
	})

	mux.HandleFunc("POST /tasks/{id}/done", func(w http.ResponseWriter, r *http.Request) {
		id, err := strconv.Atoi(r.PathValue("id"))
		if err != nil {
			http.Error(w, "bad task id", http.StatusBadRequest)
			return
		}

		mu.Lock()
		found := false
		for i := range tasks {
			if tasks[i].ID == id {
				tasks[i].Done = true
				found = true
				break
			}
		}
		mu.Unlock()

		if !found {
			http.NotFound(w, r)
			return
		}
		http.Redirect(w, r, "/", http.StatusSeeOther)
	})

	log.Println("listening on http://localhost:8080")
	log.Fatal(http.ListenAndServe(":8080", mux))
}

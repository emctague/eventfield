package main

import (
	"encoding/json"
	"github.com/gorilla/mux"
	"github.com/jinzhu/gorm"
	_ "github.com/jinzhu/gorm/dialects/sqlite"
	"golang.org/x/crypto/bcrypt"
	"gopkg.in/russross/blackfriday.v2"
	"html/template"
	"log"
	"net/http"
	"strconv"
	txtemplate "text/template"
)

type Post struct {
	gorm.Model
	Blog Blog
	BlogID int

	Body template.HTML
}

type Blog struct {
	gorm.Model
	PwHash []byte
}

func main() {
	db, err := gorm.Open("sqlite3", "test.db")
	if err != nil { log.Fatal(err) }
	defer db.Close()
	db.AutoMigrate(&Post{}, &Blog{})

	views := template.Must(template.ParseGlob("views/*"))

	script, err := txtemplate.New("index.gojs").Funcs(txtemplate.FuncMap{
		"json": func(obj interface{}) string {
			data, err := json.Marshal(obj)
			if err != nil { return "\"ERROR\"" }

			return string(data)
		},
	}).ParseFiles("index.gojs")
	if err != nil { log.Fatal(err) }

	r := mux.NewRouter()

	r.HandleFunc("/", func (w http.ResponseWriter, r *http.Request) {
		views.ExecuteTemplate(w, "index.gohtml", nil)
	})

	r.HandleFunc("/blogs", func (w http.ResponseWriter, r *http.Request) {
		hash, err := bcrypt.GenerateFromPassword([]byte(r.FormValue("password")), 15)
		if err != nil { http.Error(w, "Internal server error", 500); return }

		blog := &Blog { PwHash: hash }
		db.Create(blog)

		http.Redirect(w, r, "/blogs/" + strconv.Itoa(int(blog.ID)), 302)
	}).Methods("POST")

	r.HandleFunc("/blogs", func (w http.ResponseWriter, r *http.Request) {
		views.ExecuteTemplate(w, "blogs.gohtml", nil)
	})

	r.HandleFunc("/blogs/{id}/js/{selector}", func (w http.ResponseWriter, r *http.Request) {
		id, err := strconv.Atoi(mux.Vars(r)["id"])
		if err != nil { log.Fatal(err) }

		selector := mux.Vars(r)["selector"]

		var posts []Post
		db.Order("created_at desc").Find(&posts, &Post{ BlogID: id })

		script.Execute(w, struct { Selector string; Posts []Post } { selector, posts })
	})

	r.HandleFunc("/blogs/{id}", func (w http.ResponseWriter, r *http.Request) {
		// Hardcoded for now I guess
		views.ExecuteTemplate(w, "preview.gohtml", "https://eventfield.herokuapp.com/" + r.URL.Path)
	})

	r.HandleFunc("/blogs/{id}/add", func (w http.ResponseWriter, r *http.Request) {
		id, err := strconv.Atoi(mux.Vars(r)["id"])
		if err != nil { log.Fatal(err) }

		pw := r.FormValue("password")
		body := r.FormValue("body")

		var blog Blog
		db.First(&blog, id)

		err = bcrypt.CompareHashAndPassword(blog.PwHash, []byte(pw))
		if err != nil { http.Error(w, "Access denied", 403); return }

		db.Create(&Post {
			Body: template.HTML(blackfriday.Run([]byte(body))),
			BlogID: id,
		})

		http.Redirect(w, r, "/blogs/" + strconv.Itoa(id), 302)
	}).Methods("POST")

	r.HandleFunc("/blogs/{id}/add", func (w http.ResponseWriter, r *http.Request) {
		views.ExecuteTemplate(w, "add.gohtml", mux.Vars(r)["id"])
	})

	r.PathPrefix("/static").Handler(http.StripPrefix("/static/", http.FileServer(http.Dir("static"))))

	log.Fatal(http.ListenAndServe(":8080", r))
}

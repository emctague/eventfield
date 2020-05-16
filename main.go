package main

import (
	"encoding/json"
	"github.com/gorilla/mux"
	"github.com/jinzhu/gorm"
	_ "github.com/jinzhu/gorm/dialects/postgres"
	"github.com/russross/blackfriday/v2"
	"golang.org/x/crypto/bcrypt"
	"html/template"
	"log"
	"net/http"
	"os"
	"strconv"
	txtemplate "text/template"
)

type Post struct {
	gorm.Model
	Blog   Blog
	BlogID uint

	Body template.HTML
}

type Blog struct {
	gorm.Model
	PwHash []byte
}

func main() {
	db, err := gorm.Open("postgres", os.Getenv("DATABASE_URL"))
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()
	db.AutoMigrate(&Post{}, &Blog{})

	views := template.Must(template.ParseGlob("views/*"))

	script, err := txtemplate.New("index.gojs").
		Funcs(txtemplate.FuncMap{"json": ToJSONString}).
		ParseFiles("index.gojs")

	if err != nil {
		log.Fatal(err)
	}

	r := mux.NewRouter()

	r.HandleFunc("/blogs", func(w http.ResponseWriter, r *http.Request) {
		hash, err := bcrypt.GenerateFromPassword([]byte(r.FormValue("password")), 15)
		if err != nil {
			http.Error(w, "Internal server error", 500)
			return
		}

		blog := &Blog{PwHash: hash}
		db.Create(blog)

		http.Redirect(w, r, "/blogs/"+strconv.Itoa(int(blog.ID)), 302)
	}).Methods("POST")

	r.HandleFunc("/blogs/{id}/js/{selector}", RouteWithID(db, func(blog Blog, w http.ResponseWriter, r *http.Request) {
		selector := mux.Vars(r)["selector"]

		var posts []Post
		db.Order("created_at desc").Find(&posts, &Post{BlogID: blog.ID})

		err = script.Execute(w, struct {
			Selector string
			Posts    []Post
		}{selector, posts})

		if err != nil {
			http.Error(w, "Internal server error", 500)
		}
	}))

	r.HandleFunc("/blogs/{id}/add", RouteWithID(db, func(blog Blog, w http.ResponseWriter, r *http.Request) {
		pw := r.FormValue("password")
		body := r.FormValue("body")

		err = bcrypt.CompareHashAndPassword(blog.PwHash, []byte(pw))
		if err != nil {
			http.Error(w, "Access denied", 403)
			return
		}

		db.Create(&Post{
			Body:   template.HTML(blackfriday.Run([]byte(body))),
			BlogID: blog.ID,
		})

		http.Redirect(w, r, "/blogs/"+strconv.Itoa(int(blog.ID)), 302)
	})).Methods("POST")

	r.HandleFunc("/", PageFor(views, "index.gohtml"))
	r.HandleFunc("/blogs", PageFor(views, "blogs.gohtml"))
	r.HandleFunc("/blogs/{id}/add", PageFor(views, "add.gohtml"))
	r.HandleFunc("/blogs/{id}", PageFor(views, "preview.gohtml"))
	r.PathPrefix("/static").Handler(http.StripPrefix("/static/", http.FileServer(http.Dir("static"))))

	log.Fatal(http.ListenAndServe(":"+os.Getenv("PORT"), r))
}

func RouteWithID(db *gorm.DB, handler func(Blog, http.ResponseWriter, *http.Request)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var blog Blog
		id, err := strconv.Atoi(mux.Vars(r)["id"])

		if err != nil || db.First(&blog, id).RecordNotFound() {
			http.Error(w, "Blog not found", 404)
			return
		}

		handler(blog, w, r)
	}
}

func PageFor(views *template.Template, page string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		err := views.ExecuteTemplate(w, page,
			struct {
				Vars map[string]string
				Path string
			}{
				mux.Vars(r),
				"https://eventfield.herokuapp.com/" + r.URL.Path,
			})

		if err != nil {
			http.Error(w, "Internal server error", 500)
		}
	}
}

func ToJSONString(obj interface{}) string {
	data, err := json.Marshal(obj)
	if err != nil {
		return "\"ERROR\""
	}

	return string(data)
}

package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"text/template"
	"time"

	"github.com/laurence6/gtd.go/core"
)

const (
	dateLayout = "2006-01-02"
	timeLayout = "15:04"
)

var errInvalid = errors.New("Invalid!")

var location, _ = time.LoadLocation("Local")

var t *template.Template

func init() {
	var err error
	t, err = template.ParseFiles(
		"templates/default.html",
		"templates/edit.html",
		"templates/form.html",
		"templates/index.html",
	)
	if err != nil {
		log.Fatalln(err.Error())
	}
}

func web() {
	http.HandleFunc("/", landing)
	http.HandleFunc("/index", jsonHandlerWrapper(index))
	http.HandleFunc("/add", jsonHandlerWrapper(addTask))
	http.HandleFunc("/addSub", jsonHandlerWrapper(addSubTask))
	http.HandleFunc("/edit", jsonHandlerWrapper(editTask))
	http.HandleFunc("/done", jsonHandlerWrapper(doneTask))
	http.HandleFunc("/delete", jsonHandlerWrapper(deleteTask))
	http.HandleFunc("/update", jsonHandlerWrapper(updateTask))

	http.Handle("/css/", http.StripPrefix("/css/", http.FileServer(http.Dir("static/css"))))
	http.Handle("/js/", http.StripPrefix("/js/", http.FileServer(http.Dir("static/js"))))
	http.Handle("/fonts/", http.StripPrefix("/fonts/", http.FileServer(http.Dir("static/fonts"))))

	go func() {
		log.Fatalln(http.ListenAndServe(":8000", nil))
	}()
}

func landing(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		httpError(w, 404, "")
		return
	}
	t.ExecuteTemplate(w, "default", "")
}

type responseJSON struct {
	StatusCode int
	Content    string
}

func newResponseJSON() *responseJSON {
	return &responseJSON{200, ""}
}

func jsonHandlerWrapper(f func(r *http.Request) *responseJSON) http.HandlerFunc {
	handlerFunc := func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			httpError(w, 405, "POST")
			return
		}
		response := f(r)
		jsonObj, err := json.Marshal(response)
		if err != nil {
			httpError(w, 500, err.Error())
			return
		}
		w.Write(jsonObj)
	}
	return handlerFunc
}

func index(r *http.Request) *responseJSON {
	response := newResponseJSON()
	b := &bytes.Buffer{}
	err := t.ExecuteTemplate(b, "index", defaultIndex)
	if err != nil {
		log.Println(err.Error())
		jsonError(response, err.Error())
		return response
	}
	response.Content = b.String()
	return response
}

func addTask(r *http.Request) *responseJSON {
	response := newResponseJSON()
	b := &bytes.Buffer{}
	err := t.ExecuteTemplate(b, "form", "")
	if err != nil {
		log.Println(err.Error())
		jsonError(response, err.Error())
		return response
	}
	response.Content = b.String()
	return response
}

func addSubTask(r *http.Request) *responseJSON {
	response := newResponseJSON()
	r.ParseForm()
	id, err := stoI64(r.Form.Get("ID"))
	if err != nil {
		goto LogJsonErrRet
	}
	if id != 0 {
		tpRW.RLock()
		task := tp.Get(id)
		tpRW.RUnlock()
		if task != nil {
			tpRW.Lock()
			subTask, err := tp.NewSubTask(task)
			tpRW.Unlock()
			tp.Changed()
			if err != nil {
				goto LogJsonErrRet
			}
			jsonRedirect(response, "/edit?ID="+strconv.FormatInt(subTask.ID, 10))
			return response
		}
	}
	jsonRedirect(response, "/add")
	return response

LogJsonErrRet:
	log.Println(err.Error())
	jsonError(response, err.Error())
	return response
}

func editTask(r *http.Request) *responseJSON {
	response := newResponseJSON()
	b := &bytes.Buffer{}
	r.ParseForm()
	id, err := stoI64(r.Form.Get("ID"))
	if err != nil {
		goto LogJsonErrRet
	}
	if id != 0 {
		tpRW.RLock()
		task := tp.Get(id)
		tpRW.RUnlock()
		if task != nil {
			_ = t.ExecuteTemplate(b, "form", "")
			err = t.ExecuteTemplate(b, "parentsubtask", task)
			if err != nil {
				goto LogJsonErrRet
			}
			err = t.ExecuteTemplate(b, "edit", task)
			if err != nil {
				goto LogJsonErrRet
			}
			response.Content = b.String()
			return response
		}
	}
	jsonRedirect(response, "/add")
	return response

LogJsonErrRet:
	log.Println(err.Error())
	jsonError(response, err.Error())
	return response
}

func doneTask(r *http.Request) *responseJSON {
	response := newResponseJSON()
	r.ParseForm()
	id, err := stoI64(r.Form.Get("ID"))
	if err != nil {
		goto LogJsonErrRet
	}
	if id != 0 {
		tpRW.RLock()
		task := tp.Get(id)
		tpRW.RUnlock()
		if task != nil {
			tpRW.Lock()
			err := tp.Done(task)
			tpRW.Unlock()
			if err != nil {
				goto LogJsonErrRet
			}
			tp.Changed()
			jsonRedirect(response, "/index")
			return response
		}
	}
	jsonError(response, errInvalid.Error())
	return response

LogJsonErrRet:
	log.Println(err.Error())
	jsonError(response, err.Error())
	return response
}

func deleteTask(r *http.Request) *responseJSON {
	response := newResponseJSON()
	r.ParseForm()
	id, err := stoI64(r.Form.Get("ID"))
	if err != nil {
		goto LogJsonErrRet
	}
	if id != 0 {
		tpRW.RLock()
		task := tp.Get(id)
		tpRW.RUnlock()
		if task != nil {
			tpRW.Lock()
			err := tp.Delete(task)
			tpRW.Unlock()
			if err != nil {
				goto LogJsonErrRet
			}
			tp.Changed()
			jsonRedirect(response, "/index")
			return response
		}
	}
	jsonError(response, errInvalid.Error())
	return response

LogJsonErrRet:
	log.Println(err.Error())
	jsonError(response, err.Error())
	return response
}

func updateTask(r *http.Request) *responseJSON {
	response := newResponseJSON()
	var task *gtd.Task
	r.ParseForm()
	id, err := stoI64(r.PostForm.Get("ID"))
	if err != nil {
		goto LogJsonErrRet
	}
	if id == 0 {
		tpRW.Lock()
		task, err = tp.NewTask()
		tpRW.Unlock()
		if err != nil {
			tp.Delete(task)
			goto LogJsonErrRet
		}
		tpRW.Lock()
		err = updateTaskFromForm(task, r.PostForm)
		tpRW.Unlock()
		if err != nil {
			tp.Delete(task)
			goto LogJsonErrRet
		}
		tp.Changed()
		jsonRedirect(response, "/index")
		return response
	}
	tpRW.RLock()
	task = tp.Get(id)
	tpRW.RUnlock()
	if task != nil {
		tpRW.Lock()
		err := updateTaskFromForm(task, r.PostForm)
		tpRW.Unlock()
		if err != nil {
			goto LogJsonErrRet
		}
		tp.Changed()
		jsonRedirect(response, "/index")
		return response
	}
	jsonError(response, errInvalid.Error())
	return response

LogJsonErrRet:
	log.Println(err.Error())
	jsonError(response, err.Error())
	return response
}

var httpErrorMessage = map[int]string{
	404: "404 NotFound",
	405: "405 MethodNotAllowed",
	500: "500 InternalServerError",
}

func httpError(w http.ResponseWriter, code int, message string) {
	w.WriteHeader(code)
	codeMessage := httpErrorMessage[code]
	fmt.Fprintf(w, "<html><body><center><h1>%s</h1></center><p>%s</p></body></html>", codeMessage, message)
}

func jsonRedirect(r *responseJSON, content string) {
	r.StatusCode = 302
	r.Content = content
}

func jsonError(r *responseJSON, content string) {
	r.StatusCode = 500
	r.Content = "Oops: " + content
}

func stoI64(str string) (int64, error) {
	if str == "" {
		return 0, nil
	}
	i64, err := strconv.ParseInt(str, 10, 64)
	if err != nil {
		return 0, err
	}
	return i64, nil
}

// updateTaskFromForm updates Subject, Due, Priority, Next, Notification, Note fields
func updateTaskFromForm(task *gtd.Task, form url.Values) error {
	var err error
	task.Subject = form.Get("Subject")

	noDue := form.Get("NoDue")
	if noDue == "on" {
		task.Due.Set(0)
	} else {
		err := task.Due.ParseDateTimeInLocation(form.Get("DueDate"), form.Get("DueTime"), location)
		if err != nil {
			return err
		}
	}

	task.Priority, err = strconv.Atoi(form.Get("Priority"))
	if err != nil {
		return err
	}

	next := form.Get("Next")
	if next == "on" {
		err := task.Next.ParseDateTimeInLocation(form.Get("NextDate"), form.Get("NextTime"), location)
		if err != nil {
			return err
		}
	} else {
		task.Next.Set(0)
	}

	noNotification := form.Get("NoNotification")
	if noNotification == "on" {
		task.Notification.Set(0)
	} else {
		err := task.Notification.ParseDateTimeInLocation(form.Get("NotificationDate"), form.Get("NotificationTime"), location)
		if err != nil {
			return err
		}
	}

	task.Note = form.Get("Note")

	return nil
}

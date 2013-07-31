package main

import (
	"bytes"
	"encoding/json"
	"github.com/xconstruct/go-pushbullet"
	htmpl "html/template"
	"log"
	"net/http"
	"os"
	"strings"
	ttmpl "text/template"
)

type Payload struct {
	Repository struct {
		Name  string
		Owner struct {
			Name string
		}
	}
	Pusher struct {
		Name string
	}
	Commits []struct {
		Author struct {
                        Username string
			Name string
		}
		Id      string
		Message string
	}

	TmplParams struct {
		CommitCount int //
	}
}

func main() {

	http.HandleFunc("/pb/", func(w http.ResponseWriter, r *http.Request) {
		path := strings.Split(r.URL.Path, "/")

		if len(path) != 3 {
			http.NotFound(w, r)
			return
		}

		apikey := path[2]
		if len(apikey) != 32 {
			http.Error(w, "", http.StatusBadRequest)
			return
		}

		r.ParseForm()
		payloadJSON := r.PostFormValue("payload")

		var payload Payload
		json.Unmarshal([]byte(payloadJSON), &payload)

		payload.TmplParams.CommitCount = len(payload.Commits)

		var o bytes.Buffer
		pushTemplate.Execute(&o, payload)

		pb := pushbullet.New(apikey)
		devices, err := pb.Devices()
		if err != nil {
			http.Error(w, err.Error(), http.StatusServiceUnavailable)
			return
		}

		pb.PushNote(devices[0].Id, "GitHub", o.String())
	})

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}

		rootTemplate.Execute(w, nil)
	})

	port := ":8080"

	if p := os.Getenv("PORT"); p != "" {
		port = ":" + p
	}

	log.Println("Listening on port", port)

	log.Fatal(http.ListenAndServe(port, nil))
}

var rootTemplate = htmpl.Must(htmpl.New("root").Parse(rootTemplateHTML))

const rootTemplateHTML = `
<html>
  <head>
  <title>ghpbhoook</title>
  <style type="text/css">

    @import url(//fonts.googleapis.com/css?family=Droid+Serif);
    @import url(//fonts.googleapis.com/css?family=Droid+Sans+Mono);

    body {
       background : lightgrey ;
       margin-top : 100px ;
       font-family : 'Droid Serif' ;
    }

    div#content
    {
       margin : auto ;
       width : 75%;
    }

</style>

  <body>
    <div id="content">
        <h3>github-to-pushbullet webhook</h3>

This is a webhook for <a href="https://help.github.com/articles/post-receive-hooks">GitHub's post-receive</a> notifications.
It will forward the notification through <a href="http://pushbullet.com">PushBullet</a> to your Android device.

        <p> Steps:

        <ul>
        <li> Open your repository on GitHub and go to its Admin page
        <li>Click Service Hooks
        <li>Click WebHook URLs
        <li>Enter <b>http://ghpbhook.herokuapp.com/pb/YOUR_API_KEY</b>
        <li>Click Update Settings
        </ul>

    </div>
  </body>
</html>
`

func atMost(s string, n int) string {
	l := len(s)
	if l > n {
		l = n
	}
	return s[:l]
}

var pushTemplate = ttmpl.Must(ttmpl.New("pushmsg").Funcs(ttmpl.FuncMap{"trim": atMost}).Parse(pushTemplateText))

const pushTemplateText = `
{{ .Pusher.Name }} pushed {{ if .TmplParams.CommitCount }}{{ .TmplParams.CommitCount }} commits{{ end }} to {{ .Repository.Owner.Name }}/{{ .Repository.Name }} 
{{ range .Commits }}   {{if .Author.Username}}{{.Author.Username}}{{else}}{{.Author.Name}}{{end}} {{ trim .Id 7 }} - {{ trim .Message 40 }}
{{ end }}
`

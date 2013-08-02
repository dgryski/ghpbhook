package main

import (
	"bytes"
	"encoding/json"
	"github.com/xconstruct/go-pushbullet"
	htmpl "html/template"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	ttmpl "text/template"
)

type pushPayload struct {
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
			Name     string
		}
		Id      string
		Message string
	}
}

func main() {

	http.HandleFunc("/pb/", func(w http.ResponseWriter, r *http.Request) {
		path := strings.Split(r.URL.Path, "/")

		if len(path) != 3 && len(path) != 4 {
			http.NotFound(w, r)
			return
		}

		hasDeviceId := len(path) == 4

		apikey := path[2]
		if len(apikey) != 32 {
			http.Error(w, "", http.StatusBadRequest)
			return
		}

		var deviceId int
		if hasDeviceId {
			var err error
			deviceId, err = strconv.Atoi(path[3])
			if err != nil {
				http.Error(w, "", http.StatusBadRequest)
				return
			}
		}

		err := r.ParseForm()
		if err != nil {
			http.Error(w, "", http.StatusBadRequest)
			return
		}

		payloadJSON := r.PostFormValue("payload")
		if payloadJSON == "" {
			http.Error(w, "", http.StatusBadRequest)
			return
		}

		var payload pushPayload
		err = json.Unmarshal([]byte(payloadJSON), &payload)
		if err != nil {
			http.Error(w, "", http.StatusBadRequest)
			return
		}

		var o bytes.Buffer
		err = pushTemplate.Execute(&o, payload)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		notification := o.String()

		pb := pushbullet.New(apikey)
		devices, err := pb.Devices()
		if err != nil {
			http.Error(w, err.Error(), http.StatusServiceUnavailable)
			return
		}

		if len(devices) == 0 {
			// nothing to do, but do it successfully
			return
		}

		success := 0
		tries := 0
		for _, device := range devices {
			// TODO(dgryski): spawn these in parallel?
			if !hasDeviceId || deviceId == device.Id {
				err = pb.PushNote(device.Id, "GitHub", notification)
				tries++
				if err == nil {
					success++
				}
			}
		}

		// user sent a device id but there was no match for the devices associated with the key
		if hasDeviceId && tries == 0 {
			http.NotFound(w, r)
			return
		}

		if success == 0 {
			// no notifications succeeded :(
			http.Error(w, "Error sending notification", http.StatusServiceUnavailable)
		}

		return
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
  <title>ghpbhook</title>
  <style type="text/css">

    @import url(//fonts.googleapis.com/css?family=Droid+Serif);

    body {
       background : lightgrey ;
       margin-top : 100px ;
       font-family : 'Droid Serif' ;
    }

    div#content
    {
       margin : auto ;
       width : 90%;
    }

</style>

  <body>
    <div id="content">
        <h3>github-to-pushbullet webhook</h3>

This is a webhook for <a href="https://help.github.com/articles/post-receive-hooks">GitHub's post-receive</a> notifications.
It will forward the notification through <a href="http://pushbullet.com">PushBullet</a> to your Android device.
<pre>
Garen Torikian pushed 3 commits to octokitty/testing
    octokitty c441029 - Test
    octokitty 36c5f22 - This is me testing the windows client.
    octokitty 1481a2d - Rename madame-bovary.txt to words/madame-bovary.txt
</pre>

        <p> Setup:

        <ul>
        <li>Install the <a href="https://play.google.com/store/apps/details?id=com.pushbullet.android">PushBullet app</a> on your phone
        <li>Go to the <a href="https://www.pushbullet.com/settings">PushBullet settings page</a> and copy your API key.
        <li>Next, open your repository on GitHub and go to its Admin page
        <li>Click Service Hooks
        <li>Click WebHook URLs
        <li>Enter
            <ul><li><b><tt>http://ghpbhook.herokuapp.com/pb/YOUR_API_KEY</tt></b></ul>
            or, to limit to a specific device Id,
            <ul><li><b><tt>http://ghpbhook.herokuapp.com/pb/YOUR_API_KEY/DEVICE_ID</tt></b></ul>
        <li>Click Update Settings
        </ul>

        Bugs and patches: <a href="http://github.com/dgryski/ghpbhook">github.com/dgryski/ghpbhook</a>

    </div>
  </body>
</html>
`

func trim(s string, n int) string {
	l := len(s)
	if l > n {
		l = n
	}
	return s[:l]
}
func ellipsize(s string, n int) string {
	l := len(s)
	if l > n {
		l = n
		return s[:n-5] + "(...)"
	}
	return s
}

var pushTemplate = ttmpl.Must(ttmpl.New("pushmsg").Funcs(ttmpl.FuncMap{"trim": trim, "ellipsize": ellipsize}).Parse(pushTemplateText))

const pushTemplateText = `
{{ .Pusher.Name }} pushed {{ if len .Commits }}{{ len .Commits}} commits{{ end }} to {{ .Repository.Owner.Name }}/{{ .Repository.Name }}
{{ range .Commits }}  {{if .Author.Username}}{{.Author.Username}}{{else}}{{.Author.Name}}{{end}} {{ trim .Id 7 }} - {{ ellipsize .Message 60 }}
{{ end }}
`

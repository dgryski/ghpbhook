package main

import (
	"bytes"
	"encoding/json"
	"github.com/xconstruct/go-pushbullet"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	ttmpl "text/template"
)

type notificationMaker interface {
	who() string
	payloadToNotification(payloadJSON []byte) (string, int)
}

type github int

func (gh github) who() string { return "GitHub" }

type ghPushPayload struct {
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

var ghPushTemplate = ttmpl.Must(ttmpl.New("ghpushmsg").Funcs(ttmpl.FuncMap{"trim": trim, "ellipsize": ellipsize}).Parse(ghPushTemplateText))

const ghPushTemplateText = `
{{ .Pusher.Name }} pushed {{ if len .Commits }}{{ len .Commits}} commits{{ end }} to {{ .Repository.Owner.Name }}/{{ .Repository.Name }}
{{ range .Commits }}  {{if .Author.Username}}{{.Author.Username}}{{else}}{{.Author.Name}}{{end}} {{ trim .Id 7 }} - {{ ellipsize .Message 60 }}
{{ end }}
`

func (gh github) payloadToNotification(payloadJSON []byte) (string, int) {

	var payload ghPushPayload
	err := json.Unmarshal(payloadJSON, &payload)
	if err != nil {
		return "", http.StatusBadRequest
	}

	var o bytes.Buffer
	err = ghPushTemplate.Execute(&o, payload)
	if err != nil {
		return "", http.StatusInternalServerError
	}

	return o.String(), http.StatusOK
}

type bitbucket int

func (bb bitbucket) who() string { return "BitBucket" }

type bbPushPayload struct {
	User       string
	Repository struct {
		Slug  string
		Owner string
	}
	Commits []struct {
		Author  string
		Node    string
		Message string
	}
}

var bbPushTemplate = ttmpl.Must(ttmpl.New("bbpushmsg").Funcs(ttmpl.FuncMap{"trim": trim, "ellipsize": ellipsize}).Parse(bbPushTemplateText))

const bbPushTemplateText = `
{{ .User }} pushed {{ if len .Commits }}{{ len .Commits}} commits{{ end }} to {{ .Repository.Owner }}/{{ .Repository.Slug }}
{{ range .Commits }}  {{.Author}} {{ trim .Node 7 }} - {{ ellipsize .Message 60 }}
{{ end }}
`

func (bb bitbucket) payloadToNotification(payloadJSON []byte) (string, int) {

	var payload bbPushPayload
	err := json.Unmarshal(payloadJSON, &payload)
	if err != nil {
		return "", http.StatusBadRequest
	}

	var o bytes.Buffer
	err = bbPushTemplate.Execute(&o, payload)
	if err != nil {
		return "", http.StatusInternalServerError
	}

	return o.String(), http.StatusOK
}

func pushHandler(w http.ResponseWriter, r *http.Request, nm notificationMaker) {

	args := strings.Split(r.URL.Path, "/")
	// strip "/ghhook/push/" entries
	args = args[3:]

	if len(args) != 1 && len(args) != 2 {
		http.Error(w, "", http.StatusBadRequest)
		return
	}

	hasDeviceId := len(args) == 2

	apikey := args[0]
	if len(apikey) != 32 {
		http.Error(w, "", http.StatusBadRequest)
		return
	}

	var deviceId int
	if hasDeviceId {
		var err error
		deviceId, err = strconv.Atoi(args[1])
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

	notification, httpStatus := nm.payloadToNotification([]byte(payloadJSON))

	if httpStatus != http.StatusOK {
		http.Error(w, "", httpStatus)
		return
	}

	pb := pushbullet.New(apikey)
	devices, err := pb.Devices()
	if err != nil {
		http.Error(w, "", http.StatusServiceUnavailable)
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
			err = pb.PushNote(device.Id, nm.who(), notification)
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
}

func main() {

	// old
	http.HandleFunc("/hook/push/", func(w http.ResponseWriter, r *http.Request) { pushHandler(w, r, github(0)) })

	// new
	http.HandleFunc("/ghhook/push/", func(w http.ResponseWriter, r *http.Request) { pushHandler(w, r, github(0)) })
	http.HandleFunc("/bbhook/push/", func(w http.ResponseWriter, r *http.Request) { pushHandler(w, r, bitbucket(0)) })

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}

		w.Write([]byte(rootTemplateHTML))
	})

	port := ":8080"

	if p := os.Getenv("PORT"); p != "" {
		port = ":" + p
	}

	log.Println("Listening on port", port)

	log.Fatal(http.ListenAndServe(port, nil))
}

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
        </ul>

        <p> For GitHub
        <ul>
        <li>Open your repository and go to its <em>Settings</em> page
        <li>Click <em>Service Hooks</em>
        <li>Click <em>WebHook URLs</em>
        <li>Enter
            <ul><li><b><tt>http://ghpbhook.herokuapp.com/hook/push/YOUR_API_KEY</tt></b></ul>
            or, to limit to a specific device Id,
            <ul><li><b><tt>http://ghpbhook.herokuapp.com/hook/push/YOUR_API_KEY/DEVICE_ID</tt></b></ul>
        <li>Click <em>Update Settings</em>
        <li>Click <em>Test Hook</em> for instant gratification.
        </ul>

        <p> For BitBucket
        <ul>
        <li>Open your repository and go to its <em>Administration</em> page (the gear)
        <li>Click <em>Hooks</em>
        <li>Choose <em>POST</em> from the dropdown and click <em>Add Hook</em>
        <li>Enter
            <ul><li><b><tt>http://ghpbhook.herokuapp.com/bbhook/push/YOUR_API_KEY</tt></b></ul>
            or, to limit to a specific device Id,
            <ul><li><b><tt>http://ghpbhook.herokuapp.com/bbhook/push/YOUR_API_KEY/DEVICE_ID</tt></b></ul>
        <li>Click <em>Save</em>
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

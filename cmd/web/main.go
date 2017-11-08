package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"

	"github.com/gorilla/mux"
	"github.com/sirupsen/logrus"
)

func WriteResponseHeaders(w http.ResponseWriter, h http.Header) {
	for k, vl := range h {
		for i, v := range vl {
			if i == 0 {
				w.Header().Set(k, v)
			} else {
				w.Header().Add(k, v)
			}
		}
	}
}
func CloneHeaders(h http.Header) http.Header {
	h2 := make(http.Header, len(h))
	for k, vv := range h {
		vv2 := make([]string, len(vv))
		copy(vv2, vv)
		h2[k] = vv2
	}
	return h2
}

func redirectPolicyFunc(authHeader string) func(*http.Request, []*http.Request) error {
	return func(r *http.Request, via []*http.Request) error {
		logrus.Infof("==> %s (policy)", r.URL.String())
		// nest api proxies to firebase services somewhere else
		r.Header.Set("Authorization", authHeader)
		if via != nil && len(via) == 10 {
			return fmt.Errorf("too many redirects")
		}
		return nil
	}
}

func RouteProxy(w http.ResponseWriter, r *http.Request) {
	// make the proxy request
	path := fmt.Sprintf("https://developer-api.nest.com/%s", mux.Vars(r)["subRoute"])
	logrus.Infof("=> %s %s", r.Method, path)
	body, err := ioutil.ReadAll(r.Body) // we may need to read this multiple times... :'(
	if err != nil {
		writeError(w, err)
		return
	}

	// garmin simulator does some funky boolean values
	body = bytes.Replace(body, []byte("True"), []byte("true"), -1)
	body = bytes.Replace(body, []byte("False"), []byte("false"), -1)

	client := &http.Client{
		CheckRedirect: redirectPolicyFunc(r.Header.Get("Authorization")),
	}
	var res *http.Response
	for tries := 0; tries < 10; tries++ {
		req, err := http.NewRequest(r.Method, path, bytes.NewReader(body))
		if err != nil {
			writeError(w, err)
			return
		}
		req.Header = CloneHeaders(r.Header)
		res, err = client.Do(req)
		if err != nil {
			writeError(w, err)
			return
		} else if res.StatusCode == http.StatusTemporaryRedirect {
			req.URL, err = url.Parse(res.Header.Get("Location"))
			if err != nil {
				writeError(w, err)
				return
			} else if tries == 9 {
				writeError(w, fmt.Errorf("too many redirects"))
				return
			}
			res.Body.Close()
			logrus.Infof("==> %s (manual)", req.URL.String())
		} else {
			break
		}
	}
	defer res.Body.Close()

	// send the proxy response
	WriteResponseHeaders(w, res.Header)
	w.WriteHeader(res.StatusCode)
	//_, err = io.Copy(os.Stdout, res.Body)
	_, err = io.Copy(w, res.Body)
	if err != nil {
		writeError(w, err)
		return
	}
	logrus.Infof("<= %s", res.Status)
}

func writeError(w http.ResponseWriter, err error) {
	logrus.Warn(err)
	w.WriteHeader(http.StatusInternalServerError)
	json.NewEncoder(w).Encode(map[string]string{"message": err.Error()})
}

func main() {
	ll := os.Getenv("LOG_LEVEL")
	if ll == "" {
		ll = "INFO"
	}
	l, err := logrus.ParseLevel(ll)
	if err != nil {
		panic(err)
	}
	logrus.SetLevel(l)
	port := os.Getenv("PORT")
	if port == "" {
		port = "3000"
	}
	r := mux.NewRouter()
	r.HandleFunc("/api/{subRoute:.*}", RouteProxy).Methods("GET", "POST", "PUT", "PATCH")
	logrus.Fatal(http.ListenAndServe(fmt.Sprintf(":%s", port), r))
}

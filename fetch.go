package main

import (
	"github.com/urfave/cli"
	"net/http"
	"net/http/cookiejar"
	"log"
	"time"
	"encoding/json"
	"net/url"
	"bytes"
	"fmt"
	"io/ioutil"
	"errors"
)

func newClient(host, user, pass string) *http.Client {
	jar, err := cookiejar.New(&cookiejar.Options{PublicSuffixList: nil})
	if nil != err {
		log.Fatal(err)
	}

	r := &http.Client{
		Jar:     jar,
		Timeout: 5 * time.Second,
	}

	data := map[string]string{
		"username": user,
		"password": pass,
	}
	jsonPayload, err := json.Marshal(data)
	if nil != err {
		log.Fatal(err)
	}

	loginUrl := url.URL{Scheme: "http", Host: host, Path: "/api/1.1/login"}

	result, err := r.Post(loginUrl.String(), "application/json", bytes.NewReader(jsonPayload))

	if nil != err {
		log.Fatal(err)
	}

	if 200 != result.StatusCode {
		log.Fatalf("Cannot log into camera %s with user/pass %s/%s. %s\n", loginUrl.String(), user, pass, result.Status)
	}
	for _, cookie := range jar.Cookies(&loginUrl) {
		fmt.Printf("  %s: %s\n", cookie.Name, cookie.Value)
	}

	return r
}

func FetchAction(c *cli.Context) error {
	var client *http.Client = nil

	interval := c.Int("interval")
	host := c.String("host")
	user := c.String("user")
	pass := c.String("pass")
	if 0 >= interval {
		return errors.New("invalid interval, Please enter a positive number")
	}
	if "" == host {
		return errors.New("please provide the host IP/Domain to fetch from")
	}

	snapChan := make(chan bool)
	takePhoto := func(dt time.Time) {
		log.Println("Taking Photo")
		if nil == client {
			client = newClient(host, user, pass)
		}

		urlStr := "http://" + host + "/snap.jpeg"
		r, err := client.Get(urlStr)
		if nil != err {
			log.Println("Error fetching image", err)
			client = nil
			return
		}
		if r.StatusCode != 200 {
			log.Println("Error fetching image,", r.Status, urlStr)
			client = nil
			return
		}

		filename := "photo_" + dt.Format(time.RFC3339) + ".jpeg"
		image, err := ioutil.ReadAll(r.Body)
		if nil != err {
			log.Println(err)
			return
		}
		ioutil.WriteFile(filename, image, 0644)
		log.Printf("Wrote to file %s with %d bytes", filename, len(image))
	}
	timerDuration := time.Duration(interval) * time.Second
	timer := time.NewTimer(timerDuration)
	takePhoto(time.Now())
	for {
		select {
		case t := <-timer.C:
			go takePhoto(t)
			timer.Reset(timerDuration)
		case <-snapChan:
			go takePhoto(time.Now())
		}

	}

	return nil
}

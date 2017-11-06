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
	"io/ioutil"
	"errors"
	"path/filepath"
	"os"
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

	return r
}

func FetchAction(c *cli.Context) error {
	var client *http.Client = nil

	interval := c.Int("interval")
	host := c.String("host")
	user := c.String("user")
	pass := c.String("pass")

	directory, err := filepath.Abs(c.String("prefix"))
	if err != nil {
		return err
	}
	if err := os.Mkdir(directory, 0775); nil != err {
		if !os.IsExist(err) {
			return err
		}
	}

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
		dateFolder := directory + dt.Format("/2006-01-02/")
		if err := os.Mkdir(dateFolder, 0775); nil != err {
			if !os.IsExist(err) {
				log.Fatalln("Cannot create folder", dateFolder, err)
			}
		}

		filename := dateFolder + dt.Format("15_04_05") + ".jpeg"
		image, err := ioutil.ReadAll(r.Body)
		if nil != err {
			log.Println(err)
			return
		}
		if err := ioutil.WriteFile(filename, image, 0644); err != nil {
			log.Println(err)
		}
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

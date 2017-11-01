package main

import (
	"github.com/urfave/cli"
	"os"
	"github.com/pkg/errors"
	"time"
	"net/http"
	"net/http/cookiejar"
	"log"
	"net/url"
	"fmt"
	"encoding/json"
	"bytes"
	"io/ioutil"
	"regexp"
	"path/filepath"
	_ "image/jpeg"
	_ "image/png"
	"image"
	"image/jpeg"
	"image/draw"
	"sync"
	"sort"
)

func main() {
	app := cli.NewApp()

	app.Name = "Auto Slice of Life"
	app.Usage = "Generates Slice of Life pictures from security cameras"

	app.Commands = []cli.Command{
		{
			Name:   "fetch",
			Usage:  "Fetches an image from a URL at the given interval",
			Action: fetchAction,
			Flags: []cli.Flag{
				cli.StringFlag{
					Name:  "host",
					Usage: "The host to fetch an image from",
				},
				cli.StringFlag{
					Name:  "user",
					Usage: "The username to provide to the UBNT camera",
					Value: "ubnt",
				},
				cli.StringFlag{
					Name:  "pass",
					Usage: "The password to access the camera",
				},
				cli.IntFlag{
					Name:  "interval",
					Usage: "The number of seconds between image captures",
				},
				cli.StringFlag{
					Name:  "start",
					Usage: "A time string to start capturing at in 24 hour time, e.g. 13:00:00",
				},
			},
		},
		{
			Name:   "merge",
			Usage:  "Merges a directory of images into a single slice of life image",
			Action: mergeAction,
			Flags: []cli.Flag{
				cli.StringFlag{
					Name:  "dir",
					Usage: "The directory to look for images in",
					Value: ".",
				},
			},
		},
	}

	err := app.Run(os.Args)
	if nil != err {
		log.Fatal(err.Error())
	}
}

func fetchAction(c *cli.Context) error {
	var client *http.Client = nil

	interval := c.Int("interval")
	host := c.String("host")
	user := c.String("user")
	pass := c.String("pass")
	if 0 >= interval {
		return errors.New("Invalid interval, Please enter a positive number")
	}
	if "" == host {
		return errors.New("Please provide the host IP/Domain to fetch from")
	}

	newClient := func() *http.Client {
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

	snapChan := make(chan bool)
	takePhoto := func(dt time.Time) {
		log.Println("Taking Photo")
		if nil == client {
			client = newClient()
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


// change to fan out - fan in instead of doing it inline
func mergeAction(c *cli.Context) error {
	directory, err := filepath.Abs(c.String("dir"))
	if err != nil {
		return err
	}

	listing, err := ioutil.ReadDir(directory)
	if err != nil {
		return err
	}

	log.Println("Working in directory:", directory)

	getPhoto := func(path string) (image.Image, error) {
		f, err := os.Open(path)
		if nil != err {
			return nil, err
		}
		first, _, err := image.Decode(f)
		if nil != err {
			return nil, err
		}

		return first, nil
	}
	imageFiles := make([]string, 0)
	images := sync.Map{}
	wg := sync.WaitGroup{}
	for _, f := range listing {
		if f.IsDir() {
			continue
		}

		ok, err := regexp.MatchString(".(jpe?g|png)$", f.Name())
		if nil != err {
			return err
		}
		if ok {
			path := directory+"/"+f.Name()
			//if len(imageFiles) <1000 {
			//	wg.Add(1)
			//	go func() {
			//		photo, _ := getPhoto(path)
			//		images.Store(path, photo)
			//		wg.Done()
			//	}()
			//}

			imageFiles = append(imageFiles, path)
		}
	}
	wg.Wait()
	sort.Strings(imageFiles)

	numImages := len(imageFiles)
	if numImages == 0 {
		return errors.New("No images to work with")
	}
	log.Println("Found", numImages, "image files to process...")

	// load the first image and get the dimensions of our new merge


	first, err := getPhoto(imageFiles[0])
	if err != nil {
		return err
	}
	width := first.Bounds().Dx()
	height := first.Bounds().Dy()

	rgba := image.NewRGBA(first.Bounds())

	first = nil

	var x, i int
	var photo image.Image
	for x < width {
		fmt.Printf("%d / %d (%0.2f%%)   \r", i, numImages, (float32(i)/float32(numImages))*100)
		if i >= numImages {
			break
		}
		p, ok := images.Load(imageFiles[i])

		if ok {
			photo = p.(image.Image)
		} else {
			photo, _ = getPhoto(imageFiles[i])
		}
		var numColsToCopy = 1

		draw.Draw(rgba, image.Rect(x, 0, x+numColsToCopy, height), photo, image.Pt(x, 0), draw.Src)
		x += numColsToCopy
		photo = nil
		i++
	}

	filename := fmt.Sprintf("slice_of_life_%d.jpeg", numImages)
	f, err := os.OpenFile(filename, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0644)
	if err != nil {
		return err
	}
	return jpeg.Encode(f, rgba, &jpeg.Options{Quality: 80})
}

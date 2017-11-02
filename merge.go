package main

import (
	"path/filepath"
	"io/ioutil"
	"log"
	"image"
	"os"
	"regexp"
	"sort"
	"fmt"
	"image/draw"
	"image/jpeg"
	_ "image/png"
	"github.com/urfave/cli"
	"errors"
	"runtime"
)

type photoSlice struct {
	image         image.Image
	path          string
	width, height int
	toCopy        image.Rectangle
	toPlace       image.Point
}

func newPhoto(path string, toCopy image.Rectangle, toPlace image.Point) *photoSlice {
	p := photoSlice{
		path:    path,
		toCopy:  toCopy,
		toPlace: toPlace,
	}
	return &p
}

func (p *photoSlice) load() error {
	if nil == p.image {
		f, err := os.Open(p.path)
		if nil != err {
			return err
		}
		p.image, _, err = image.Decode(f)
		if nil != err {
			return err
		}
		p.width = p.image.Bounds().Dx()
		p.height = p.image.Bounds().Dy()

	}

	return nil
}
func (p *photoSlice) Bounds() image.Rectangle {
	if nil == p.image {
		return image.Rect(0, 0, 0, 0)
	}

	return p.image.Bounds()
}

// change to fan out - fan in instead of doing it inline
func MergeAction(c *cli.Context) error {
	directory, err := filepath.Abs(c.String("dir"))
	if err != nil {
		return err
	}

	imageFiles, err := getImageList(directory)
	if err != nil {
		return err
	}

	numImages := len(imageFiles)
	if numImages == 0 {
		return errors.New("no images to work with")
	}
	log.Println("Found", numImages, "image files to process...")

	first := newPhoto(imageFiles[0], image.Rect(0, 0, 0, 0), image.Pt(0, 0))

	if err := first.load(); nil != err {
		return err
	}

	rgba := image.NewRGBA(first.Bounds())

	imagesToProcessQueue := make(chan *photoSlice)
	quitChan := make(chan bool)

	maxProcs := runtime.NumCPU() - 1
	for i := 0; i < maxProcs; i++ {
		go handleImage(rgba, imagesToProcessQueue, quitChan)
	}

	remainingWidth := first.width
	var x, i int
	var numColsToCopy int
	for remainingWidth > 0 && i < numImages {
		numColsToCopy = remainingWidth / (numImages - i)

		if 0 == numColsToCopy {
			numColsToCopy = 1
		}

		r := image.Rect(x, 0, x+numColsToCopy, first.height)
		pt := image.Pt(x, 0)
		imagesToProcessQueue <- newPhoto(imageFiles[i], r, pt)

		remainingWidth -= numColsToCopy
		x += numColsToCopy
		i++
	}

	for i := 0; i < maxProcs; i++ {
		quitChan <- true
	}

	re := regexp.MustCompile("[^a-zA-Z0-9]")
	filename := fmt.Sprintf("slice_of_life_%s_%d.jpeg", re.ReplaceAllString(filepath.Base(directory), "_"), numImages)
	f, err := os.OpenFile(filename, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0644)
	if err != nil {
		return err
	}
	err = jpeg.Encode(f, rgba, &jpeg.Options{Quality: 80})

	log.Println("Wrote jpeg", filename)
	return err
}

func handleImage(rgba *image.RGBA, imagesToProcessQueue chan *photoSlice, quitChan chan bool) {
	for {
		select {
		case photo := <-imagesToProcessQueue:
			photo.load()
			draw.Draw(rgba, photo.toCopy, photo.image, photo.toPlace, draw.Src)
		case <-quitChan:
			return
		}
	}
}

func getImageList(directory string) ([]string, error) {
	imageFiles := make([]string, 0)

	listing, err := ioutil.ReadDir(directory)
	if err != nil {
		return imageFiles, err
	}

	log.Println("Working in directory:", directory)

	for _, f := range listing {
		if f.IsDir() {
			continue
		}

		ok, err := regexp.MatchString(".(jpe?g|png)$", f.Name())
		if nil != err {
			return imageFiles, err
		}
		if ok {
			path := directory + "/" + f.Name()

			imageFiles = append(imageFiles, path)
		}
	}
	sort.Strings(imageFiles)
	return imageFiles, nil
}

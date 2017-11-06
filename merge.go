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
	"time"
)

type (
	photoJob struct {
		image         image.Image
		path          string
		width, height int
		toCopy        image.Rectangle
		toPlace       image.Point
	}

	imageSlicer struct {
		in     chan *photoJob
		out    chan *photoJob
		bounds image.Rectangle
		image  *image.RGBA
		quit   chan bool
		// generates the photoJob's that the combiner combines
		slicer   func()
		combiner func()
	}
)

func newPhotoJob(path string, toCopy image.Rectangle, toPlace image.Point) *photoJob {
	p := photoJob{
		path:    path,
		toCopy:  toCopy,
		toPlace: toPlace,
	}
	return &p
}

func (p *photoJob) load() error {
	if nil == p.image {
		debugMessage("Loading image", p.path)
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

func (p *photoJob) Bounds() image.Rectangle {
	if nil == p.image {
		return image.Rect(0, 0, 0, 0)
	}

	return p.image.Bounds()
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

func debugMessage(msg ...interface{}) {
	if debug {
		log.Println(msg...)
	}
}

// change to fan out - fan in instead of doing it inline
func MergeAction(c *cli.Context) error {
	directory, err := filepath.Abs(c.String("dir"))
	if err != nil {
		return err
	}

	verticalSlice := c.BoolT("vertical")
	horizontalSlice := c.BoolT("horizontal")
	prefix := c.String("prefix")

	imageFiles, err := getImageList(directory)
	if err != nil {
		return err
	}

	numImages := len(imageFiles)
	if numImages == 0 {
		return errors.New("no images to work with")
	}

	debugMessage("Found", numImages, "image files to merge...")

	first := newPhotoJob(imageFiles[0], image.Rect(0, 0, 0, 0), image.Pt(0, 0))

	if err := first.load(); nil != err {
		return err
	}

	is := newImageSlicer(first.Bounds())
	is.combiner = is.straightCombiner
	if verticalSlice {
		is.slicer = is.verticalSlicer(imageFiles)
	}
	if horizontalSlice {
		is.slicer = is.horizontalSlicer(imageFiles)
	}

	is.wait()

	re := regexp.MustCompile("[^a-zA-Z0-9]")
	if "" != prefix {
		prefix += "_"
	}

	filename := fmt.Sprintf("%sslice_of_life_%s_%d.jpeg", prefix, re.ReplaceAllString(filepath.Base(directory), "_"), numImages)
	saveImage(filename, is.image)

	log.Println("Wrote jpeg", filename)
	return err
}

func (is *imageSlicer) verticalSlicer(list []string) func() {
	numImages := len(list)
	return func() {
		remainingWidth := is.bounds.Dx()
		var x, i int
		var numColsToCopy int
		for remainingWidth > 0 && i < numImages {
			debugMessage("x", x, "Remaining Width:", remainingWidth, "#", i, "/", numImages)
			numColsToCopy = remainingWidth / (numImages - i)

			if 0 == numColsToCopy {
				numColsToCopy = 1
			}

			r := image.Rect(x, 0, x+numColsToCopy, is.bounds.Dy())
			pt := image.Pt(x, 0)
			is.in <- newPhotoJob(list[i], r, pt)

			remainingWidth -= numColsToCopy
			x += numColsToCopy
			i++
		}
		debugMessage("Slicing complete")
	}
}

func (is *imageSlicer) horizontalSlicer(list []string) func() {
	numImages := len(list)
	return func() {
		remainingHeight := is.bounds.Dy()
		var y, i int
		var numColsToCopy int
		for remainingHeight > 0 && i < numImages {
			debugMessage("y", y, "Remaining Width:", remainingHeight, "#", i, "/", numImages)
			numColsToCopy = remainingHeight / (numImages - i)

			if 0 == numColsToCopy {
				numColsToCopy = 1
			}

			r := image.Rect(0, y, is.bounds.Dx(), y+numColsToCopy)
			pt := image.Pt(0, y)
			is.in <- newPhotoJob(list[i], r, pt)

			remainingHeight -= numColsToCopy
			y += numColsToCopy
			i++
		}
	}
}

func (is *imageSlicer) straightCombiner() {
	is.image = image.NewRGBA(is.bounds)

	for job := range is.out {
		draw.Draw(is.image, job.toCopy, job.image, image.Pt(0,0), draw.Src)
	}
}

func newImageSlicer(bounds image.Rectangle) *imageSlicer {
	is := imageSlicer{
		bounds: bounds,
		image:  image.NewRGBA(bounds),
		in:     make(chan *photoJob),
		out:    make(chan *photoJob),
		quit:   make(chan bool),
	}
	maxProcs := runtime.NumCPU() - 1
	for i := 0; i < maxProcs; i++ {
		go is.handleImage()
	}

	return &is
}

func (is *imageSlicer) handleImage() {
	for {
		select {
		case photo := <-is.in:
			photo.load()

			r := image.Rect(0,0, photo.toCopy.Dx(), photo.toCopy.Dy())

			debugMessage("Slice Size: ", r)

			slice := image.NewRGBA(r)
			draw.Draw(slice, r, photo.image, photo.toPlace, draw.Src)

			photo.image = slice
			is.out <- photo
		case <-is.quit:
			return
		}
	}
}

func saveImage(filename string, img image.Image) {
	f, err := os.OpenFile(filename, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0644)
	defer f.Close()
	if err != nil {
		log.Fatalln(err)
	}
	err = jpeg.Encode(f, img, &jpeg.Options{Quality: 80})
}

func (is *imageSlicer) wait() {
	go is.combiner()

	is.slicer()

	maxProcs := runtime.NumCPU() - 1
	for i := 0; i < maxProcs; i++ {
		is.quit <- true
	}
	for 0 != len(is.out) {
		time.Sleep(time.Millisecond)
	}
	close(is.out)
}
